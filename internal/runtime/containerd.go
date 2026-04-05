package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	typeurl "github.com/containerd/typeurl/v2"

	"github.com/mathesh-me/wharfeye/internal/models"
)

// ContainerdClient implements Client for containerd using the containerd v2 SDK.
type ContainerdClient struct {
	client     *containerd.Client
	socketPath string
	namespace  string
}

// NewContainerdClient creates a new containerd runtime client.
func NewContainerdClient(socketPath string) (*ContainerdClient, error) {
	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to containerd at %s: %w", socketPath, err)
	}

	return &ContainerdClient{
		client:     client,
		socketPath: socketPath,
		namespace:  "default",
	}, nil
}

// nsCtx returns a context with the containerd namespace set.
func (c *ContainerdClient) nsCtx(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, c.namespace)
}

func (c *ContainerdClient) Info(ctx context.Context) (*RuntimeInfo, error) {
	ctx = c.nsCtx(ctx)
	version, err := c.client.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting containerd version: %w", err)
	}

	return &RuntimeInfo{
		Name:       "containerd",
		Version:    version.Version,
		SocketPath: c.socketPath,
	}, nil
}

func (c *ContainerdClient) ListContainers(ctx context.Context) ([]models.Container, error) {
	ctx = c.nsCtx(ctx)
	ctrs, err := c.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containerd containers: %w", err)
	}

	result := make([]models.Container, 0, len(ctrs))
	for _, ctr := range ctrs {
		info, err := ctr.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			slog.Warn("failed to get container info", "id", ctr.ID(), "error", err)
			continue
		}

		state := "created"
		status := "Created"

		task, err := ctr.Task(ctx, nil)
		if err == nil {
			taskStatus, err := task.Status(ctx)
			if err == nil {
				state = string(taskStatus.Status)
				status = formatContainerdStatus(taskStatus.Status, taskStatus.ExitTime)
			}
		}

		name := containerName(info)

		result = append(result, models.Container{
			ID:      ctr.ID(),
			Name:    name,
			Image:   info.Image,
			ImageID: info.Image,
			Status:  status,
			State:   state,
			Created: info.CreatedAt,
			Labels:  info.Labels,
			Ports:   []models.Port{},
			Mounts:  containerdMounts(info),
			Runtime: "containerd",
		})
	}

	return result, nil
}

func (c *ContainerdClient) InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error) {
	ctx = c.nsCtx(ctx)
	ctr, err := c.client.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("loading container %s: %w", id, err)
	}

	info, err := ctr.Info(ctx, containerd.WithoutRefreshedMetadata)
	if err != nil {
		return nil, fmt.Errorf("inspecting container %s: %w", id, err)
	}

	state := "created"
	status := "Created"
	task, err := ctr.Task(ctx, nil)
	if err == nil {
		taskStatus, err := task.Status(ctx)
		if err == nil {
			state = string(taskStatus.Status)
			status = formatContainerdStatus(taskStatus.Status, taskStatus.ExitTime)
		}
	}

	name := containerName(info)

	spec, err := ctr.Spec(ctx)

	var user string
	var env []string
	var cmd []string
	var workingDir string
	if spec != nil && spec.Process != nil {
		user = spec.Process.User.Username
		env = spec.Process.Env
		cmd = spec.Process.Args
		workingDir = spec.Process.Cwd
	}

	detail := &models.ContainerDetail{
		Container: models.Container{
			ID:      ctr.ID(),
			Name:    name,
			Image:   info.Image,
			ImageID: info.Image,
			Status:  status,
			State:   state,
			Created: info.CreatedAt,
			Labels:  info.Labels,
			Ports:   []models.Port{},
			Mounts:  containerdMounts(info),
			Runtime: "containerd",
		},
		Config: models.ContainerConfig{
			User:       user,
			Env:        env,
			Cmd:        cmd,
			WorkingDir: workingDir,
			Labels:     info.Labels,
		},
		HostConfig: models.HostConfig{},
	}

	// Extract security-relevant info from spec
	if spec != nil && spec.Process != nil {
		if spec.Process.User.UID == 0 {
			detail.Config.User = "root"
		}
	}
	if spec != nil && spec.Linux != nil {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Type == "pid" && ns.Path != "" {
				detail.HostConfig.PidMode = "host"
			}
		}
		if spec.Linux.Resources != nil && spec.Linux.Resources.Memory != nil && spec.Linux.Resources.Memory.Limit != nil {
			detail.HostConfig.MemoryLimit = *spec.Linux.Resources.Memory.Limit
		}
		if spec.Linux.Resources != nil && spec.Linux.Resources.CPU != nil {
			if spec.Linux.Resources.CPU.Quota != nil {
				detail.HostConfig.CPUQuota = *spec.Linux.Resources.CPU.Quota
			}
			if spec.Linux.Resources.CPU.Shares != nil {
				detail.HostConfig.CPUShares = int64(*spec.Linux.Resources.CPU.Shares)
			}
		}
	}

	_ = err // spec error is non-fatal

	return detail, nil
}

