package models

import (
	"testing"
	"time"
)

func TestVolumeFields(t *testing.T) {
	v := Volume{
		Name:       "my-volume",
		Driver:     "local",
		Mountpoint: "/var/lib/docker/volumes/my-volume/_data",
		CreatedAt:  time.Now(),
		Labels: map[string]string{
			"project": "wharfeye",
		},
		InUse: true,
	}

	if v.Name != "my-volume" {
		t.Errorf("unexpected name: %s", v.Name)
	}
	if v.Driver != "local" {
		t.Errorf("unexpected driver: %s", v.Driver)
	}
	if !v.InUse {
		t.Error("expected volume to be in use")
	}
	if v.Labels["project"] != "wharfeye" {
		t.Errorf("unexpected label: %s", v.Labels["project"])
	}
}
