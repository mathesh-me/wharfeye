package runtime

import (
	"context"
	"os"
	"testing"
)

// Integration tests require a running Docker daemon.
// They are skipped if no Docker socket is available.

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("skipping: Docker socket not available")
	}
}

func TestDockerIntegration_Info(t *testing.T) {
	skipIfNoDocker(t)

	client, err := NewDockerClient("/var/run/docker.sock")
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	info, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("getting info: %v", err)
	}

	if info.Name != "Docker" {
		t.Errorf("expected runtime name 'Docker', got %q", info.Name)
	}
	if info.Version == "" {
		t.Error("expected non-empty version")
	}
}

func TestDockerIntegration_ListContainers(t *testing.T) {
	skipIfNoDocker(t)

	client, err := NewDockerClient("/var/run/docker.sock")
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	containers, err := client.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("listing containers: %v", err)
	}

	// Just verify it doesn't error - there may be 0 containers
	t.Logf("found %d containers", len(containers))
}

func TestDockerIntegration_ListImages(t *testing.T) {
	skipIfNoDocker(t)

	client, err := NewDockerClient("/var/run/docker.sock")
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	images, err := client.ListImages(context.Background())
	if err != nil {
		t.Fatalf("listing images: %v", err)
	}

	t.Logf("found %d images", len(images))
}

func TestDockerIntegration_ListVolumes(t *testing.T) {
	skipIfNoDocker(t)

	client, err := NewDockerClient("/var/run/docker.sock")
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	volumes, err := client.ListVolumes(context.Background())
	if err != nil {
		t.Fatalf("listing volumes: %v", err)
	}

	t.Logf("found %d volumes", len(volumes))
}
