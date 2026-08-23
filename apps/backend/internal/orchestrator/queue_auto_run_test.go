package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type queueAutoRunController interface {
	SetQueueAutoRun(context.Context, string, bool) (bool, bool, error)
}

func requireQueueAutoRunController(t *testing.T, svc *Service) queueAutoRunController {
	t.Helper()
	controller, ok := interface{}(svc).(queueAutoRunController)
	require.True(t, ok, "orchestrator Service must implement queue Auto-run control")
	return controller
}

func TestSetQueueAutoRunOffDoesNotCancelActiveTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentManager := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	_, err := svc.messageQueue.QueueMessage(ctx, "session-1", "task-1", "held", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	autoRun, dispatched, err := requireQueueAutoRunController(t, svc).SetQueueAutoRun(ctx, "session-1", false)
	require.NoError(t, err)
	assert.False(t, autoRun)
	assert.False(t, dispatched)
	assert.Equal(t, int32(0), agentManager.cancelAgentCalls.Load())
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	assert.False(t, status.AutoRun)
	assert.Equal(t, 1, status.Count)
}

func TestSetQueueAutoRunOnDispatchesPromptableHead(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentManager := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	_, err := svc.messageQueue.QueueMessage(ctx, "session-1", "task-1", "resume me", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))

	autoRun, dispatched, err := requireQueueAutoRunController(t, svc).SetQueueAutoRun(ctx, "session-1", true)
	require.NoError(t, err)
	assert.True(t, autoRun)
	assert.True(t, dispatched)
	select {
	case <-agentManager.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resumed queue dispatch")
	}
	assert.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

func TestSetQueueAutoRunOnDoesNotBypassClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	seedPendingClarificationMessage(t, repo, "task-1", "session-1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	_, err := svc.messageQueue.QueueMessage(ctx, "session-1", "task-1", "held", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))

	autoRun, dispatched, err := requireQueueAutoRunController(t, svc).SetQueueAutoRun(ctx, "session-1", true)
	require.NoError(t, err)
	assert.True(t, autoRun)
	assert.False(t, dispatched)
	assert.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

func TestHandleAgentReady_AutoRunOffHoldsACPAndPassthroughQueues(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "acp"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-1", "session-1", "step-1")
			session, err := repo.GetTaskSession(ctx, "session-1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateRunning
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			stepGetter := newMockStepGetter()
			stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Work"}
			agentManager := &mockAgentManager{isPassthrough: passthrough}
			svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentManager)
			_, err = svc.messageQueue.QueueMessage(ctx, "session-1", "task-1", "held", "", messagequeue.QueuedByUser, false, nil)
			require.NoError(t, err)
			require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))

			svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "task-1", SessionID: "session-1"})

			status := svc.messageQueue.GetStatus(ctx, "session-1")
			assert.False(t, status.AutoRun)
			assert.Equal(t, 1, status.Count)
			agentManager.mu.Lock()
			assert.Empty(t, agentManager.capturedPrompts)
			assert.Empty(t, agentManager.passthroughStdinCalls)
			agentManager.mu.Unlock()
		})
	}
}

func TestCIAutomationReplacementTreatsAutoRunOffAsQueuedSuccess(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	const coalesceKey = "ci-feedback"
	metadata := map[string]interface{}{messagequeue.MetadataCoalesceKey: coalesceKey}
	_, _, err := svc.messageQueue.QueueMessageWithCoalesceKey(
		ctx, "session-1", "task-1", "old feedback", "", messagequeue.QueuedByWorkflow,
		false, nil, metadata, coalesceKey, true,
	)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))
	session, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)

	result, err := svc.dispatchCIAutomationPromptToIdleSession(ctx, session, ciAutomationDispatchParams{
		ChatPrompt: "new feedback", CoalesceKey: coalesceKey, Metadata: metadata, AllowNewRound: true,
	})
	require.NoError(t, err)
	assert.Equal(t, ciAutomationDispatchQueuedReplace, result.kind)
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	assert.False(t, status.AutoRun)
	require.Len(t, status.Entries, 1)
	assert.Equal(t, "new feedback", status.Entries[0].Content)
}

func TestLifecyclePromptTreatsAutoRunOffAsQueuedSuccess(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))
	session, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)

	gotSessionID, err := svc.queueAndDrainLifecyclePrompt(
		ctx, session, "task-1", "lifecycle feedback",
		map[string]interface{}{"origin": githubPRAutomationOrigin}, "lifecycle-feedback", assert.AnError,
	)
	require.NoError(t, err)
	assert.Equal(t, "session-1", gotSessionID)
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	assert.False(t, status.AutoRun)
	assert.Equal(t, 1, status.Count)
}
