package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPreparationAllowsUnusedHelperWithDefaultCommand(t *testing.T) {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	for _, state := range []string{"default", "explicit", "configured", "started", "passthrough-stopped"} {
		t.Run(state, func(t *testing.T) {
			base := &config.Config{}
			base.Defaults.AgentCommand = "auggie --acp"
			overrides := &config.InstanceOverrides{TaskID: "task"}
			if state == "explicit" {
				overrides.AgentCommand = "configured-agent"
			}
			cfg, err := base.NewInstanceConfig(43210, overrides)
			require.NoError(t, err)
			manager := &Manager{cfg: cfg, logger: newTestLogger(t)}
			manager.status.Store(StatusStopped)
			if state == "passthrough-stopped" {
				manager.passthrough = &PassthroughOwner{}
				manager.passthrough.current.Generation = 1
			}
			if state == "configured" {
				require.NoError(t, manager.Configure("configured-agent", nil, false, nil, "", nil, false))
			}
			if state == "started" {
				manager.beginManagedStartupGeneration()
			}
			applied := false
			err = manager.PrepareBuiltinProfile(profile, func(p.Context) error { applied = true; return nil })
			if state == "default" {
				require.NoError(t, err)
				require.True(t, applied)
				require.NotNil(t, cfg.McpProfile.BuiltinPackages)
			} else {
				require.Error(t, err)
				require.False(t, applied)
			}
		})
	}
}
