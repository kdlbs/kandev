package process

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/pkg/agent"
)

// A node policy reaches the adapter config only when enforced. Requested but
// unenforced (required with no wrapper, or a wrapper that is merely optional)
// must never select the outer-sandbox mode.
func TestBuildAdapterConfigOuterSandboxFollowsEnforcedNodePolicy(t *testing.T) {
	spec := launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{"/usr/bin/true", "--"}}
	tests := []struct {
		name string
		node launchwrapper.Node
		want bool
	}{
		{"no node policy", launchwrapper.Node{}, false},
		{"required but no wrapper loaded", launchwrapper.Node{Required: true}, false},
		{"wrapper loaded but optional", launchwrapper.Node{Spec: spec}, false},
		{"required and loaded", launchwrapper.Node{Spec: spec, Required: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				cfg: &config.InstanceConfig{
					AgentArgs:         []string{"cat"},
					WorkDir:           t.TempDir(),
					AgentEnv:          []string{"PATH=/usr/bin"},
					Protocol:          agent.ProtocolACP,
					NodeLaunchWrapper: tt.node,
				},
				logger: newTestLogger(t),
			}
			require.NoError(t, m.buildAdapterConfig())
			t.Cleanup(func() { _ = m.adapter.Close() })
			require.Equal(t, tt.want, m.adapterCfg.OuterSandboxEnforced)
			require.Equal(t, tt.want, m.adapterCfg.ToSharedConfig().OuterSandboxEnforced)
		})
	}
}
