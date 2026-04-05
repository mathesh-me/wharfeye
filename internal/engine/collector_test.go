package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// mockClient implements runtime.Client for testing.
type mockClient struct {
	mu         sync.Mutex
	containers []models.Container
	stats      map[string]*models.ContainerStats
	images     []models.Image
	volumes    []models.Volume
	info       *runtime.RuntimeInfo
	callCount  int
}

func newMockClient() *mockClient {
	return &mockClient{
		info: &runtime.RuntimeInfo{
			Name:    "MockRuntime",
			Version: "1.0.0",
		},
		containers: []models.Container{
			{
				ID:    "abc123",
				Name:  "web-server",
				Image: "nginx:latest",
				State: "running",
				Status: "Up 2 hours",
			},
			{
				ID:    "def456",
				Name:  "database",
				Image: "postgres:16",
				State: "running",
				Status: "Up 1 day",
			},
			{
				ID:    "ghi789",
				Name:  "stopped-app",
				Image: "myapp:v1",
				State: "exited",
				Status: "Exited (0) 3 hours ago",
			},
		},
		stats: map[string]*models.ContainerStats{
			"abc123": {
				ContainerID: "abc123",
				CPU:         models.CPUStats{Percent: 2.5},
				Memory:      models.MemStats{Usage: 128 * 1024 * 1024, Limit: 512 * 1024 * 1024, Percent: 25.0},
				PIDs:        5,
			},
			"def456": {
				ContainerID: "def456",
				CPU:         models.CPUStats{Percent: 8.1},
				Memory:      models.MemStats{Usage: 512 * 1024 * 1024, Limit: 1024 * 1024 * 1024, Percent: 50.0},
				PIDs:        12,
			},
		},
		images:  []models.Image{{ID: "img1"}, {ID: "img2"}},
		volumes: []models.Volume{{Name: "vol1"}},
	}
}

func (m *mockClient) Info(ctx context.Context) (*runtime.RuntimeInfo, error) {
	return m.info, nil
}

func (m *mockClient) ListContainers(ctx context.Context) ([]models.Container, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return m.containers, nil
}

func (m *mockClient) InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error) {
	for _, ctr := range m.containers {
		if ctr.ID == id {
			return &models.ContainerDetail{
				Container: ctr,
				Config: models.ContainerConfig{
					User: "",
				},
				HostConfig: models.HostConfig{},
			}, nil
		}
	}
	return &models.ContainerDetail{}, nil
}

func (m *mockClient) ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error) {
	if s, ok := m.stats[id]; ok {
		s.Timestamp = time.Now()
		return s, nil
	}
	return &models.ContainerStats{ContainerID: id, Timestamp: time.Now()}, nil
}

func (m *mockClient) StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error) {
	return nil, nil
}

func (m *mockClient) ListImages(ctx context.Context) ([]models.Image, error) {
	return m.images, nil
}

func (m *mockClient) InspectImage(ctx context.Context, id string) (*models.ImageDetail, error) {
	return nil, nil
}

func (m *mockClient) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	return m.volumes, nil
}

func (m *mockClient) StopContainer(ctx context.Context, id string) error {
	return nil
}

func (m *mockClient) RestartContainer(ctx context.Context, id string) error {
	return nil
}

func TestCollector_NewCollector(t *testing.T) {
	client := newMockClient()
	c := NewCollector(client, 2*time.Second, 1*time.Hour)

	if c.interval != 2*time.Second {
		t.Errorf("expected interval 2s, got %v", c.interval)
	}
	if c.bufSize != 1800 { // 3600s / 2s = 1800
		t.Errorf("expected buffer size 1800, got %d", c.bufSize)
	}
}

