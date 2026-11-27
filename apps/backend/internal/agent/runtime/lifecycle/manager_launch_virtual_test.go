package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestResolveLaunchableAgent_RejectsVirtualFamily pins the W-045 chokepoint:
// a virtual execution family selected for a direct launch must fail closed
// with ErrVirtualProfile instead of reaching command building, where the
// empty virtual command would surface only as "agent command cannot be empty".
func TestResolveLaunchableAgent_RejectsVirtualFamily(t *testing.T) {
	mgr := newTestManager(t)
	// The test registry already carries the built-in DynamicAgent, which is
	// exactly why a virtual selection can reach command building unguarded.
	require.NoError(t, mgr.registry.Register(&testAgent{id: "concrete-agent", name: "concrete-agent", enabled: true}))

	_, err := mgr.resolveLaunchableAgent(agents.DynamicAgentID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrVirtualProfile)
	require.NotContains(t, err.Error(), "cannot be empty")

	_, err = mgr.resolveLaunchableAgent("no-such-agent")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrVirtualProfile)
	require.Contains(t, err.Error(), "not found in registry")

	got, err := mgr.resolveLaunchableAgent("concrete-agent")
	require.NoError(t, err)
	require.Equal(t, "concrete-agent", got.ID())
}

// TestRestartAgentProcess_VirtualAgentFailsClosed proves the relaunch path no
// longer reaches command building for a virtual family: the restart is
// refused with ErrVirtualProfile and the current process is left untouched.
func TestRestartAgentProcess_VirtualAgentFailsClosed(t *testing.T) {
	mgr := newTestManager(t)
	// See above: DynamicAgent is already registered in the test registry.
	mgr.profileResolver = &restartProfileResolver{profile: &AgentProfileInfo{
		ProfileID: "profile-1",
		AgentName: agents.DynamicAgentID,
	}}
	mock := newRestartMockAgentctlServer(t, false, false)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	execution := &AgentExecution{
		ID:             "exec-virtual-restart",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		ACPSessionID:   "old-acp-session",
		AgentCommand:   "concrete --resume old-acp-session",
		AgentArgs:      []string{"concrete", "--resume", "old-acp-session"},
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.RestartAgentProcess(context.Background(), execution.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrVirtualProfile)
	require.NotContains(t, err.Error(), "cannot be empty")

	current, found := mgr.executionStore.Get(execution.ID)
	require.True(t, found)
	require.Equal(t, "concrete --resume old-acp-session", current.AgentCommand)
	require.Equal(t, []string{"concrete", "--resume", "old-acp-session"}, current.AgentArgs)
}
