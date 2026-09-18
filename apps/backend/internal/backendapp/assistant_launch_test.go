package backendapp

import (
	"context"
	"fmt"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	orchmodels "github.com/kandev/kandev/internal/orchestration/models"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-ORCHESTRATION-ASSISTANT-004.1
func TestAssistantPreparedSessionPersistsRestriction(t *testing.T) {
	for _, name := range []string{"assistant", "coordinator", "revoked"} {
		t.Run(name, func(t *testing.T) {
			a, _, _, conversation := privateConversationFixture(t)
			ctx := context.Background()
			require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
				ID: "session", TaskID: conversation, State: taskmodels.TaskSessionStateCreated,
				Metadata: map[string]any{"native_conversation": true},
			}))
			launch := orchestrationruntime.Launch{TaskID: conversation, OnSessionPrepared: func(ctx context.Context, id string) error {
				if name == "revoked" {
					return fmt.Errorf("authority revoked")
				}
				return a.taskRepo.SetSessionMetadataKey(ctx, id, "run_bound", true)
			}}
			if name != "coordinator" {
				launch.Authority = &orchmodels.AssistantAuthority{Restriction: "claude-broker-v1"}
			}
			prepared := orchestrationLaunchContext(&Repositories{Task: a.taskRepo}, launch)
			err := prepared.OnSessionPrepared(ctx, "session")
			if name == "revoked" {
				require.ErrorContains(t, err, "authority revoked")
			} else {
				require.NoError(t, err)
			}
			session, err := a.taskRepo.GetTaskSession(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, true, session.Metadata["native_conversation"])
			require.NotNil(t, prepared.McpProfile)
			switch name {
			case "assistant":
				require.Equal(t, mcpprofile.SurfaceAssistantBroker, prepared.McpProfile.Surface)
				require.Equal(t, true, session.Metadata["run_bound"])
				require.Equal(t, string(mcpprofile.SurfaceAssistantBroker), session.Metadata[orchestrationruntime.AssistantPolicyMetadata])
			case "coordinator":
				require.Equal(t, mcpprofile.SurfaceOrchestratorBroker, prepared.McpProfile.Surface)
				require.NotContains(t, session.Metadata, orchestrationruntime.AssistantPolicyMetadata)
			case "revoked":
				require.Equal(t, mcpprofile.SurfaceAssistantBroker, prepared.McpProfile.Surface)
			}
		})
	}
}