func TestCollector_CollectOnce_ViaEngine(t *testing.T) {
	client := newMockClient()
	eng := New(client, DefaultConfig())

	snapshot, err := eng.CollectOnce(context.Background())
	if err != nil {
		t.Fatalf("CollectOnce error: %v", err)
	}

	if len(snapshot.Containers) != 3 {
		t.Errorf("expected 3 containers, got %d", len(snapshot.Containers))
	}

	if snapshot.Host.RunningContainers != 2 {
		t.Errorf("expected 2 running, got %d", snapshot.Host.RunningContainers)
	}
	if snapshot.Host.StoppedContainers != 1 {
		t.Errorf("expected 1 stopped, got %d", snapshot.Host.StoppedContainers)
	}

	if snapshot.Host.RuntimeName != "MockRuntime" {
		t.Errorf("expected runtime MockRuntime, got %s", snapshot.Host.RuntimeName)
	}

	// Verify stats were collected for running containers
	if len(snapshot.Stats) != 2 {
		t.Errorf("expected 2 stats entries, got %d", len(snapshot.Stats))
	}

	webStats, ok := snapshot.Stats["abc123"]
	if !ok {
		t.Fatal("expected stats for abc123")
	}
	if webStats.CPU.Percent != 2.5 {
		t.Errorf("expected CPU 2.5%%, got %.1f%%", webStats.CPU.Percent)
	}

	// Verify host aggregates
	expectedCPU := 2.5 + 8.1
	if snapshot.Host.TotalCPUPercent != expectedCPU {
		t.Errorf("expected total CPU %.1f%%, got %.1f%%", expectedCPU, snapshot.Host.TotalCPUPercent)
	}

	expectedMem := uint64((128 + 512) * 1024 * 1024)
	if snapshot.Host.TotalMemoryUsage != expectedMem {
		t.Errorf("expected total memory %d, got %d", expectedMem, snapshot.Host.TotalMemoryUsage)
	}

	if snapshot.Host.ImageCount != 2 {
		t.Errorf("expected 2 images, got %d", snapshot.Host.ImageCount)
	}
	if snapshot.Host.VolumeCount != 1 {
		t.Errorf("expected 1 volume, got %d", snapshot.Host.VolumeCount)
	}
}

func TestCollector_Run_CollectsMultipleTimes(t *testing.T) {
	client := newMockClient()
	c := NewCollector(client, 50*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	_ = c.Run(ctx)

	// Should have collected multiple times
	history := c.History()
	if len(history) < 2 {
		t.Errorf("expected at least 2 snapshots in history, got %d", len(history))
	}

	latest := c.Latest()
	if latest == nil {
		t.Fatal("expected non-nil latest snapshot")
	}
	if latest.Host.RuntimeName != "MockRuntime" {
		t.Errorf("expected MockRuntime, got %s", latest.Host.RuntimeName)
	}
}

func TestCollector_Subscribe(t *testing.T) {
	client := newMockClient()
	c := NewCollector(client, 50*time.Millisecond, 5*time.Second)

	id, ch := c.Subscribe()
	defer c.Unsubscribe(id)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() {
		_ = c.Run(ctx)
	}()

	// Should receive at least one snapshot
	select {
	case snap := <-ch:
		if snap.Host.RuntimeName != "MockRuntime" {
			t.Errorf("expected MockRuntime, got %s", snap.Host.RuntimeName)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for snapshot")
	}
}

func TestCollector_Unsubscribe(t *testing.T) {
	client := newMockClient()
	c := NewCollector(client, 100*time.Millisecond, 5*time.Second)

	id, ch := c.Subscribe()
	c.Unsubscribe(id)

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestCollector_HistoryOrder(t *testing.T) {
	client := newMockClient()
	c := NewCollector(client, 30*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_ = c.Run(ctx)

	history := c.History()
	if len(history) < 2 {
		t.Skip("not enough samples collected")
	}

	// Verify timestamps are in order (oldest first)
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Errorf("history not in order: [%d]=%v > [%d]=%v",
				i-1, history[i-1].Timestamp, i, history[i].Timestamp)
		}
	}
}

func TestEngine_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.CollectorInterval != 2*time.Second {
		t.Errorf("expected 2s interval, got %v", cfg.CollectorInterval)
	}
	if cfg.HistoryWindow != 1*time.Hour {
		t.Errorf("expected 1h window, got %v", cfg.HistoryWindow)
	}
}