func (c *ContainerdClient) ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error) {
	ctx = c.nsCtx(ctx)
	ctr, err := c.client.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("loading container %s: %w", id, err)
	}

	task, err := ctr.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting task for container %s: %w", id, err)
	}

	metric, err := task.Metrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting metrics for container %s: %w", id, err)
	}

	stats := &models.ContainerStats{
		ContainerID: id,
		Timestamp:   time.Now(),
	}

	// The metrics come as a protobuf Any message.
	// We need to unmarshal it to get cgroup stats.
	data, err := typeurl.UnmarshalAny(metric.Data)
	if err != nil {
		slog.Warn("failed to unmarshal containerd metrics", "container", id, "error", err)
		return stats, nil
	}

	parseCgroupStats(stats, data)

	return stats, nil
}

func (c *ContainerdClient) StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error) {
	ch := make(chan *models.ContainerStats)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s, err := c.ContainerStats(ctx, id)
				if err != nil {
					slog.Error("streaming containerd stats", "container", id, "error", err)
					return
				}
				select {
				case ch <- s:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func (c *ContainerdClient) ListImages(ctx context.Context) ([]models.Image, error) {
	ctx = c.nsCtx(ctx)
	images, err := c.client.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containerd images: %w", err)
	}

	result := make([]models.Image, 0, len(images))
	for _, img := range images {
		target := img.Target()
		size, _ := img.Size(ctx)

		tags := []string{img.Name()}
		digest := target.Digest.String()

		result = append(result, models.Image{
			ID:      digest,
			Tags:    tags,
			Size:    size,
			Created: img.Metadata().CreatedAt,
			Digest:  digest,
		})
	}

	return result, nil
}

func (c *ContainerdClient) InspectImage(ctx context.Context, id string) (*models.ImageDetail, error) {
	ctx = c.nsCtx(ctx)
	img, err := c.client.GetImage(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting containerd image %s: %w", id, err)
	}

	target := img.Target()
	size, _ := img.Size(ctx)
	digest := target.Digest.String()

	return &models.ImageDetail{
		Image: models.Image{
			ID:      digest,
			Tags:    []string{img.Name()},
			Size:    size,
			Created: img.Metadata().CreatedAt,
			Digest:  digest,
		},
	}, nil
}

func (c *ContainerdClient) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	// containerd doesn't have a native volume concept like Docker.
	// Return empty list.
	return []models.Volume{}, nil
}

func (c *ContainerdClient) StopContainer(ctx context.Context, id string) error {
	ctx = c.nsCtx(ctx)
	ctr, err := c.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("loading container %s: %w", id, err)
	}

	task, err := ctr.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil // no task, already stopped
		}
		return fmt.Errorf("getting task for container %s: %w", id, err)
	}

	if err := task.Kill(ctx, 15); err != nil { // SIGTERM
		return fmt.Errorf("killing container %s: %w", id, err)
	}

	return nil
}

func (c *ContainerdClient) RestartContainer(ctx context.Context, id string) error {
	// containerd doesn't have a native restart - stop then start
	if err := c.StopContainer(ctx, id); err != nil {
		return fmt.Errorf("stopping container for restart %s: %w", id, err)
	}
	// Re-creating the task is complex in containerd; return a helpful error
	return fmt.Errorf("containerd restart requires re-creating the task; use your orchestrator to restart container %s", id)
}

// containerName extracts a human-readable name from container info.
func containerName(info containers.Container) string {
	if name, ok := info.Labels["io.kubernetes.container.name"]; ok {
		return name
	}
	if name, ok := info.Labels["nerdctl/name"]; ok {
		return name
	}
	// Use first 12 chars of ID as fallback
	id := info.ID
	if len(id) > 12 {
		id = id[:12]
	}
	return id
}

// containerdMounts converts containerd snapshot/mounts info to our model.
func containerdMounts(info containers.Container) []models.Mount {
	// containerd doesn't expose mounts the same way Docker does
	// in the container info. Mounts are part of the OCI spec.
	return []models.Mount{}
}

// formatContainerdStatus formats a containerd task status into a human-readable string.
func formatContainerdStatus(status containerd.ProcessStatus, exitTime time.Time) string {
	switch status {
	case containerd.Running:
		return "Up"
	case containerd.Stopped:
		if !exitTime.IsZero() {
			since := time.Since(exitTime).Truncate(time.Second)
			return fmt.Sprintf("Exited (%s ago)", since)
		}
		return "Exited"
	case containerd.Paused:
		return "Paused"
	default:
		s := string(status)
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return s
	}
}
