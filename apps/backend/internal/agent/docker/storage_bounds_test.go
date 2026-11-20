package docker

import (
	"testing"

	"github.com/moby/moby/api/types/mount"
)

func TestNewContainerInheritsDockerStorageDefaults(t *testing.T) {
	cfg := buildHostConfig(ContainerConfig{}, nil, nil)
	if cfg.LogConfig.Type != "" || len(cfg.LogConfig.Config) != 0 {
		t.Fatalf("new container overrides the daemon logging defaults: %#v", cfg.LogConfig)
	}
	if _, ok := cfg.Tmpfs["/tmp"]; ok {
		t.Fatalf("new container overrides the image /tmp default: %#v", cfg.Tmpfs)
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
			cfg := buildHostConfig(ContainerConfig{TmpfsSizeBytes: 2 * 1024 * 1024 * 1024}, []mount.Mount{original}, nil)
			if _, ok := cfg.Tmpfs["/tmp"]; ok != tc.wantTmpfs {
				t.Fatalf("configured tmpfs present = %v, want %v", ok, tc.wantTmpfs)
			}
			if tc.wantTmpfs && cfg.Tmpfs["/tmp"] != "rw,exec,nosuid,nodev,size=2147483648,mode=1777" {
				t.Fatalf("configured tmpfs = %q, want the requested size", cfg.Tmpfs["/tmp"])
			}
			if len(cfg.Mounts) != 1 || cfg.Mounts[0].Type != original.Type ||
				cfg.Mounts[0].Source != original.Source || cfg.Mounts[0].Target != original.Target ||
				cfg.Mounts[0].ReadOnly != original.ReadOnly {
				t.Fatalf("explicit mount changed: %#v", cfg.Mounts)
			}
		})
	}
}
