package components

import (
	"strings"
	"testing"

	"github.com/mathesh-me/wharfeye/internal/models"
)

func TestRenderTable_Empty(t *testing.T) {
	result := RenderTable(nil, 0, 100)
	if !strings.Contains(result, "NAME") {
		t.Error("expected header even with no rows")
	}
}

func TestRenderTable_WithRows(t *testing.T) {
	rows := []ContainerRow{
		{
			Container: models.Container{
				ID:    "abc123",
				Name:  "web-server",
				Image: "nginx:latest",
				State: "running",
				Status: "Up 2 hours",
			},
			CPU:    2.5,
			Memory: 128 * 1024 * 1024,
		},
		{
			Container: models.Container{
				ID:    "def456",
				Name:  "stopped-app",
				Image: "myapp:v1",
				State: "exited",
				Status: "Exited (0)",
			},
		},
	}

	result := RenderTable(rows, 0, 120)

	if !strings.Contains(result, "web-server") {
		t.Error("expected web-server in output")
	}
	if !strings.Contains(result, "stopped-app") {
		t.Error("expected stopped-app in output")
	}
	if !strings.Contains(result, "NAME") {
		t.Error("expected header")
	}
}

func TestFormatBytesShort(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{500, "500B"},
		{2048, "2K"},
		{50 * 1024 * 1024, "50M"},
		{2 * 1024 * 1024 * 1024, "2.0G"},
	}

	for _, tt := range tests {
		got := formatBytesShort(tt.input)
		if got != tt.want {
			t.Errorf("formatBytesShort(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"a-very-long-container-name", 15, "a-very-long-..."},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncateStr(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
