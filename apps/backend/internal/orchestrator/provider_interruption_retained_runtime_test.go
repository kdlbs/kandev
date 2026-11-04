package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type retainedDeliveryTestManager struct {
	*durableDeliveryTestAgentManager
	submissionID string
	onSubmission func(context.Context, string)
}

func (m *retainedDeliveryTestManager) PromptAgentWithAdmissionCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment,
	dispatchOnly bool, beforeAdmission func(context.Context, string, []v1.MessageAttachment) error, onDispatched func(), submissionID string,
) (*executor.PromptResult, error) {
	m.submissionID = submissionID
	admit := func(ctx context.Context, prompt string, attachments []v1.MessageAttachment) error {
		if beforeAdmission != nil {
			if err := beforeAdmission(ctx, prompt, attachments); err != nil {
				return err
			}
		}
		m.onSubmission(ctx, submissionID)
		return nil
	}
	return m.PromptAgentWithAdmissionCallback(ctx, executionID, prompt, attachments, dispatchOnly, admit, onDispatched)
}

func TestRetainedContinuationPersistsSubmissionBeforeDispatch(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	base := installContinuationRestoreFixture(t, svc)
	base.currentPromptExecutionID = "execution-1"
	repo := svc.repo.(*sqliterepo.Repository)
	mgr := &retainedDeliveryTestManager{
		durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
			mockAgentManager: base, advertised: true,
			capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
		},
		onSubmission: func(ctx context.Context, id string) {
			stored, err := repo.GetAgentDeliverySubmission(ctx, id)
			require.NoError(t, err)
			require.Equal(t, "s1", stored.SessionID)
			require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
		},
	}
	svc.agentManager = mgr
	svc.executor = executor.NewExecutor(mgr, repo, testLogger(), executor.ExecutorConfig{})
	require.NoError(t, repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	require.Contains(t, mgr.submissionID, "prompt:continuation:")
	stored, err := repo.GetAgentDeliverySubmission(context.Background(), mgr.submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, stored.State,
		"dispatch acceptance must wait for the durable terminal event before settlement")
	require.Len(t, base.capturedPromptCalls, 1)
	require.True(t, base.capturedPromptCalls[0].DispatchOnly)
}

func TestContinuationUsesRetainedRuntime(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return nil, errors.New("retained continuation unexpectedly relaunched the runtime")
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime

	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	require.NotNil(t, entry.retainedRuntime)
	t.Cleanup(entry.cancel)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs, "a live continuation must not stop the preserved execution")
	require.Empty(t, mgr.stopAgentArgs)
	require.Len(t, mgr.capturedPromptCalls, 1)
	require.Equal(t, "execution-1", mgr.capturedPromptCalls[0].ExecutionID)
	require.True(t, mgr.capturedPromptCalls[0].DispatchOnly,
		"the retry owner settles at prompt acceptance and does not wait for the provider turn")
	require.Equal(t, "continue", mgr.capturedPrompts[0])
	require.NotContains(t, mgr.capturedPrompts[0], "test original request")
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.8
func TestInterruptionContinuationPendingToolsRetained(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.currentPromptExecutionID = "execution-1"
	mgr.currentPromptGeneration.Store(7)
	mgr.getACPSessionIDForSessionFunc = func(string) (string, bool) {
		return "provider-session", true
	}
	mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		t.Fatal("pending-tool continuation must not launch a replacement runtime")
		return nil, nil
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = realPeerDisconnected
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: 7,
		Known: true, Pending: true, PermissionsResolved: true, CompletedTools: 1,
	}

	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	require.Equal(t, continuationPolicy("agent_managed_pending_live"), entry.continuationPolicy)
	require.NotNil(t, entry.retainedRuntime)
	t.Cleanup(entry.cancel)
	require.Equal(t, retainedRuntimeRetryUsable,
		svc.retainedRuntimeRetryDisposition(entry.retryCtx, "t1", "s1", entry))
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
	require.Len(t, mgr.capturedPromptCalls, 1)
	require.Equal(t, "execution-1", mgr.capturedPromptCalls[0].ExecutionID)
	require.True(t, mgr.capturedPromptCalls[0].DispatchOnly)
	require.Equal(t, "continue", mgr.capturedPrompts[0])
	require.Equal(t, "provider-session", entry.continuation.nativeID)
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.8
func TestInterruptionContinuationPendingRuntimeLoss(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.currentPromptExecutionID = "execution-1"
	mgr.currentPromptGeneration.Store(7)
	launches := 0
	mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		return &executor.LaunchAgentResponse{AgentExecutionID: "replacement"}, nil
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = realPeerDisconnected
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: 7,
		Known: true, Pending: true, PermissionsResolved: true, CompletedTools: 1,
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.True(t, entry.claim())

	mgr.isAgentRunningFn = func(context.Context, string) bool { return false }
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	require.Zero(t, launches, "lost retained execution cannot fall through to native restoration")
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
	require.Empty(t, mgr.capturedPromptCalls)
	var failure *mockSessionMessage
	for i := range mc.sessionMessages {
		message := &mc.sessionMessages[i]
		if message.metadata["runtime_retained"] == true && message.metadata["recovery_disposition"] == recoveryDispositionRefused {
			failure = message
		}
	}
	require.NotNil(t, failure)
	require.Equal(t, 0, failure.metadata["attempts_started"])
	require.Equal(t, "unsupported_restore", failure.metadata["recovery_reason"])
}

