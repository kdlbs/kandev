package docker

import (
	"fmt"
	"path"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// applyContainerStorageOverrides applies only the storage options selected by
// the executor profile. Explicit task mounts retain their own storage policy.
func applyContainerStorageOverrides(hc *container.HostConfig, mounts []mount.Mount, tmpSizeBytes int64, logMaxSize, logMaxFiles string) {
	if logMaxSize != "" || logMaxFiles != "" {
		hc.LogConfig.Config = make(map[string]string, 2)
		if logMaxSize != "" {
			hc.LogConfig.Config["max-size"] = logMaxSize
		}
		if logMaxFiles != "" {
			hc.LogConfig.Config["max-file"] = logMaxFiles
		}
	}
	if tmpSizeBytes <= 0 {
		return
	}
	for _, m := range mounts {
		if path.Clean(m.Target) == "/tmp" {
			return
		}
	}
	hc.Tmpfs = map[string]string{
		"/tmp": fmt.Sprintf("rw,exec,nosuid,nodev,size=%d,mode=1777", tmpSizeBytes),
	}
}
