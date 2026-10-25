package executor

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func resolveCoordinatorProfile(t *testing.T, metadata map[string]interface{}) (mcpprofile.Context, error) {
	t.Helper()
	return resolveCoordinatorProfileWithPhase2(t, metadata, true)
}

func resolveCoordinatorProfileWithPhase2(t *testing.T, metadata map[string]interface{}, phase2 bool) (mcpprofile.Context, error) {
	t.Helper()
	task, session := coordinatorTaskAndSession()
	task.Metadata = metadata
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesReady: true, phase2: phase2})
	return exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
}

func encodedBinding(t *testing.T, taskID string) string {
	t.Helper()
	encoded, err := mcpprofile.MarshalCoordinatorToolPolicy(mcpprofile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: "coord-1", WorkspaceID: "ws-1", ConversationTaskID: taskID,
		ToolNames: []string{"list_tasks_kandev"},
	})
	require.NoError(t, err)
	return encoded
}

func TestResolveTaskSessionMCPProfile_CoordinatorBindingIsCarried(t *testing.T) {
	profile, err := resolveCoordinatorProfile(t, map[string]interface{}{
		mcpprofile.CoordinatorToolPolicyMetadataKey: encodedBinding(t, "conversation-task"),
	})
	require.NoError(t, err)
	require.Equal(t, mcpprofile.SurfaceCoordinator, profile.Surface)
	require.NotNil(t, profile.CoordinatorToolPolicy)
	require.Equal(t, []string{"list_tasks_kandev"}, profile.CoordinatorToolPolicy.ToolNames)
}

func TestResolveTaskSessionMCPProfile_CoordinatorWithoutBindingKeepsPhaseOne(t *testing.T) {
	profile, err := resolveCoordinatorProfile(t, nil)
	require.NoError(t, err)
	require.Nil(t, profile.CoordinatorToolPolicy)
}

func TestResolveTaskSessionMCPProfile_CoordinatorBindingFailsClosed(t *testing.T) {
	for name, value := range map[string]interface{}{
		"unparsable":    "not json",
		"wrong type":    42,
		"other task":    encodedBinding(t, "someone-else"),
		"empty binding": "",
	} {
		_, err := resolveCoordinatorProfile(t, map[string]interface{}{mcpprofile.CoordinatorToolPolicyMetadataKey: value})
		require.Error(t, err, name)
	}
}

// With phase 2 off a stored binding is ignored, valid or not, so the
// conversation keeps the phase-1 tool set and a corrupt binding cannot fail a
// session start.
func TestResolveTaskSessionMCPProfile_CoordinatorBindingIgnoredWithPhase2Off(t *testing.T) {
	for name, value := range map[string]interface{}{
		"valid":      encodedBinding(t, "conversation-task"),
		"unparsable": "not json",
		"other task": encodedBinding(t, "someone-else"),
	} {
		profile, err := resolveCoordinatorProfileWithPhase2(t,
			map[string]interface{}{mcpprofile.CoordinatorToolPolicyMetadataKey: value}, false)
		require.NoError(t, err, name)
		require.Equal(t, mcpprofile.SurfaceCoordinator, profile.Surface, name)
		require.Nil(t, profile.CoordinatorToolPolicy, name)
	}
}
