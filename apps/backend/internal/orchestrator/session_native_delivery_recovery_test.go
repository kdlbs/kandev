package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
func TestRecoverSessionUnknownPromptKeepsNativeHistory(t *testing.T) {
	for _, failLaunch := range []bool{false, true} {
		name := "resume"
		if failLaunch {
			name = "launch failure"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
			session, err := repo.GetTaskSession(ctx, "session1")
			require.NoError(t, err)
			session.AgentProfileID = "profile1"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			now := time.Now().UTC()
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "running1", SessionID: session.ID, TaskID: session.TaskID,
				AgentExecutionID: "old-execution", ResumeToken: "native-conversation", Resumable: true,
				CreatedAt: now, UpdatedAt: now,
			}))
			submission := &models.AgentDeliverySubmission{
				ID: "prompt:interrupted", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				PayloadHash: "original-hash", Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
				Outcome: "prompt_dispatch_failed",
			}
			_, err = repo.PrepareAgentDeliverySubmission(ctx, submission)
			require.NoError(t, err)
			block := &models.SessionRecoveryBlock{
				SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
				ConsumerReference: "agent_delivery", DeliverySubmissionID: submission.ID,
			}
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
			var launched *executor.LaunchAgentRequest
			started := false
			manager := &sessionUpdatingAgentManager{
				mockAgentManager: &mockAgentManager{
					repoForExecutionLookup: repo,
					launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
						launched = req
						if failLaunch {
							return nil, errors.New("runtime unavailable")
						}
						return &executor.LaunchAgentResponse{AgentExecutionID: "new-execution", Status: v1.AgentStatusStarting}, nil
					},
				},
				repo: repo, sessionID: session.ID, taskID: session.TaskID, onStartCalled: &started,
			}
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			service.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
			response, err := service.RecoverSession(ctx, session.TaskID, session.ID, "resume")
			if failLaunch {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, response.Success)
				require.True(t, started)
			}
			require.NotNil(t, launched)
			require.Equal(t, "native-conversation", launched.ACPSessionID)
			require.Empty(t, manager.capturedPromptCalls, "resume must not replay the interrupted prompt")
			stored, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			require.Equal(t, submission.Payload, stored.Payload)
			recovery, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			if failLaunch {
				require.Equal(t, models.RecoveryBlockOpen, recovery.State)
			} else {
				require.Equal(t, models.RecoveryBlockResolved, recovery.State)
				require.Equal(t, "resume", recovery.AuthorizedAction)
			}
		})
	}
}
