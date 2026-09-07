package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLaunchPromotesWorkspaceOnlyExecutionWithRequestedOfficeIdentity(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID: "execution-profile",
		AgentName: "auggie",
	}}

	existing := &AgentExecution{
		ID:                   "execution-workspace-only",
		SessionID:            "session-reviewer",
		TaskID:               "task-reviewer",
		AgentProfileID:       "execution-profile",
		OfficeAgentProfileID: "assignee",
	}
	require.NoError(t, mgr.executionStore.Add(existing))

	got, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:             existing.TaskID,
		SessionID:          existing.SessionID,
		AgentProfileID:     "reviewer",
		ExecutionProfileID: "execution-profile",
		ACPSessionID:       "acp-reviewer",
	})
	require.NoError(t, err)
	require.Same(t, existing, got)
	require.Equal(t, "reviewer", got.OfficeAgentProfileID,
		"promotion must replace the stale session owner with the acting Office identity")
}

func TestWorkspaceOfficeAgentProfileIDPrefersPersistedIdentity(t *testing.T) {
	got := workspaceOfficeAgentProfileID(&WorkspaceInfo{
		AgentProfileID: "assignee",
		Metadata: map[string]interface{}{
			MetadataKeyOfficeAgentProfileID: "reviewer",
		},
	})
	require.Equal(t, "reviewer", got)
}
