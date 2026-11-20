package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/pkg/agent"
)

type restartMCPAcceptanceSpy struct {
	*sessionMCPStateFake
	execution  *AgentExecution
	called     bool
	clientHeld bool
}

func (r *restartMCPAcceptanceSpy) SaveAcceptedMCPSetup(context.Context, string, int64, string, *mcpconfig.MCPSetupSnapshot) error {
	r.called = true
	if r.execution.agentctlLifecycleMu.TryLock() {
		r.execution.agentctlLifecycleMu.Unlock()
	} else {
		r.clientHeld = true
	}
	return nil
}

func TestMCPRestartReleasesClientBeforeAcceptanceLease(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()
	client := createTestClient(t, mock.server.URL)
	defer client.Close()
	_ = connectAgentStream(t, mock, client)
	manager := newTestManager(t)
	manager.streamManager = nil
	manager.sessionManager = NewSessionManager(newSessionTestLogger(), manager.stopCh)
	execution := &AgentExecution{ID: "restart", TaskID: "task", SessionID: "session", WorkspacePath: "/workspace", agentctl: client}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	repository := &restartMCPAcceptanceSpy{sessionMCPStateFake: newSessionMCPStateFake(), execution: execution}
	manager.SetMCPSelectionStateRepository(repository)
	operation := prepareMCPResolution(&mcpconfig.EffectiveMCPResolution{Setup: &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}})
	config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP}}
	if err := manager.initializeACPSessionForRestart(operation.context(context.Background()), execution, config, nil); err != nil {
		t.Fatal(err)
	}
	if !repository.called || repository.clientHeld {
		t.Fatal("accepted-setup callback retained the RPC client lease while acquiring the startup lease")
	}
}
