package models

import "time"

// Volume represents a container volume.
type Volume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	CreatedAt  time.Time         `json:"created_at"`
	Labels     map[string]string `json:"labels"`
	InUse      bool              `json:"in_use"`
}