func TestPendingContinuationRefusalPersistsActualStartedAttempts(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.currentPromptExecutionID = "execution-1"
	mgr.currentPromptGeneration.Store(7)
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = realPeerDisconnected
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: 7,
		Known: true, Pending: true, PermissionsResolved: true,
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	entry.mu.Lock()
	entry.started = 2
	entry.mu.Unlock()

	svc.finishRetainedRetryWithoutDispatch(context.Background(), "t1", "s1", entry, recoveryDispositionRefused)

	var failure *mockSessionMessage
	for i := range mc.sessionMessages {
		message := &mc.sessionMessages[i]
		if message.metadata["runtime_retained"] == true && message.metadata["recovery_disposition"] == recoveryDispositionRefused {
			failure = message
		}
	}
	require.NotNil(t, failure)
	require.Equal(t, 2, failure.metadata["attempts_started"])
	require.Equal(t, "unsupported_restore", failure.metadata["recovery_reason"])
}

func TestRetainedContinuationQueuedUserWorkWins(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.currentPromptExecutionID = "execution-1"
	mgr.currentPromptGeneration.Store(7)
	mgr.getACPSessionIDForSessionFunc = func(string) (string, bool) {
		return "provider-session", true
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = realPeerDisconnected
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: 7,
		Known: true, Pending: true, PermissionsResolved: true,
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	_, err := svc.messageQueue.QueueMessage(context.Background(), "s1", "t1", "human queued prompt", "", "user", false, nil)
	require.NoError(t, err)

	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
	require.Empty(t, mgr.capturedPromptCalls, "the automatic continuation must yield to queued human work")
}

func TestRetainedContinuationCancellationPersistsWaitingState(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.currentPromptExecutionID = "execution-1"
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)

	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	session, err = svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, "provider-session", session.DownstreamACPSessionID)
	require.Equal(t, int32(1), mgr.cancelAgentCalls.Load())
	require.Empty(t, mgr.stopAgentArgs)
	require.Empty(t, mgr.stopAgentWithReasonArgs)
}

func TestContinuationCancellationSettlesWithoutWorkflowCompletion(t *testing.T) {
	svc, _, _ := continuationFailureFixture(t)
	require.NoError(t, svc.repo.UpdateTaskSessionState(t.Context(), "s1", models.TaskSessionStateRunning, ""))
	session, err := svc.repo.GetTaskSession(t.Context(), "s1")
	require.NoError(t, err)
	ctx := context.WithValue(t.Context(), continuationCancelContextKey{}, &transientRetryEntry{})
	require.NoError(t, svc.finishCancelledAgentTurn(ctx, "s1", cancelAgentPreparation{
		session: session, completionEligible: false,
	}))
	settled, err := svc.repo.GetTaskSession(t.Context(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, settled.State, "confirmed cancellation parks the session even without workflow completion")
}
