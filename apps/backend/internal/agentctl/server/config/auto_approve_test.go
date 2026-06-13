package config

import "testing"

func TestNewInstanceConfigAutoApproveFromEnvOverride(t *testing.T) {
	cfg := &Config{
		Defaults: InstanceDefaults{
			AutoApprovePermissions: false,
		},
	}

	instanceCfg := cfg.NewInstanceConfig(0, &InstanceOverrides{
		Env: []string{"AGENTCTL_AUTO_APPROVE_PERMISSIONS=true"},
	})

	if !instanceCfg.AutoApprovePermissions {
		t.Fatal("AutoApprovePermissions = false, want true from per-instance env override")
	}
}

func TestNewInstanceConfigAutoApproveFromExplicitOverride(t *testing.T) {
	cfg := &Config{
		Defaults: InstanceDefaults{
			AutoApprovePermissions: false,
		},
	}

	autoApprove := true
	instanceCfg := cfg.NewInstanceConfig(0, &InstanceOverrides{
		AutoApprovePermissions: &autoApprove,
	})

	if !instanceCfg.AutoApprovePermissions {
		t.Fatal("AutoApprovePermissions = false, want true from explicit per-instance override")
	}
}

func TestNewInstanceConfigAutoApproveExplicitOverrideCanDisableDefault(t *testing.T) {
	cfg := &Config{
		Defaults: InstanceDefaults{
			AutoApprovePermissions: true,
		},
	}

	autoApprove := false
	instanceCfg := cfg.NewInstanceConfig(0, &InstanceOverrides{
		AutoApprovePermissions: &autoApprove,
	})

	if instanceCfg.AutoApprovePermissions {
		t.Fatal("AutoApprovePermissions = true, want false from explicit per-instance override")
	}
}
