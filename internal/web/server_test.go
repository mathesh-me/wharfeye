package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// testClient implements runtime.Client for web handler tests.
type testClient struct {
	containers []models.Container
	stats      map[string]*models.ContainerStats
}

func newTestClient() *testClient {
	return &testClient{
		containers: []models.Container{
			{ID: "abc123", Name: "web", Image: "nginx:latest", State: "running", Status: "Up 1h"},
			{ID: "def456", Name: "db", Image: "postgres:16", State: "running", Status: "Up 2h"},
		},
		stats: map[string]*models.ContainerStats{
			"abc123": {
				ContainerID: "abc123",
				CPU:         models.CPUStats{Percent: 5.0},
				Memory:      models.MemStats{Usage: 64 * 1024 * 1024, Limit: 256 * 1024 * 1024, Percent: 25.0},
			},
			"def456": {
				ContainerID: "def456",
				CPU:         models.CPUStats{Percent: 12.0},
				Memory:      models.MemStats{Usage: 256 * 1024 * 1024, Limit: 1024 * 1024 * 1024, Percent: 25.0},
			},
		},
	}
}

func (c *testClient) Info(ctx context.Context) (*runtime.RuntimeInfo, error) {
	return &runtime.RuntimeInfo{Name: "TestRuntime", Version: "1.0"}, nil
}

func (c *testClient) ListContainers(ctx context.Context) ([]models.Container, error) {
	return c.containers, nil
}

func (c *testClient) InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error) {
	for _, ctr := range c.containers {
		if ctr.ID == id {
			return &models.ContainerDetail{Container: ctr}, nil
		}
	}
	return nil, &notFoundError{id: id}
}

func (c *testClient) ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error) {
	if s, ok := c.stats[id]; ok {
		return s, nil
	}
	return &models.ContainerStats{ContainerID: id, Timestamp: time.Now()}, nil
}

func (c *testClient) StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error) {
	return nil, nil
}

func (c *testClient) ListImages(ctx context.Context) ([]models.Image, error) {
	return []models.Image{{ID: "img1", Tags: []string{"nginx:latest"}}}, nil
}

func (c *testClient) InspectImage(ctx context.Context, id string) (*models.ImageDetail, error) {
	return nil, nil
}

func (c *testClient) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	return []models.Volume{{Name: "vol1", InUse: true}}, nil
}

func (c *testClient) StopContainer(ctx context.Context, id string) error  { return nil }
func (c *testClient) RestartContainer(ctx context.Context, id string) error { return nil }

type notFoundError struct{ id string }

func (e *notFoundError) Error() string { return "not found: " + e.id }

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	client := newTestClient()
	eng := engine.New(client, engine.Config{
		CollectorInterval: 50 * time.Millisecond,
		HistoryWindow:     5 * time.Second,
	})

	// Run collector briefly to populate a snapshot
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = eng.Collector.Run(ctx)

	scanner := engine.NewScanner(client)
	return NewServer(eng, scanner, client)
}

func TestHandleSnapshot(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	w := httptest.NewRecorder()
	srv.handleSnapshot(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var snap engine.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(snap.Containers) != 2 {
		t.Errorf("expected 2 containers, got %d", len(snap.Containers))
	}
	if snap.Host.RuntimeName != "TestRuntime" {
		t.Errorf("expected TestRuntime, got %s", snap.Host.RuntimeName)
	}
}

func TestHandleSnapshot_NoData(t *testing.T) {
	client := newTestClient()
	eng := engine.New(client, engine.DefaultConfig())
	srv := NewServer(eng, nil, client)

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	w := httptest.NewRecorder()
	srv.handleSnapshot(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no data, got %d", w.Code)
	}
}

func TestHandleContainerDetail(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/containers/abc123", nil)
	req.SetPathValue("id", "abc123")
	w := httptest.NewRecorder()
	srv.handleContainerDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	detail, ok := resp["detail"].(map[string]any)
	if !ok {
		t.Fatal("expected detail field in response")
	}
	if detail["name"] != "web" {
		t.Errorf("expected container name 'web', got %v", detail["name"])
	}
}

func TestHandleContainerDetail_NotFound(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/containers/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	srv.handleContainerDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown container, got %d", w.Code)
	}
}

func TestHandleRecommendations(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/recommendations", nil)
	w := httptest.NewRecorder()
	srv.handleRecommendations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var report models.AdvisorReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
}

func TestHandleSecurity(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/security", nil)
	w := httptest.NewRecorder()
	srv.handleSecurity(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var report models.FleetSecurityReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(report.Containers) != 2 {
		t.Errorf("expected 2 container reports, got %d", len(report.Containers))
	}
}

func TestHandleSecurity_NoScanner(t *testing.T) {
	client := newTestClient()
	eng := engine.New(client, engine.DefaultConfig())
	srv := NewServer(eng, nil, client)
	// Set scanner to nil
	srv.scanner = nil

	req := httptest.NewRequest(http.MethodGet, "/api/security", nil)
	w := httptest.NewRecorder()
	srv.handleSecurity(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no scanner, got %d", w.Code)
	}
}

func TestNewServer(t *testing.T) {
	client := newTestClient()
	eng := engine.New(client, engine.DefaultConfig())
	scanner := engine.NewScanner(client)

	srv := NewServer(eng, scanner, client)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.advisor == nil {
		t.Error("expected advisor to be initialized")
	}
	if len(srv.clients) != 0 {
		t.Error("expected empty clients map")
	}
}
