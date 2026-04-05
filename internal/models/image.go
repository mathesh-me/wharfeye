package models

import (
	"fmt"
	"time"
)

// Image represents a container image.
type Image struct {
	ID      string    `json:"id"`
	Tags    []string  `json:"tags"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	Digest  string    `json:"digest"`
}

// ImageDetail extends Image with additional inspection data.
type ImageDetail struct {
	Image
	OS           string   `json:"os"`
	Architecture string   `json:"architecture"`
	Author       string   `json:"author"`
	Layers       []string `json:"layers"`
	Env          []string `json:"env"`
	Cmd          []string `json:"cmd"`
	Entrypoint   []string `json:"entrypoint"`
	WorkingDir   string   `json:"working_dir"`
}

// ShortID returns the first 12 characters of the image ID.
func (i *Image) ShortID() string {
	id := i.ID
	// Strip sha256: prefix if present
	if len(id) > 7 && id[:7] == "sha256:" {
		id = id[7:]
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// HumanSize returns image size in human-readable format.
func (i *Image) HumanSize() string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case i.Size >= gb:
		return fmt.Sprintf("%.1f GB", float64(i.Size)/float64(gb))
	case i.Size >= mb:
		return fmt.Sprintf("%.1f MB", float64(i.Size)/float64(mb))
	case i.Size >= kb:
		return fmt.Sprintf("%.1f KB", float64(i.Size)/float64(kb))
	default:
		return fmt.Sprintf("%d B", i.Size)
	}
}
