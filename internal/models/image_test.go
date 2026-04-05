package models

import (
	"testing"
	"time"
)

func TestImage_ShortID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"with sha256 prefix", "sha256:abc123def456789", "abc123def456"},
		{"without prefix", "abc123def456789", "abc123def456"},
		{"short ID", "abc123", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := &Image{ID: tt.id}
			if got := img.ShortID(); got != tt.want {
				t.Errorf("ShortID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImage_HumanSize(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{"bytes", 500, "500 B"},
		{"kilobytes", 2048, "2.0 KB"},
		{"megabytes", 50 * 1024 * 1024, "50.0 MB"},
		{"gigabytes", 2 * 1024 * 1024 * 1024, "2.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := &Image{Size: tt.size}
			if got := img.HumanSize(); got != tt.want {
				t.Errorf("HumanSize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImageFields(t *testing.T) {
	img := Image{
		ID:      "sha256:abc123",
		Tags:    []string{"nginx:latest", "nginx:1.25"},
		Size:    150 * 1024 * 1024,
		Created: time.Now(),
		Digest:  "sha256:digest123",
	}

	if len(img.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(img.Tags))
	}
	if img.Digest != "sha256:digest123" {
		t.Errorf("unexpected digest: %s", img.Digest)
	}
}
