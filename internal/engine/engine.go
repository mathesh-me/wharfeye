package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// Engine orchestrates all subsystems: collector, scanner, and advisor.
// In Phase 2, only the collector is active.
type Engine struct {
	Client    runtime.Client
	Collector *Collector
}

// Config holds engine configuration.
type Config struct {
	CollectorInterval time.Duration
	HistoryWindow     time.Duration
}

// DefaultConfig returns sensible default engine configuration.
func DefaultConfig() Config {
	return Config{
		CollectorInterval: 2 * time.Second,
		HistoryWindow:     1 * time.Hour,
	}
}

// New creates a new engine with the given runtime client and configuration.
func New(client runtime.Client, cfg Config) *Engine {
	return &Engine{
		Client:    client,
		Collector: NewCollector(client, cfg.CollectorInterval, cfg.HistoryWindow),
	}
}

// Start launches all engine subsystems. It blocks until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	slog.Info("starting engine", "interval", e.Collector.interval)

	errCh := make(chan error, 1)

	go func() {
		if err := e.Collector.Run(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("collector stopped: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// CollectOnce performs a single collection and returns the snapshot.
// Useful for one-shot CLI commands like `wharfeye status`.
func (e *Engine) CollectOnce(ctx context.Context) (*Snapshot, error) {
	// Get runtime info
	info, err := e.Client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting runtime info: %w", err)
	}

	containers, err := e.Client.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
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
			s, err := e.Client.ContainerStats(ctx, ctr.ID)
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

	imageCount := 0
	volumeCount := 0
	if images, err := e.Client.ListImages(ctx); err == nil {
		imageCount = len(images)
	}
	if volumes, err := e.Client.ListVolumes(ctx); err == nil {
		volumeCount = len(volumes)
	}

	return &Snapshot{
		Timestamp:  time.Now(),
		Containers: containers,
		Stats:      stats,
		Host: HostSummary{
			RuntimeName:       info.Name,
			RuntimeVersion:    info.Version,
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
	}, nil
}
