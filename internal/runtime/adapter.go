package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mathesh-me/wharfeye/internal/models"
)

// RuntimeInfo holds metadata about the detected container runtime.
type RuntimeInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	StorageDriver string `json:"storage_driver"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	SocketPath    string `json:"socket_path"`
}

// Client is the contract every container runtime must implement.
type Client interface {
	// Info returns metadata about the container runtime.
	Info(ctx context.Context) (*RuntimeInfo, error)

	// ListContainers returns all containers (running and stopped).
	ListContainers(ctx context.Context) ([]models.Container, error)

	// InspectContainer returns detailed info for a single container.
	InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error)

	// ContainerStats returns point-in-time resource usage for a container.
	ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error)

	// StreamStats returns a channel of stats updates for a container.
	StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error)

	// ListImages returns all images.
	ListImages(ctx context.Context) ([]models.Image, error)

	// InspectImage returns detailed info for a single image.
	InspectImage(ctx context.Context, id string) (*models.ImageDetail, error)

	// ListVolumes returns all volumes.
	ListVolumes(ctx context.Context) ([]models.Volume, error)

	// StopContainer stops a running container.
	StopContainer(ctx context.Context, id string) error

	// RestartContainer restarts a container.
	RestartContainer(ctx context.Context, id string) error
}

// socketCandidate describes a socket path and its associated runtime.
type socketCandidate struct {
	path        string
	runtimeName string
}

// socketCandidates returns the ordered list of socket paths to probe.
func socketCandidates() []socketCandidate {
	candidates := []socketCandidate{
		{path: "/var/run/docker.sock", runtimeName: "docker"},
		{path: "/run/podman/podman.sock", runtimeName: "podman"},
	}

	// Podman rootless socket - check current user's XDG_RUNTIME_DIR
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime != "" {
		candidates = append(candidates, socketCandidate{
			path:        filepath.Join(xdgRuntime, "podman", "podman.sock"),
			runtimeName: "podman",
		})
	}

	// When running as root (e.g. sudo), also probe rootless Podman sockets
	// for real users under /run/user/<uid>/podman/podman.sock
	if os.Getuid() == 0 {
		matches, _ := filepath.Glob("/run/user/*/podman/podman.sock")
		for _, m := range matches {
			candidates = append(candidates, socketCandidate{
				path:        m,
				runtimeName: "podman",
			})
		}
	}

	candidates = append(candidates, socketCandidate{
		path:        "/run/containerd/containerd.sock",
		runtimeName: "containerd",
	})

	return candidates
}

// socketExists checks if a Unix socket exists at the given path.
// This is a variable so tests can override it.
var socketExists = func(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Type()&os.ModeSocket != 0
}

// NewClient creates a runtime client for the given runtime type and socket path.
func NewClient(runtimeType, socketPath string) (Client, error) {
	switch runtimeType {
	case "docker":
		return NewDockerClient(socketPath)
	case "podman":
		return NewPodmanClient(socketPath)
	case "containerd":
		return NewContainerdClient(socketPath)
	default:
		return nil, fmt.Errorf("unsupported runtime type: %s", runtimeType)
	}
}

// DetectedRuntime holds info about a runtime found on the system.
type DetectedRuntime struct {
	Name   string
	Socket string
}

// DetectAll probes all known socket paths and returns info about every
// available runtime on the system. Multiple sockets for the same runtime
// (e.g. rootful and rootless Podman) are returned as separate entries.
func DetectAll() []DetectedRuntime {
	var found []DetectedRuntime
	seen := make(map[string]bool) // keyed by socket path to avoid duplicates
	for _, c := range socketCandidates() {
		if socketExists(c.path) && !seen[c.path] {
			seen[c.path] = true
			found = append(found, DetectedRuntime{
				Name:   c.runtimeName,
				Socket: c.path,
			})
		}
	}
	return found
}

// InaccessibleSocket describes a socket that exists but the current user
// cannot access, along with instructions to fix it.
type InaccessibleSocket struct {
	Name   string
	Socket string
	Fix    string
}

// DetectInaccessible returns sockets that exist on the system but cannot be
// connected to by the current user, along with remediation instructions.
func DetectInaccessible() []InaccessibleSocket {
	var result []InaccessibleSocket
	seen := make(map[string]bool)

	fixMap := map[string]string{
		"docker":     "sudo usermod -aG docker $USER && newgrp docker",
		"podman":     "systemctl --user enable --now podman.socket",
		"containerd": "sudo groupadd -f containerd && sudo chgrp containerd /run/containerd/containerd.sock && sudo chmod 660 /run/containerd/containerd.sock && sudo usermod -aG containerd $USER && newgrp containerd",
	}

	for _, c := range socketCandidates() {
		if seen[c.path] {
			continue
		}
		seen[c.path] = true

		info, err := os.Stat(c.path)
		if err != nil || info.Mode().Type()&os.ModeSocket == 0 {
			continue
		}

		// Socket exists - try connecting to it
		client, err := NewClient(c.runtimeName, c.path)
		if err != nil {
			// Can't connect - permission or other error
			fix := fixMap[c.runtimeName]
			if fix == "" {
				fix = fmt.Sprintf("sudo chmod 666 %s", c.path)
			}
			result = append(result, InaccessibleSocket{
				Name:   c.runtimeName,
				Socket: c.path,
				Fix:    fix,
			})
			continue
		}

		// Client created - try a real operation to verify access
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, infoErr := client.Info(ctx)
		cancel()
		if infoErr != nil {
			fix := fixMap[c.runtimeName]
			if fix == "" {
				fix = fmt.Sprintf("sudo chmod 666 %s", c.path)
			}
			result = append(result, InaccessibleSocket{
				Name:   c.runtimeName,
				Socket: c.path,
				Fix:    fix,
			})
		}
	}
	return result
}

// AutoDetect probes known socket paths and returns a client for the first
// available container runtime. It checks Docker, Podman (rootful then rootless),
// and containerd in that order.
func AutoDetect(ctx context.Context) (Client, error) {
	candidates := socketCandidates()

	checked := make([]string, 0, len(candidates))
	for _, c := range candidates {
		checked = append(checked, c.path)
		if socketExists(c.path) {
			client, err := NewClient(c.runtimeName, c.path)
			if err != nil {
				return nil, fmt.Errorf("connecting to %s at %s: %w", c.runtimeName, c.path, err)
			}
			return client, nil
		}
	}

	return nil, fmt.Errorf("no container runtime found; checked sockets: %v", checked)
}

// Detect finds the runtime and returns both the client and runtime info.
func Detect(ctx context.Context, runtimeType, socketPath string) (Client, error) {
	// If runtime type is explicitly specified, use it
	if runtimeType != "" && runtimeType != "auto" {
		if socketPath == "" {
			// Find the default socket for this runtime
			for _, c := range socketCandidates() {
				if c.runtimeName == runtimeType && socketExists(c.path) {
					socketPath = c.path
					break
				}
			}
		}
		if socketPath == "" {
			return nil, fmt.Errorf("no socket found for runtime %q", runtimeType)
		}
		return NewClient(runtimeType, socketPath)
	}

	// If a custom socket is specified but no runtime type, try Docker-compatible first
	if socketPath != "" {
		client, err := NewDockerClient(socketPath)
		if err != nil {
			return nil, fmt.Errorf("connecting to socket %s: %w", socketPath, err)
		}
		return client, nil
	}

	// Auto-detect
	return AutoDetect(ctx)
}
