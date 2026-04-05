package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Runtime RuntimeConfig `mapstructure:"runtime"`
	Web     WebConfig     `mapstructure:"web"`
}

type RuntimeConfig struct {
	Type   string `mapstructure:"type"`
	Socket string `mapstructure:"socket"`
}

type WebConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

// ConfigDir returns the default config directory path.
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "wharfeye")
}

// ConfigPath returns the default config file path.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// SetDefaults sets default configuration values.
func SetDefaults() {
	viper.SetDefault("runtime.type", "auto")
	viper.SetDefault("runtime.socket", "")

	viper.SetDefault("web.port", 9090)
	viper.SetDefault("web.host", "0.0.0.0")
}

// Load reads config from file and environment.
func Load() (*Config, error) {
	SetDefaults()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(ConfigDir())
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("WHARFEYE")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found is fine - use defaults
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// DefaultYAML returns the default config file content.
func DefaultYAML() string {
	return `# WharfEye configuration
# See: https://github.com/mathesh-me/wharfeye

# Container runtime (auto-detected if not specified)
runtime:
  type: auto    # auto | docker | podman | containerd
  socket: ""    # auto-detect if empty

# Web server
web:
  port: 9090
  host: 0.0.0.0
`
}

// WriteDefault writes the default config file to the standard location.
func WriteDefault() (string, error) {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	path := ConfigPath()
	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("config file already exists at %s", path)
	}

	if err := os.WriteFile(path, []byte(DefaultYAML()), 0o644); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return path, nil
}
