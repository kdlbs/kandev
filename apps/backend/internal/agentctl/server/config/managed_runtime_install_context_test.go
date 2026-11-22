package config

import (
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestNewInstanceConfigPropagatesManagedRuntimeInstallContext(t *testing.T) {
	config := &Config{HomeDir: t.TempDir()}
	context := &agentctltypes.ManagedRuntimeInstallContext{
		InstallerEnvDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		OwnerScope:         "task:owner-a",
	}
	instance, err := config.NewInstanceConfig(0, &InstanceOverrides{
		WorkDir:                      t.TempDir(),
		ManagedRuntimeInstallContext: context,
	})
	if err != nil {
		t.Fatalf("create instance config: %v", err)
	}
	if instance.ManagedRuntimeInstallContext == nil ||
		*instance.ManagedRuntimeInstallContext != *context {
		t.Fatalf("install context = %#v, want the authenticated provenance", instance.ManagedRuntimeInstallContext)
	}
	context.OwnerScope = "changed"
	if instance.ManagedRuntimeInstallContext.OwnerScope != "task:owner-a" {
		t.Fatal("instance config retained a mutable caller-owned install context")
	}
	defaults, err := config.NewInstanceConfig(0, nil)
	if err != nil {
		t.Fatalf("create default instance config: %v", err)
	}
	if defaults.ManagedRuntimeInstallContext != nil {
		t.Fatalf("default install context = %#v, want nil", defaults.ManagedRuntimeInstallContext)
	}
}
