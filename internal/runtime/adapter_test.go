package runtime

import (
	"context"
	"testing"
)

func TestSocketCandidates(t *testing.T) {
	candidates := socketCandidates()

	if len(candidates) < 3 {
		t.Fatalf("expected at least 3 socket candidates, got %d", len(candidates))
	}

	// First candidate should be Docker
	if candidates[0].runtimeName != "docker" {
		t.Errorf("first candidate should be docker, got %s", candidates[0].runtimeName)
	}
	if candidates[0].path != "/var/run/docker.sock" {
		t.Errorf("docker socket path should be /var/run/docker.sock, got %s", candidates[0].path)
	}

	// Second should be Podman rootful
	if candidates[1].runtimeName != "podman" {
		t.Errorf("second candidate should be podman, got %s", candidates[1].runtimeName)
	}
	if candidates[1].path != "/run/podman/podman.sock" {
		t.Errorf("podman rootful path should be /run/podman/podman.sock, got %s", candidates[1].path)
	}

	// Last should be containerd
	last := candidates[len(candidates)-1]
	if last.runtimeName != "containerd" {
		t.Errorf("last candidate should be containerd, got %s", last.runtimeName)
	}
}

func TestAutoDetect_NoSockets(t *testing.T) {
	// Override socketExists to return false for all paths
	original := socketExists
	socketExists = func(path string) bool { return false }
	defer func() { socketExists = original }()

	_, err := AutoDetect(context.Background())
	if err == nil {
		t.Fatal("expected error when no sockets available")
	}

	if !containsStr(err.Error(), "no container runtime found") {
		t.Errorf("error should mention no runtime found, got: %s", err.Error())
	}
}

func TestAutoDetect_DockerFirst(t *testing.T) {
	original := socketExists
	socketExists = func(path string) bool {
		return path == "/var/run/docker.sock"
	}
	defer func() { socketExists = original }()

	// AutoDetect will try to create a Docker client.
	// Since there's no actual Docker socket, we just verify it tries Docker.
	client, err := AutoDetect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The client should be a DockerClient
	if _, ok := client.(*DockerClient); !ok {
		t.Errorf("expected DockerClient, got %T", client)
	}
}

func TestAutoDetect_PodmanRootful(t *testing.T) {
	original := socketExists
	socketExists = func(path string) bool {
		return path == "/run/podman/podman.sock"
	}
	defer func() { socketExists = original }()

	client, err := AutoDetect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := client.(*PodmanClient); !ok {
		t.Errorf("expected PodmanClient, got %T", client)
	}
}

func TestAutoDetect_Containerd(t *testing.T) {
	original := socketExists
	socketExists = func(path string) bool {
		return path == "/run/containerd/containerd.sock"
	}
	defer func() { socketExists = original }()

	client, err := AutoDetect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := client.(*ContainerdClient); !ok {
		t.Errorf("expected ContainerdClient, got %T", client)
	}
}

func TestAutoDetect_PriorityOrder(t *testing.T) {
	// When both Docker and Podman are available, Docker should win
	original := socketExists
	socketExists = func(path string) bool {
		return path == "/var/run/docker.sock" || path == "/run/podman/podman.sock"
	}
	defer func() { socketExists = original }()

	client, err := AutoDetect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := client.(*DockerClient); !ok {
		t.Errorf("expected DockerClient (priority), got %T", client)
	}
}

func TestNewClient_UnsupportedRuntime(t *testing.T) {
	_, err := NewClient("kubernetes", "/some/path")
	if err == nil {
		t.Fatal("expected error for unsupported runtime")
	}
}

func TestDetect_ExplicitRuntime(t *testing.T) {
	original := socketExists
	socketExists = func(path string) bool {
		return path == "/run/podman/podman.sock"
	}
	defer func() { socketExists = original }()

	client, err := Detect(context.Background(), "podman", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := client.(*PodmanClient); !ok {
		t.Errorf("expected PodmanClient, got %T", client)
	}
}

func TestDetect_ExplicitSocket(t *testing.T) {
	client, err := Detect(context.Background(), "", "/var/run/docker.sock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := client.(*DockerClient); !ok {
		t.Errorf("expected DockerClient for explicit socket, got %T", client)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
