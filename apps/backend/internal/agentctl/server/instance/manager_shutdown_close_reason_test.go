package instance

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

// TestShutdownClosesAgentStreamsWithAgentctlShutdownReason pins the
// "Capability and close reason" bullet of system design part 2: a graceful
// agentctl-wide Shutdown closes every live instance's current backend stream
// with code 1000 and reason agentctl_shutdown, before tearing the instance
// down -- distinct from an unexpected agent exit (agent_exited) or an
// ordinary single-instance stop.
func TestShutdownClosesAgentStreamsWithAgentctlShutdownReason(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	procMgr := &fakeProcessManager{}
	inst := &Instance{
		ID:        "shutdown-close-reason",
		Status:    "running",
		CreatedAt: time.Now(),
		manager:   procMgr,
		server:    &fakeHTTPServer{},
	}
	mgr.mu.Lock()
	mgr.instances[inst.ID] = inst
	mgr.mu.Unlock()

	require.NoError(t, mgr.Shutdown(context.Background()))

	require.True(t, procMgr.closeAgentStreamCalled, "Shutdown() must close the instance's agent stream")
	require.Equal(t, process.CloseCodeNormal, procMgr.closedCode)
	require.Equal(t, process.CloseReasonAgentctlShutdown, procMgr.closedReason)
	require.True(t, procMgr.stopped, "Shutdown() must still stop the process manager")
}
