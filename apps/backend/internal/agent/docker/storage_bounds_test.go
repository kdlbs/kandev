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
	if cfg.Tmpfs["/tmp"] != "rw,exec,nosuid,nodev,size=2147483648,mode=1777" {
		t.Fatalf("new container temporary scratch is not bounded: %#v", cfg.Tmpfs)
	}
}

func TestExplicitTemporaryMountIsNotShadowed(t *testing.T) {
	cases := []struct {
		target    string
		wantTmpfs bool
	}{
		{target: "/tmp"},
		{target: "/tmp/"},
		{target: "//tmp"},
		{target: "/var/../tmp"},
		{target: "/var/tmp", wantTmpfs: true},
		{target: "/tmp/cache", wantTmpfs: true},
		{target: "/tmp-other", wantTmpfs: true},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			original := mount.Mount{Type: mount.TypeBind, Source: "/host/task-tmp", Target: tc.target, ReadOnly: true}
			cfg := buildHostConfig(ContainerConfig{}, []mount.Mount{original}, nil)
			if _, ok := cfg.Tmpfs["/tmp"]; ok != tc.wantTmpfs {
				t.Fatalf("default tmpfs present = %v, want %v", ok, tc.wantTmpfs)
			}
			if len(cfg.Mounts) != 1 || cfg.Mounts[0].Type != original.Type ||
				cfg.Mounts[0].Source != original.Source || cfg.Mounts[0].Target != original.Target ||
				cfg.Mounts[0].ReadOnly != original.ReadOnly {
				t.Fatalf("explicit mount changed: %#v", cfg.Mounts)
			}
		})
	}
}
