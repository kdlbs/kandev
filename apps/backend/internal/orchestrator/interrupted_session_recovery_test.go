package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap/zaptest"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type interruptedRecoveryManager struct {
	*mockAgentManager
	terminated bool
}

func (m *interruptedRecoveryManager) RecoverAgentPromptStreamWithIdentity(context.Context, lifecycle.AgentDeliveryRecoveryIdentity) lifecycle.DeliveryReconciliationResult {
	return lifecycle.DeliveryReconciliationResult{Outcome: lifecycle.DeliveryReconciliationUncertain, Reason: "terminal_outcome_missing", ProcessTerminated: m.terminated}
}

func TestInterruptedContinuationRequiresVerifiedTerminationAndRevision(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-interrupted", "session-interrupted", "step1")
	seedExecutorRunning(t, repo, "session-interrupted", "task-interrupted", "old-execution")
	session, err := repo.GetTaskSession(ctx, "session-interrupted")
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1, NativeSessionID: "native", CreationReason: "initial"}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "old", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("old instruction"), State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	recovery := models.AgentDeliveryRecovery{SessionID: session.ID, AgentExecutionID: "old-execution", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, SubmissionID: "old", StreamID: "stream", PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain}
	block := &models.SessionRecoveryBlock{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: "old", DeliveryStreamID: "stream", State: models.RecoveryBlockOpen}
	_, err = repo.UpsertAgentDeliveryRecovery(ctx, &recovery, block)
	require.NoError(t, err)
	manager := &interruptedRecoveryManager{mockAgentManager: &mockAgentManager{}, terminated: true}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	require.NoError(t, repo.DeleteExecutorRunningBySessionID(ctx, session.ID))
	result, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryUncertain, result.Outcome, "durable identity must survive execution-row removal")
	require.Equal(t, []SessionDeliveryRecoveryAction{SessionDeliveryRecoveryActionContinueInterrupted}, result.AllowedActions)
	manager.terminated = false
	result, err = service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Empty(t, result.AllowedActions)
	require.Empty(t, manager.capturedPrompts)
}

func TestInterruptedResumeDispatchesOnlyNewInstructionOnce(t *testing.T) {
	ctx := context.Background()
	svc, request, launches := interruptedResumeFixture(t)
	repo := svc.repo.(*sqliterepo.Repository)
	manager := svc.agentManager.(*interruptedRecoveryManager).mockAgentManager
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	result, err := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryOutcome("continued"), result.Outcome, "native restore must also accept the new instruction")
	result, err = svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryOutcome("continued"), result.Outcome)
	require.Equal(t, 1, *launches)
	require.Equal(t, []string{request.Instruction}, manager.capturedPrompts)
	messages, err := repo.ListMessages(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, messages, 2, "accepted continuation must persist one user instruction alongside the original")
	var continuation *models.Message
	for _, message := range messages {
		if message.Content == request.Instruction {
			require.Nil(t, continuation, "new instruction must appear once")
			continuation = message
		}
	}
	require.NotNil(t, continuation)
	require.Equal(t, models.MessageAuthorUser, continuation.AuthorType)
	require.NotEmpty(t, continuation.TurnID)
	old, err := repo.GetAgentDeliverySubmission(ctx, "old")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, old.State)
	stored, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, session.TaskEnvironmentID, stored.TaskEnvironmentID)

	restarted := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), svc.agentManager)
	batch, err := restarted.ResumeInterruptedSessions(ctx, []InterruptedSessionBatchItem{
		{TaskID: "t1", SessionID: "s1", InterruptedSessionResumeRequest: request},
		{TaskID: "missing", SessionID: "missing", InterruptedSessionResumeRequest: request},
	})
	require.NoError(t, err)
	require.Equal(t, 2, batch.Completed)
	require.Equal(t, SessionDeliveryRecoveryContinued, batch.Results[0].Outcome)
	require.Equal(t, SessionDeliveryRecoveryBlocked, batch.Results[1].Outcome)
	require.Equal(t, 1, *launches)
	require.Equal(t, []string{request.Instruction}, manager.capturedPrompts, "batch retries after restart must retain acceptance")
	messages, err = repo.ListMessages(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, messages, 2, "retries must not duplicate the saved instruction")
	request.Instruction = "different instruction with same key"
	result, err = svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, result.Outcome)
	require.Len(t, manager.capturedPrompts, 1)
}

func (m *interruptedRecoveryManager) DurableDeliveryCapabilityForExecution(_ context.Context, executionID string) (lifecycle.DurableDeliveryCapability, bool) {
	return lifecycle.DurableDeliveryCapability{Version: 1, Durable: true, Unresolved: executionID != "new-execution"}, true
}

func (m *interruptedRecoveryManager) PromptAgentWithAdmissionCallbackAndSubmissionID(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, beforeAdmission func() error, onDispatched func(), submissionID string) (*executor.PromptResult, error) {
	return m.PromptAgentWithAdmissionCallback(ctx, executionID, prompt, attachments, dispatchOnly, beforeAdmission, onDispatched)
}

func interruptedResumeFixture(t *testing.T) (*Service, InterruptedSessionResumeRequest, *int) {
	ctx := context.Background()
	svc, _, _ := continuationFailureFixture(t)
	svc.logger, _ = logger.NewFromZap(zaptest.NewLogger(t))
	repo := svc.repo.(*sqliterepo.Repository)
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo}
	manager := svc.agentManager.(*mockAgentManager)
	svc.agentManager = &interruptedRecoveryManager{mockAgentManager: manager, terminated: true}
	svc.executor = executor.NewExecutor(svc.agentManager, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "s1", "t1", "old-execution")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1, NativeSessionID: "provider-session", CreationReason: "initial"}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "old", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("old instruction"), State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	recovery := models.AgentDeliveryRecovery{SessionID: session.ID, AgentExecutionID: "old-execution", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, SubmissionID: "old", StreamID: "stream", PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain}
	_, err = repo.UpsertAgentDeliveryRecovery(ctx, &recovery, &models.SessionRecoveryBlock{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: "old", DeliveryStreamID: "stream", State: models.RecoveryBlockOpen})
	require.NoError(t, err)
	launches := 0
	manager.isAgentReadyFn = func(context.Context, string) bool { return true }
	manager.getACPSessionIDForSessionFunc = func(string) (string, bool) { return "provider-session", true }
	manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "new-running", TaskID: "t1", SessionID: "s1", AgentExecutionID: "new-execution", ResumeToken: "provider-session", Resumable: true, Status: "ready"}))
		require.Equal(t, "provider-session", req.ACPSessionID)
		require.Equal(t, "provider-session", req.RequiredNativeConversationID)
		require.Empty(t, req.TaskDescription)
		require.EqualValues(t, 2, req.DeliveryHarnessGeneration)
		require.Equal(t, "old", req.InterruptedSubmissionID)
		require.False(t, req.ForceContextContinuation)
		_, _, updateErr := repo.UpdateTaskSessionStateIfCurrent(ctx, req.SessionID, models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput, "")
		require.NoError(t, updateErr)
		manager.isAgentRunning = true
		return &executor.LaunchAgentResponse{AgentExecutionID: "new-execution"}, nil
	}
	request := InterruptedSessionResumeRequest{Acknowledge: true, RecoveryRevision: recovery.Revision, RecoveryIdentity: SessionDeliveryRecoveryIdentity{SubmissionID: "old", StreamID: "stream", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, PromptGeneration: 1}, Instruction: "Inspect the retained changes and continue from them", IdempotencyKey: "resume-one"}
	return svc, request, &launches
}
