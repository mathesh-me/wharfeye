package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mathesh-me/wharfeye/internal/models"
)

// PodmanClient implements Client for Podman by wrapping the Docker-compatible API.
// Podman exposes a Docker-compatible REST API, so most operations are identical.
type PodmanClient struct {
	docker     *DockerClient
	socketPath string
}

// NewPodmanClient creates a new Podman runtime client.
// Since Podman exposes a Docker-compatible API, we reuse the Docker client.
func NewPodmanClient(socketPath string) (*PodmanClient, error) {
	docker, err := NewDockerClient(socketPath)
	if err != nil {
		return nil, fmt.Errorf("creating podman client: %w", err)
	}

	return &PodmanClient{
		docker:     docker,
		socketPath: socketPath,
	}, nil
}

// DetectPodmanSocket returns the first available Podman socket path.
func DetectPodmanSocket() string {
	// Rootful socket
	if socketExists("/run/podman/podman.sock") {
		return "/run/podman/podman.sock"
	}

	// Rootless socket
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime != "" {
		rootless := filepath.Join(xdgRuntime, "podman", "podman.sock")
		if socketExists(rootless) {
			return rootless
		}
	}

	return ""
}

func (p *PodmanClient) Info(ctx context.Context) (*RuntimeInfo, error) {
	info, err := p.docker.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting podman info: %w", err)
	}

	// Override the name to indicate Podman
	info.Name = "Podman"
	info.SocketPath = p.socketPath

	return info, nil
}

func (p *PodmanClient) ListContainers(ctx context.Context) ([]models.Container, error) {
	containers, err := p.docker.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	// Tag containers with podman runtime
	for i := range containers {
		containers[i].Runtime = "podman"
	}

	return containers, nil
}

func (p *PodmanClient) InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error) {
	detail, err := p.docker.InspectContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.Runtime = "podman"
	return detail, nil
}

func (p *PodmanClient) ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error) {
	return p.docker.ContainerStats(ctx, id)
}

func (p *PodmanClient) StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error) {
	return p.docker.StreamStats(ctx, id)
}

func (p *PodmanClient) ListImages(ctx context.Context) ([]models.Image, error) {
	return p.docker.ListImages(ctx)
}

func (p *PodmanClient) InspectImage(ctx context.Context, id string) (*models.ImageDetail, error) {
	return p.docker.InspectImage(ctx, id)
}

func (p *PodmanClient) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	return p.docker.ListVolumes(ctx)
}

func (p *PodmanClient) StopContainer(ctx context.Context, id string) error {
	return p.docker.StopContainer(ctx, id)
}

func (p *PodmanClient) RestartContainer(ctx context.Context, id string) error {
	return p.docker.RestartContainer(ctx, id)
}
