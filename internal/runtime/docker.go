package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"

	"github.com/mathesh-me/wharfeye/internal/models"
)

// DockerClient implements Client for Docker.
type DockerClient struct {
	cli        *dockerclient.Client
	socketPath string
}

// NewDockerClient creates a new Docker runtime client.
func NewDockerClient(socketPath string) (*DockerClient, error) {
	opts := []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
	}
	if socketPath != "" {
		opts = append(opts, dockerclient.WithHost("unix://"+socketPath))
	}

	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &DockerClient{
		cli:        cli,
		socketPath: socketPath,
	}, nil
}

func (d *DockerClient) Info(ctx context.Context) (*RuntimeInfo, error) {
	info, err := d.cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting docker info: %w", err)
	}

	version, err := d.cli.ServerVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting docker version: %w", err)
	}

	return &RuntimeInfo{
		Name:          "Docker",
		Version:       version.Version,
		StorageDriver: info.Driver,
		OS:            info.OSType,
		Arch:          info.Architecture,
		SocketPath:    d.socketPath,
	}, nil
}

func (d *DockerClient) ListContainers(ctx context.Context) ([]models.Container, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	result := make([]models.Container, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		ports := make([]models.Port, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, models.Port{
				IP:            p.IP,
				HostPort:      p.PublicPort,
				ContainerPort: p.PrivatePort,
				Protocol:      p.Type,
			})
		}

		mounts := make([]models.Mount, 0, len(c.Mounts))
		for _, m := range c.Mounts {
			mounts = append(mounts, models.Mount{
				Source:      m.Source,
				Destination: m.Destination,
				Mode:        m.Mode,
				RW:          m.RW,
			})
		}

		result = append(result, models.Container{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			ImageID: c.ImageID,
			Status:  c.Status,
			State:   c.State,
			Created: time.Unix(c.Created, 0),
			Ports:   ports,
			Labels:  c.Labels,
			Mounts:  mounts,
			Runtime: "docker",
		})
	}

	return result, nil
}

func (d *DockerClient) InspectContainer(ctx context.Context, id string) (*models.ContainerDetail, error) {
	inspect, err := d.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspecting container %s: %w", id, err)
	}

	ports := make([]models.Port, 0)
	for port, bindings := range inspect.NetworkSettings.Ports {
		for _, b := range bindings {
			hp := uint16(0)
			if b.HostPort != "" {
				fmt.Sscanf(b.HostPort, "%d", &hp)
			}
			ports = append(ports, models.Port{
				IP:            b.HostIP,
				HostPort:      hp,
				ContainerPort: uint16(port.Int()),
				Protocol:      port.Proto(),
			})
		}
	}

	mounts := make([]models.Mount, 0, len(inspect.Mounts))
	for _, m := range inspect.Mounts {
		mounts = append(mounts, models.Mount{
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			RW:          m.RW,
		})
	}

	name := inspect.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	var healthcheck *models.Healthcheck
	if inspect.Config.Healthcheck != nil {
		healthcheck = &models.Healthcheck{
			Test:     inspect.Config.Healthcheck.Test,
			Interval: inspect.Config.Healthcheck.Interval,
			Timeout:  inspect.Config.Healthcheck.Timeout,
			Retries:  inspect.Config.Healthcheck.Retries,
		}
	}

	restartPolicy := ""
	if inspect.HostConfig != nil {
		restartPolicy = string(inspect.HostConfig.RestartPolicy.Name)
	}

	detail := &models.ContainerDetail{
		Container: models.Container{
			ID:      inspect.ID,
			Name:    name,
			Image:   inspect.Config.Image,
			ImageID: inspect.Image,
			Status:  inspect.State.Status,
			State:   inspect.State.Status,
			Created: parseTime(inspect.Created),
			Ports:   ports,
			Labels:  inspect.Config.Labels,
			Mounts:  mounts,
			Runtime: "docker",
		},
		Config: models.ContainerConfig{
			Hostname:    inspect.Config.Hostname,
			User:        inspect.Config.User,
			Env:         inspect.Config.Env,
			Cmd:         inspect.Config.Cmd,
			Entrypoint:  inspect.Config.Entrypoint,
			WorkingDir:  inspect.Config.WorkingDir,
			Labels:      inspect.Config.Labels,
			Healthcheck: healthcheck,
		},
		HostConfig: models.HostConfig{
			Privileged:        inspect.HostConfig.Privileged,
			PidMode:           string(inspect.HostConfig.PidMode),
			IpcMode:           string(inspect.HostConfig.IpcMode),
			UTSMode:           string(inspect.HostConfig.UTSMode),
			ReadonlyRootfs:    inspect.HostConfig.ReadonlyRootfs,
			CapAdd:            inspect.HostConfig.CapAdd,
			CapDrop:           inspect.HostConfig.CapDrop,
			SecurityOpt:       inspect.HostConfig.SecurityOpt,
			MemoryLimit:       inspect.HostConfig.Memory,
			CPUShares:         inspect.HostConfig.CPUShares,
			CPUQuota:          inspect.HostConfig.CPUQuota,
			NanoCPUs:          inspect.HostConfig.NanoCPUs,
			PidsLimit:         pidsLimit(inspect.HostConfig.PidsLimit),
			RestartPolicy:     restartPolicy,
			RestartMaxRetries: inspect.HostConfig.RestartPolicy.MaximumRetryCount,
		},
		NetworkMode:  string(inspect.HostConfig.NetworkMode),
		RestartCount: inspect.RestartCount,
		LogPath:      inspect.LogPath,
	}

	return detail, nil
}

