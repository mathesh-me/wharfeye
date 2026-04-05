package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// Snapshot represents a point-in-time collection of all container metrics.
type Snapshot struct {
	Timestamp  time.Time                      `json:"timestamp"`
	Containers []models.Container             `json:"containers"`
	Stats      map[string]*models.ContainerStats `json:"stats"`
	Host       HostSummary                    `json:"host"`
}

// HostSummary holds aggregated host-level metrics.
type HostSummary struct {
	RuntimeName       string  `json:"runtime_name"`
	RuntimeVersion    string  `json:"runtime_version"`
	TotalContainers   int     `json:"total_containers"`
	RunningContainers int     `json:"running_containers"`
	StoppedContainers int     `json:"stopped_containers"`
	TotalCPUPercent   float64 `json:"total_cpu_percent"`
	AvgCPUPercent     float64 `json:"avg_cpu_percent"`
	TotalMemoryUsage  uint64  `json:"total_memory_usage"`
	AvgMemoryUsage    uint64  `json:"avg_memory_usage"`
	TotalMemoryLimit  uint64  `json:"total_memory_limit"`
	TotalNetRx        uint64  `json:"total_net_rx"`
	TotalNetTx        uint64  `json:"total_net_tx"`
	TotalBlockRead    uint64  `json:"total_block_read"`
	TotalBlockWrite   uint64  `json:"total_block_write"`
	ImageCount        int     `json:"image_count"`
	VolumeCount       int     `json:"volume_count"`
}

// Collector polls the container runtime at regular intervals and maintains
// a rolling buffer of metric snapshots.
type Collector struct {
	client   runtime.Client
	interval time.Duration
	bufSize  int

	mu       sync.RWMutex
	buffer   *RingBuffer[Snapshot]
	latest   *Snapshot
	info     *runtime.RuntimeInfo

	// subscribers receive snapshots on each collection tick
	subMu   sync.RWMutex
	subs    map[string]chan Snapshot
	nextID  int
}

// NewCollector creates a new metrics collector.
func NewCollector(client runtime.Client, interval time.Duration, historyWindow time.Duration) *Collector {
	// Calculate buffer size: how many snapshots fit in the history window
	bufSize := int(historyWindow / interval)
	if bufSize < 10 {
		bufSize = 10
	}
	if bufSize > 3600 {
		bufSize = 3600 // cap at ~1h at 1s intervals
	}

	return &Collector{
		client:   client,
		interval: interval,
		bufSize:  bufSize,
		buffer:   NewRingBuffer[Snapshot](bufSize),
		subs:     make(map[string]chan Snapshot),
	}
}

// Run starts the collection loop. It blocks until the context is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	// Fetch runtime info once at startup
	info, err := c.client.Info(ctx)
	if err != nil {
		return fmt.Errorf("getting runtime info: %w", err)
	}
	c.mu.Lock()
	c.info = info
	c.mu.Unlock()

	// Collect immediately on start
	c.collect(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// collect performs a single collection cycle.
func (c *Collector) collect(ctx context.Context) {
	containers, err := c.client.ListContainers(ctx)
	if err != nil {
		slog.Error("collecting containers", "error", err)
		return
	}

	stats := make(map[string]*models.ContainerStats, len(containers))
	var totalCPU float64
	var totalMem, totalMemLimit uint64
	var totalNetRx, totalNetTx, totalBlockRead, totalBlockWrite uint64
	running, stopped := 0, 0
	statsCount := 0

	for _, ctr := range containers {
		if ctr.IsRunning() {
			running++
			s, err := c.client.ContainerStats(ctx, ctr.ID)
			if err != nil {
				slog.Warn("collecting stats", "container", ctr.Name, "error", err)
				continue
			}
			stats[ctr.ID] = s
			statsCount++
			totalCPU += s.CPU.Percent
			totalMem += s.Memory.Usage
			if s.Memory.Limit > 0 && s.Memory.Limit != ^uint64(0) {
				totalMemLimit += s.Memory.Limit
			}
			totalNetRx += s.NetworkIO.RxBytes
			totalNetTx += s.NetworkIO.TxBytes
			totalBlockRead += s.BlockIO.ReadBytes
			totalBlockWrite += s.BlockIO.WriteBytes
		} else {
			stopped++
		}
	}

	avgCPU := 0.0
	avgMem := uint64(0)
	if statsCount > 0 {
		avgCPU = totalCPU / float64(statsCount)
		avgMem = totalMem / uint64(statsCount)
	}

	c.mu.RLock()
	info := c.info
	c.mu.RUnlock()

	runtimeName := ""
	runtimeVersion := ""
	if info != nil {
		runtimeName = info.Name
		runtimeVersion = info.Version
	}

	// Get image and volume counts (best effort)
	imageCount := 0
	volumeCount := 0
	if images, err := c.client.ListImages(ctx); err == nil {
		imageCount = len(images)
	}
	if volumes, err := c.client.ListVolumes(ctx); err == nil {
		volumeCount = len(volumes)
	}

	snapshot := Snapshot{
		Timestamp:  time.Now(),
		Containers: containers,
		Stats:      stats,
		Host: HostSummary{
			RuntimeName:       runtimeName,
			RuntimeVersion:    runtimeVersion,
			TotalContainers:   len(containers),
			RunningContainers: running,
			StoppedContainers: stopped,
			TotalCPUPercent:   totalCPU,
			AvgCPUPercent:     avgCPU,
			TotalMemoryUsage:  totalMem,
			AvgMemoryUsage:    avgMem,
			TotalMemoryLimit:  totalMemLimit,
			TotalNetRx:        totalNetRx,
			TotalNetTx:        totalNetTx,
			TotalBlockRead:    totalBlockRead,
			TotalBlockWrite:   totalBlockWrite,
			ImageCount:        imageCount,
			VolumeCount:       volumeCount,
		},
	}

	c.mu.Lock()
	c.buffer.Push(snapshot)
	c.latest = &snapshot
	c.mu.Unlock()

	// Notify subscribers
	c.subMu.RLock()
	for _, ch := range c.subs {
		select {
		case ch <- snapshot:
		default:
			// subscriber is slow, drop the snapshot
		}
	}
	c.subMu.RUnlock()
}

// Latest returns the most recent snapshot, or nil if no collection has occurred.
func (c *Collector) Latest() *Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// History returns all snapshots in the rolling buffer, oldest first.
func (c *Collector) History() []Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buffer.All()
}

// RuntimeInfo returns the cached runtime information.
func (c *Collector) RuntimeInfo() *runtime.RuntimeInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info
}

// Subscribe returns a channel that receives snapshots on each collection tick.
// Call Unsubscribe with the returned ID when done.
func (c *Collector) Subscribe() (string, <-chan Snapshot) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	id := fmt.Sprintf("sub-%d", c.nextID)
	c.nextID++

	ch := make(chan Snapshot, 1)
	c.subs[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber.
func (c *Collector) Unsubscribe(id string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	if ch, ok := c.subs[id]; ok {
		close(ch)
		delete(c.subs, id)
	}
}
