package models

import (
	"fmt"
	"time"
)

// Container represents a container's basic information.
type Container struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	ImageID string            `json:"image_id"`
	Status  string            `json:"status"`
	State   string            `json:"state"`
	Created time.Time         `json:"created"`
	Ports   []Port            `json:"ports"`
	Labels  map[string]string `json:"labels"`
	Mounts  []Mount           `json:"mounts"`
	Runtime string            `json:"runtime"`
}

// Port represents a container port mapping.
type Port struct {
	IP            string `json:"ip,omitempty"`
	HostPort      uint16 `json:"host_port,omitempty"`
	ContainerPort uint16 `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// Mount represents a container volume mount.
type Mount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

// ContainerDetail extends Container with additional inspection data.
type ContainerDetail struct {
	Container
	Config      ContainerConfig `json:"config"`
	HostConfig  HostConfig      `json:"host_config"`
	NetworkMode string          `json:"network_mode"`
	RestartCount int            `json:"restart_count"`
	Platform    string          `json:"platform"`
	LogPath     string          `json:"log_path"`
}

// ContainerConfig holds container configuration details.
type ContainerConfig struct {
	Hostname    string            `json:"hostname"`
	User        string            `json:"user"`
	Env         []string          `json:"env"`
	Cmd         []string          `json:"cmd"`
	Entrypoint  []string          `json:"entrypoint"`
	WorkingDir  string            `json:"working_dir"`
	Labels      map[string]string `json:"labels"`
	Healthcheck *Healthcheck      `json:"healthcheck,omitempty"`
}

// Healthcheck holds container health check configuration.
type Healthcheck struct {
	Test     []string      `json:"test"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Retries  int           `json:"retries"`
}

// HostConfig holds container host configuration.
type HostConfig struct {
	Privileged        bool     `json:"privileged"`
	PidMode           string   `json:"pid_mode"`
	IpcMode           string   `json:"ipc_mode"`
	UTSMode           string   `json:"uts_mode"`
	ReadonlyRootfs    bool     `json:"readonly_rootfs"`
	CapAdd            []string `json:"cap_add"`
	CapDrop           []string `json:"cap_drop"`
	SecurityOpt       []string `json:"security_opt"`
	MemoryLimit       int64    `json:"memory_limit"`
	CPUShares         int64    `json:"cpu_shares"`
	CPUQuota          int64    `json:"cpu_quota"`
	NanoCPUs          int64    `json:"nano_cpus"`
	PidsLimit         int64    `json:"pids_limit"`
	RestartPolicy     string   `json:"restart_policy"`
	RestartMaxRetries int      `json:"restart_max_retries"`
}

// HasCPULimit returns true if any form of CPU limit is set.
func (h HostConfig) HasCPULimit() bool {
	return h.CPUQuota > 0 || h.NanoCPUs > 0
}

// ContainerStats holds point-in-time container resource usage.
type ContainerStats struct {
	ContainerID string    `json:"container_id"`
	Timestamp   time.Time `json:"timestamp"`
	CPU         CPUStats  `json:"cpu"`
	Memory      MemStats  `json:"memory"`
	NetworkIO   NetStats  `json:"network_io"`
	BlockIO     IOStats   `json:"block_io"`
	PIDs        int64     `json:"pids"`
}

// CPUStats holds CPU usage metrics.
type CPUStats struct {
	Percent float64 `json:"percent"`
	System  uint64  `json:"system"`
	User    uint64  `json:"user"`
}

// MemStats holds memory usage metrics.
type MemStats struct {
	Usage   uint64  `json:"usage"`
	Limit   uint64  `json:"limit"`
	Percent float64 `json:"percent"`
}

// NetStats holds network I/O metrics.
type NetStats struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// IOStats holds block I/O metrics.
type IOStats struct {
	ReadBytes  uint64 `json:"read_bytes"`
	WriteBytes uint64 `json:"write_bytes"`
}

// IsRunning returns true if the container is in running state.
func (c *Container) IsRunning() bool {
	return c.State == "running"
}

// ShortID returns the first 12 characters of the container ID.
func (c *Container) ShortID() string {
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// PortString returns a human-readable port mapping string.
func (p Port) String() string {
	if p.HostPort == 0 {
		return fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol)
	}
	if p.IP != "" {
		return fmt.Sprintf("%s:%d->%d/%s", p.IP, p.HostPort, p.ContainerPort, p.Protocol)
	}
	return fmt.Sprintf("%d->%d/%s", p.HostPort, p.ContainerPort, p.Protocol)
}
