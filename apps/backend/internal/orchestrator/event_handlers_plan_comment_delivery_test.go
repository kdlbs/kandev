package orchestrator

import (
	"testing"

	"github.com/jmoiron/sqlx"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PLAN-COMMENTS-002.2
// @covers AC-TASKS-PLAN-COMMENTS-003.1
func TestExecuteQueuedMessagePlanCommentReachesAgentAdmission(t *testing.T) {
	for _, durable := range []bool{false, true} {
		protocol := "legacy"
		if durable {
			protocol = "durable"
		}
		for _, origin := range []string{"message", "queue", "ordinary"} {
			t.Run(protocol+"/"+origin, func(t *testing.T) {
				svc, manager, messages := newPlanCommentDeliveryService(t, durable)
				ctx := t.Context()
				const feedback = "Review the plan feedback before continuing."
				if origin == "message" {
					require.NoError(t, messages.CreateUserMessageIdempotent(
						ctx, "feedback-message", "t1", feedback, "s1", "", nil,
					))
				}
				queued, err := svc.messageQueue.QueueMessageWithMetadata(
					ctx, "s1", "t1", feedback, "",
					messagequeue.QueuedByUser, false, nil, planCommentDeliveryMetadata(origin),
				)
				require.NoError(t, err)
				identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "t1", "s1")
				require.NoError(t, err)
				reserved, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, queued.ID, reserved.ID)
				reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(ctx, identity, reserved.ID, reserved)

				svc.executeQueuedMessageWithReservation("s1", reserved, reservation)

				require.Len(t, manager.capturedPromptCalls, 1, "accepted feedback must reach the agent")
				require.Contains(t, manager.capturedPromptCalls[0].Prompt, queued.Content)
				require.Len(t, messages.userMessages, 1)
				require.Zero(t, svc.messageQueue.GetStatus(ctx, "s1").Count)
				pending, err := svc.messageQueue.ListPendingQueueDispatches(ctx)
				require.NoError(t, err)
				require.Empty(t, pending)
				if durable {
					require.Len(t, manager.submissionIDs, 1)
					deliveryRepo := svc.repo.(repository.AgentDeliveryRepository)
					stored, err := deliveryRepo.GetAgentDeliverySubmission(ctx, manager.submissionIDs[0])
					require.NoError(t, err)
					require.Contains(t, string(stored.Payload), queued.Content)
					require.Equal(t, journal.SubmissionHash(stored.Payload), stored.PayloadHash)
				}
			})
		}
	}
}

func newPlanCommentDeliveryService(t *testing.T, durable bool) (*Service, *retentionPromptAgentManager, *mockMessageCreator) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), "s1", models.TaskSessionStateWaitingForInput, ""))
	manager := &retentionPromptAgentManager{
		durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
			mockAgentManager: &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo},
			advertised:       durable,
			capability:       agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: durable},
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	return svc, manager, messages
}

func planCommentDeliveryMetadata(origin string) map[string]interface{} {
	switch origin {
	case "message":
		return map[string]interface{}{
			messagequeue.MetadataDurableTranscriptMessageID: "feedback-message",
			plancomments.MetadataClientMessageFingerprint:   "feedback-message-fingerprint",
		}
	case "queue":
		return map[string]interface{}{
			plancomments.MetadataClientQueueID:      "feedback-queue",
			plancomments.MetadataRequestFingerprint: "feedback-queue-fingerprint",
		}
	default:
		return nil
	}
}