func (d *DockerClient) ContainerStats(ctx context.Context, id string) (*models.ContainerStats, error) {
	resp, err := d.cli.ContainerStats(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("getting stats for container %s: %w", id, err)
	}
	defer resp.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decoding stats for container %s: %w", id, err)
	}

	cpuPercent := calculateCPUPercent(&stats)
	memPercent := 0.0
	if stats.MemoryStats.Limit > 0 {
		memPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	var rxBytes, txBytes uint64
	for _, netStats := range stats.Networks {
		rxBytes += netStats.RxBytes
		txBytes += netStats.TxBytes
	}

	var readBytes, writeBytes uint64
	for _, bioEntry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch bioEntry.Op {
		case "read", "Read":
			readBytes += bioEntry.Value
		case "write", "Write":
			writeBytes += bioEntry.Value
		}
	}

	return &models.ContainerStats{
		ContainerID: id,
		Timestamp:   time.Now(),
		CPU: models.CPUStats{
			Percent: cpuPercent,
			System:  stats.CPUStats.SystemUsage,
			User:    stats.CPUStats.CPUUsage.UsageInUsermode,
		},
		Memory: models.MemStats{
			Usage:   stats.MemoryStats.Usage,
			Limit:   stats.MemoryStats.Limit,
			Percent: memPercent,
		},
		NetworkIO: models.NetStats{
			RxBytes: rxBytes,
			TxBytes: txBytes,
		},
		BlockIO: models.IOStats{
			ReadBytes:  readBytes,
			WriteBytes: writeBytes,
		},
		PIDs: int64(stats.PidsStats.Current),
	}, nil
}

func (d *DockerClient) StreamStats(ctx context.Context, id string) (<-chan *models.ContainerStats, error) {
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
				stats, err := d.ContainerStats(ctx, id)
				if err != nil {
					slog.Error("streaming stats", "container", id, "error", err)
					return
				}
				select {
				case ch <- stats:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func (d *DockerClient) ListImages(ctx context.Context) ([]models.Image, error) {
	images, err := d.cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	result := make([]models.Image, 0, len(images))
	for _, img := range images {
		tags := img.RepoTags
		if tags == nil {
			tags = []string{}
		}

		digest := ""
		if len(img.RepoDigests) > 0 {
			digest = img.RepoDigests[0]
		}

		result = append(result, models.Image{
			ID:      img.ID,
			Tags:    tags,
			Size:    img.Size,
			Created: time.Unix(img.Created, 0),
			Digest:  digest,
		})
	}

	return result, nil
}

func (d *DockerClient) InspectImage(ctx context.Context, id string) (*models.ImageDetail, error) {
	inspect, _, err := d.cli.ImageInspectWithRaw(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspecting image %s: %w", id, err)
	}

	tags := inspect.RepoTags
	if tags == nil {
		tags = []string{}
	}

	digest := ""
	if len(inspect.RepoDigests) > 0 {
		digest = inspect.RepoDigests[0]
	}

	var cmd, entrypoint, env []string
	var workingDir string
	if inspect.Config != nil {
		cmd = inspect.Config.Cmd
		entrypoint = inspect.Config.Entrypoint
		env = inspect.Config.Env
		workingDir = inspect.Config.WorkingDir
	}

	return &models.ImageDetail{
		Image: models.Image{
			ID:      inspect.ID,
			Tags:    tags,
			Size:    inspect.Size,
			Created: parseTime(inspect.Created),
			Digest:  digest,
		},
		OS:           inspect.Os,
		Architecture: inspect.Architecture,
		Author:       inspect.Author,
		Layers:       inspect.RootFS.Layers,
		Env:          env,
		Cmd:          cmd,
		Entrypoint:   entrypoint,
		WorkingDir:   workingDir,
	}, nil
}

func (d *DockerClient) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	resp, err := d.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	// Get containers to determine which volumes are in use
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers for volume usage: %w", err)
	}

	usedVolumes := make(map[string]bool)
	for _, c := range containers {
		for _, m := range c.Mounts {
			if m.Type == "volume" {
				usedVolumes[m.Name] = true
			}
		}
	}

	result := make([]models.Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		createdAt := time.Time{}
		if v.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
				createdAt = t
			}
		}

		result = append(result, models.Volume{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			CreatedAt:  createdAt,
			Labels:     v.Labels,
			InUse:      usedVolumes[v.Name],
		})
	}

	return result, nil
}

func (d *DockerClient) StopContainer(ctx context.Context, id string) error {
	if err := d.cli.ContainerStop(ctx, id, container.StopOptions{}); err != nil {
		return fmt.Errorf("stopping container %s: %w", id, err)
	}
	return nil
}

func (d *DockerClient) RestartContainer(ctx context.Context, id string) error {
	if err := d.cli.ContainerRestart(ctx, id, container.StopOptions{}); err != nil {
		return fmt.Errorf("restarting container %s: %w", id, err)
	}
	return nil
}

// calculateCPUPercent computes CPU usage percentage from Docker stats.
func calculateCPUPercent(stats *container.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuCount := float64(stats.CPUStats.OnlineCPUs)
		if cpuCount == 0 {
			cpuCount = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		if cpuCount == 0 {
			cpuCount = 1
		}
		return (cpuDelta / systemDelta) * cpuCount * 100.0
	}
	return 0.0
}

// parseTime parses a Docker API time string into time.Time.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// pidsLimit converts a *int64 to int64, returning 0 for nil.
func pidsLimit(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
