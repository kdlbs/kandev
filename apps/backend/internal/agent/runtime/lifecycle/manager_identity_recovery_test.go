package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
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

type recordingMcpConfigProvider struct {
	profileIDs []string
}

func (p *recordingMcpConfigProvider) GetConfigByProfileID(_ context.Context, profileID string) (*mcpconfig.ProfileConfig, error) {
	p.profileIDs = append(p.profileIDs, profileID)
	return nil, nil
}

func TestPromoteWorkspaceExecutionRebindsAndPersistsSelectedProfile(t *testing.T) {
	for _, officeProfileID := range []string{"", "reviewer"} {
		t.Run("office_profile_"+officeProfileID, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.profileResolver = &mockPassthroughProfileResolver{agentName: "pi-acp"}
			mcp := &recordingMcpConfigProvider{}
			mgr.mcpProvider = mcp
			writer := &captureExecutorRunningWriter{}
			mgr.SetExecutorRunningWriter(writer)
			execution := &AgentExecution{
				ID: "exec-workspace-only", TaskID: "task-1", SessionID: "session-1",
				AgentProfileID: "previous-candidate", OfficeAgentProfileID: "assignee",
				WorkspacePath: t.TempDir(), metadata: map[string]interface{}{}, standalonePort: 45678,
			}
			require.NoError(t, mgr.executionStore.Add(execution))

			err := mgr.promoteWorkspaceExecution(context.Background(), execution, &LaunchRequest{
				TaskID: "task-1", SessionID: "session-1",
				AgentProfileID: officeProfileID, ExecutionProfileID: "selected-candidate",
			})
			require.NoError(t, err)

			require.Equal(t, "selected-candidate", execution.AgentProfileID)
			require.Equal(t, []string{"selected-candidate"}, mcp.profileIDs,
				"project MCP must resolve with the selected profile, not the previous candidate")
			require.NotNil(t, writer.running, "a changed profile must be persisted")
			require.Equal(t, "selected-candidate", writer.running.ExecutionProfileID)
			wantOffice := "assignee"
			if officeProfileID != "" {
				wantOffice = officeProfileID
			}
			require.Equal(t, wantOffice, execution.OfficeAgentProfileID,
				"the Office identity stays separate from the execution profile")
		})
	}
}
