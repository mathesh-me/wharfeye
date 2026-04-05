package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	if dir == "" {
		t.Skip("could not determine home directory")
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "wharfeye")
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath()
	if path == "" {
		t.Skip("could not determine home directory")
	}

	if filepath.Base(path) != "config.yaml" {
		t.Errorf("config file should be named config.yaml, got %s", filepath.Base(path))
	}
}

func TestDefaultYAML(t *testing.T) {
	yaml := DefaultYAML()

	if len(yaml) == 0 {
		t.Error("DefaultYAML should return non-empty content")
	}

	// Verify key sections exist
	sections := []string{"runtime:", "web:"}
	for _, section := range sections {
		if !containsStr(yaml, section) {
			t.Errorf("DefaultYAML missing section: %s", section)
		}
	}
}

func TestWriteDefault_AlreadyExists(t *testing.T) {
	// Create a temp dir to simulate existing config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "wharfeye")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating temp config dir: %v", err)
	}

	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("existing"), 0o644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	// Temporarily override HOME
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := WriteDefault()
	if err == nil {
		t.Error("expected error when config already exists")
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Use a temp dir with no config file to test defaults
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	if cfg.Runtime.Type != "auto" {
		t.Errorf("default runtime type should be 'auto', got %q", cfg.Runtime.Type)
	}
	if cfg.Web.Port != 9090 {
		t.Errorf("default web port should be 9090, got %d", cfg.Web.Port)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
