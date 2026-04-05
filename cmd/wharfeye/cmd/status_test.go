package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mathesh-me/wharfeye/internal/models"
	"github.com/mathesh-me/wharfeye/internal/runtime"
)

type statusTestClient struct {
	containers []models.Container
	detail     *models.ContainerDetail
	inspectErr error
}

func (c *statusTestClient) Info(context.Context) (*runtime.RuntimeInfo, error) {
	return &runtime.RuntimeInfo{Name: "docker", Version: "test"}, nil
}

func (c *statusTestClient) ListContainers(context.Context) ([]models.Container, error) {
	return c.containers, nil
}

func (c *statusTestClient) InspectContainer(context.Context, string) (*models.ContainerDetail, error) {
	if c.inspectErr != nil {
		return nil, c.inspectErr
	}
	return c.detail, nil
}

func (c *statusTestClient) ContainerStats(context.Context, string) (*models.ContainerStats, error) {
	return &models.ContainerStats{}, nil
}

func (c *statusTestClient) StreamStats(context.Context, string) (<-chan *models.ContainerStats, error) {
	return nil, nil
}

func (c *statusTestClient) ListImages(context.Context) ([]models.Image, error) {
	return nil, nil
}

func (c *statusTestClient) InspectImage(context.Context, string) (*models.ImageDetail, error) {
	return nil, nil
}

func (c *statusTestClient) ListVolumes(context.Context) ([]models.Volume, error) {
	return nil, nil
}

func (c *statusTestClient) StopContainer(context.Context, string) error {
	return nil
}

func (c *statusTestClient) RestartContainer(context.Context, string) error {
	return nil
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w

	outCh := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, _ = io.Copy(&b, r)
		outCh <- b.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = origStdout
	return <-outCh
}

func TestRunStatusSingleStoppedContainerFallsBackToSummary(t *testing.T) {
	summary := models.Container{
		ID:      "abc1234567890",
		Name:    "wharfeye-medium",
		Image:   "alpine:3.20",
		Status:  "Exited (137) 10 hours ago",
		State:   "exited",
		Runtime: "docker",
		Created: time.Date(2026, 4, 2, 6, 39, 10, 0, time.UTC),
	}
	client := &statusTestClient{
		containers: []models.Container{summary},
		inspectErr: errors.New("inspect failed"),
	}

	prevFormat := formatFlag
	formatFlag = "table"
	defer func() { formatFlag = prevFormat }()

	output := captureStdout(t, func() {
		if err := runStatusSingle(context.Background(), client, summary.Name); err != nil {
			t.Fatalf("runStatusSingle returned error: %v", err)
		}
	})

	if !strings.Contains(output, summary.Status) {
		t.Fatalf("expected output to contain summary status %q, got %q", summary.Status, output)
	}
	if !strings.Contains(output, "Runtime: docker") {
		t.Fatalf("expected output to contain runtime fallback, got %q", output)
	}
}

func TestMergeContainerDetailUsesSummaryFields(t *testing.T) {
	summary := models.Container{
		ID:      "abc1234567890",
		Name:    "wharfeye-medium",
		Image:   "alpine:3.20",
		Status:  "Exited (137) 10 hours ago",
		State:   "exited",
		Runtime: "docker",
		Created: time.Date(2026, 4, 2, 6, 39, 10, 0, time.UTC),
	}
	detail := &models.ContainerDetail{
		Container: models.Container{
			ID:     summary.ID,
			Name:   summary.Name,
			Image:  summary.Image,
			Status: "exited",
			State:  "exited",
		},
	}

	merged := mergeContainerDetail(detail, summary)

	if merged.Status != summary.Status {
		t.Fatalf("expected merged status %q, got %q", summary.Status, merged.Status)
	}
	if merged.Runtime != summary.Runtime {
		t.Fatalf("expected runtime %q, got %q", summary.Runtime, merged.Runtime)
	}
	if !merged.Created.Equal(summary.Created) {
		t.Fatalf("expected created time %v, got %v", summary.Created, merged.Created)
	}
}
