package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type clarificationDeliveryTestManager struct {
	*durableDeliveryTestAgentManager
	submissionID string
	onSubmission func(context.Context, string)
}

func (m *clarificationDeliveryTestManager) PromptAgentWithDispatchCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, onDispatched func(), submissionID string,
) (*executor.PromptResult, error) {
	m.submissionID = submissionID
	m.onSubmission(ctx, submissionID)
	return m.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, onDispatched)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
func TestDetachedClarificationPersistsDeliveryBeforeDispatch(t *testing.T) {
	for _, test := range []struct {
		name     string
		rejected bool
		legacy   bool
	}{
		{name: "answer"},
		{name: "rejection", rejected: true},
		{name: "legacy event", legacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-answer", "session-answer", "step-1")
			seedExecutorRunning(t, repo, "session-answer", "task-answer", "execution-answer")
			require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-answer", models.TaskSessionStateWaitingForInput, ""))
			base := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
			var admitted *models.AgentDeliverySubmission
			manager := &clarificationDeliveryTestManager{
				durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
					mockAgentManager: base,
					advertised:       true,
					capability:       agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
				},
				onSubmission: func(ctx context.Context, id string) {
					var err error
					admitted, err = repo.GetAgentDeliverySubmission(ctx, id)
					require.NoError(t, err, "the backend record must exist before agentctl receives the prompt")
					require.Equal(t, models.DeliverySubmissionDispatching, admitted.State)
				},
			}
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			service.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
			service.turnService = &repoBackedTurnService{repo: repo}
			request := clarification.DetachedClarificationResume{
				TaskID: "task-answer", SessionID: "session-answer", PendingID: "pending-answer",
				Question: "Continue?", AnswerText: "Continue", Rejected: test.rejected, RejectReason: "Skip",
			}
			if test.legacy {
				require.NoError(t, service.resumeDetachedClarification(ctx, clarificationAnsweredData{
					TaskID: request.TaskID, SessionID: request.SessionID, PendingID: request.PendingID,
					Question: request.Question, AnswerText: request.AnswerText,
				}))
			} else {
				require.NoError(t, service.ResumeDetachedClarification(ctx, request))
			}
			require.NotEmpty(t, manager.submissionID, "clarification prompts must use a backend-owned delivery identity")
			require.NotNil(t, admitted)
			require.Equal(t, request.SessionID, admitted.SessionID)
			require.Equal(t, journal.SubmissionHash(admitted.Payload), admitted.PayloadHash)
			var payload agentDeliveryPromptPayload
			require.NoError(t, json.Unmarshal(admitted.Payload, &payload))
			require.Equal(t, []string{payload.Text}, base.capturedPrompts)
			if !test.legacy {
				stored, err := repo.GetAgentDeliverySubmission(ctx, manager.submissionID)
				require.NoError(t, err)
				require.Equal(t, models.DeliverySubmissionDispatching, stored.State,
					"accepting an asynchronous answer is not proof that the prompt completed")
			}
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
func TestDetachedClarificationBlocksUnresolvedDelivery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-unresolved-answer", "session-unresolved-answer", "step-1")
	seedExecutorRunning(t, repo, "session-unresolved-answer", "task-unresolved-answer", "execution-answer")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-unresolved-answer", models.TaskSessionStateWaitingForInput, ""))
	base := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	manager := &durableDeliveryTestAgentManager{
		mockAgentManager: base,
		advertised:       true,
		capability: agentruntime.DurableDeliveryCapability{
			Version: journal.CurrentVersion, Durable: true, Unresolved: true,
		},
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	service.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	service.turnService = &repoBackedTurnService{repo: repo}
	err := service.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID: "task-unresolved-answer", SessionID: "session-unresolved-answer", PendingID: "pending-answer",
		Question: "Continue?", AnswerText: "Continue",
	})
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)
	require.Empty(t, base.capturedPrompts)
	block, err := service.GetOpenSessionRecoveryBlock(ctx, "session-unresolved-answer")
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, durableDeliveryUnresolvedReason, block.Reason)
}
