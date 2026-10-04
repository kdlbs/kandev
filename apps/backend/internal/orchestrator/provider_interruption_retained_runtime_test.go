package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

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
	require.Contains(t, mgr.capturedPrompts[0], "Continue the unfinished request")
	require.NotContains(t, mgr.capturedPrompts[0], "test original request")
}

func TestRetainedContinuationQueuedUserWorkWins(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := installContinuationRestoreFixture(t, svc)
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.AgentProfileID = "profile-1"
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
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
