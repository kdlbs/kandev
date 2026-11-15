package docker

import (
	"path"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// applyContainerStorageBounds limits disposable writes at container creation.
// Explicit task mounts retain their lifecycle and retention policy.
func applyContainerStorageBounds(hc *container.HostConfig, mounts []mount.Mount) {
	hc.LogConfig = container.LogConfig{
		Type:   "json-file",
		Config: map[string]string{"max-size": "100m", "max-file": "3"},
	}
	for _, m := range mounts {
		if path.Clean(m.Target) == "/tmp" {
			return
		}
	}
	hc.Tmpfs = map[string]string{"/tmp": "rw,exec,nosuid,nodev,size=2147483648,mode=1777"}
}
