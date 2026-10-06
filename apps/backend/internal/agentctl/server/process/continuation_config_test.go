package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContinuationConfigAdapterInheritance(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), Protocol: agent.ProtocolACP, ProviderInterruptionContinuation: enabled}, newTestLogger(t))
		t.Cleanup(mgr.stopWorkspaceTrackers)
		require.NoError(t, mgr.buildAdapterConfig())
		t.Cleanup(func() { _ = mgr.adapter.Close() })
		require.Equal(t, enabled, mgr.adapterCfg.ToSharedConfig().ProviderInterruptionContinuation)
	}
}
