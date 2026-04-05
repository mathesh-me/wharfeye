package models

import (
	"testing"
	"time"
)

func TestContainer_IsRunning(t *testing.T) {
	tests := []struct {
		name  string
		state string
		want  bool
	}{
		{"running container", "running", true},
		{"stopped container", "exited", false},
		{"created container", "created", false},
		{"paused container", "paused", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{State: tt.state}
			if got := c.IsRunning(); got != tt.want {
				t.Errorf("IsRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainer_ShortID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"long ID", "abc123def456789abcdef", "abc123def456"},
		{"exact 12", "abc123def456", "abc123def456"},
		{"short ID", "abc123", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{ID: tt.id}
			if got := c.ShortID(); got != tt.want {
				t.Errorf("ShortID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPort_String(t *testing.T) {
	tests := []struct {
		name string
		port Port
		want string
	}{
		{
			"container only",
			Port{ContainerPort: 80, Protocol: "tcp"},
			"80/tcp",
		},
		{
			"with host port",
			Port{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			"8080->80/tcp",
		},
		{
			"with IP",
			Port{IP: "0.0.0.0", HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			"0.0.0.0:8080->80/tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.port.String(); got != tt.want {
				t.Errorf("Port.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContainerFields(t *testing.T) {
	c := Container{
		ID:      "test-id-123456789012",
		Name:    "my-container",
		Image:   "nginx:latest",
		ImageID: "sha256:abcdef",
		Status:  "Up 3 hours",
		State:   "running",
		Created: time.Now(),
		Runtime: "docker",
		Labels: map[string]string{
			"app": "web",
		},
	}

	if c.Name != "my-container" {
		t.Errorf("unexpected name: %s", c.Name)
	}
	if c.Runtime != "docker" {
		t.Errorf("unexpected runtime: %s", c.Runtime)
	}
	if c.Labels["app"] != "web" {
		t.Errorf("unexpected label: %s", c.Labels["app"])
	}
}

func TestContainerStats_Fields(t *testing.T) {
	stats := ContainerStats{
		ContainerID: "test-id",
		Timestamp:   time.Now(),
		CPU: CPUStats{
			Percent: 25.5,
			System:  1000,
			User:    500,
		},
		Memory: MemStats{
			Usage:   1024 * 1024 * 256, // 256MB
			Limit:   1024 * 1024 * 512, // 512MB
			Percent: 50.0,
		},
		NetworkIO: NetStats{
			RxBytes: 1024,
			TxBytes: 2048,
		},
		BlockIO: IOStats{
			ReadBytes:  4096,
			WriteBytes: 8192,
		},
		PIDs: 10,
	}

	if stats.CPU.Percent != 25.5 {
		t.Errorf("unexpected CPU percent: %f", stats.CPU.Percent)
	}
	if stats.Memory.Usage != 1024*1024*256 {
		t.Errorf("unexpected memory usage: %d", stats.Memory.Usage)
	}
	if stats.PIDs != 10 {
		t.Errorf("unexpected PIDs: %d", stats.PIDs)
	}
}
