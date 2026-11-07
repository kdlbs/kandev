package docker

import (
	"testing"

	"github.com/moby/moby/api/types/mount"
)

func TestNewContainerHasRotatingLogsAndBoundedTemporaryScratch(t *testing.T) {
	cfg := buildHostConfig(ContainerConfig{}, nil, nil)
	if cfg.LogConfig.Type != "json-file" || cfg.LogConfig.Config["max-size"] != "100m" || cfg.LogConfig.Config["max-file"] != "3" {
		t.Fatalf("new container logs have no expected rotation budget: %#v", cfg.LogConfig)
	}
	if cfg.Tmpfs["/tmp"] != "rw,nosuid,nodev,size=2147483648,mode=1777" {
		t.Fatalf("new container temporary scratch is not bounded: %#v", cfg.Tmpfs)
	}
}

func TestExplicitTemporaryMountIsNotShadowed(t *testing.T) {
	mounts := []mount.Mount{{Type: mount.TypeBind, Source: "/host/task-tmp", Target: "/tmp"}}
	cfg := buildHostConfig(ContainerConfig{}, mounts, nil)
	if _, ok := cfg.Tmpfs["/tmp"]; ok {
		t.Fatal("explicit task storage mount shadowed by temporary scratch")
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0].Source != "/host/task-tmp" {
		t.Fatal("explicit mount changed")
	}
}
