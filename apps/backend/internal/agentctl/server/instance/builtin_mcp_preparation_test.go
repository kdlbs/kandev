package instance

import (
	"context"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestCreatedHelperPreservesCommandProvenanceForBuiltinPreparation(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "explicit"}[explicit], func(t *testing.T) {
			mgr := NewManager(&config.Config{Ports: config.PortConfig{Base: 0, Max: 0}, Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP, AgentCommand: "auggie --acp"}}, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, mgr.Shutdown(context.Background())) })
			var proc *process.Manager
			mgr.SetServerFactory(func(_ *config.InstanceConfig, manager *process.Manager, _ *logger.Logger) http.Handler {
				proc = manager
				return http.NotFoundHandler()
			})
			request := &CreateRequest{TaskID: "task", WorkspacePath: t.TempDir()}
			if explicit {
				request.AgentCommand = "explicit-agent"
			}
			_, err := mgr.CreateInstance(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, proc)
			profile := p.New(p.SurfaceKanbanTask, nil, nil)
			profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			applied := false
			err = proc.PrepareBuiltinProfile(profile, func(p.Context) error { applied = true; return nil })
			if explicit {
				require.Error(t, err)
				require.False(t, applied)
			} else {
				require.NoError(t, err)
				require.True(t, applied)
			}
		})
	}
}
