package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// TestQueueRestartRestoresPendingEntriesWithoutStartingAgent pins the restart
// rule: backend startup may repair queue state, but it never starts or resumes
// an agent to deliver it.
func TestQueueRestartRestoresPendingEntriesWithoutStartingAgent(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	queue.SetMergeEnabled(false)
	queue.SetAutoMergeEnabled(false)
	first, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "first queued message", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	second, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "second queued message", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	// Send Now forces Auto-run ON for a claimed entry, so the restart fixture
	// exercises the setting that could otherwise auto-deliver.
	claim, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*second})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	agentManager := &mockAgentManager{}
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue, agentManager: agentManager}

	require.NoError(t, restarted.reconcilePendingSendNowClaimsOnStartup(ctx))

	status := restartedQueue.GetStatus(ctx, "session-1")
	require.True(t, status.AutoRun, "Auto-run must survive the restart unchanged")
	require.Len(t, status.Entries, 2, "startup restores the claimed entry")
	require.Equal(t, first.Content, status.Entries[0].Content)
	require.Equal(t, second.Content, status.Entries[1].Content)
	require.Equal(t, claim.Dispatch.Content, second.Content)

	require.Empty(t, agentManager.capturedPrompts, "startup must not prompt an agent")
	require.Empty(t, agentManager.startAgentProcessCalls, "startup must not start an agent process")
	_, pending := restarted.queuedDispatchDrainPending.Load("session-1")
	require.False(t, pending, "startup must not schedule a delivery drain")
}

// TestQueueRestartResumeReadinessDeliversEachEntryOnce pins the second half of
// the rule: once the user opens the task and the session is ready, Auto-run
// delivers each pending entry exactly once under existing FIFO policy.
func TestQueueRestartResumeReadinessDeliversEachEntryOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	setSessionWaiting(t, repo, "session-1")

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	completed := make(chan struct{}, 2)
	svc.onQueuedMessageExecutionComplete = func() { completed <- struct{}{} }

	queue := svc.messageQueue
	queue.SetMergeEnabled(false)
	queue.SetAutoMergeEnabled(false)
	first, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "first queued message", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	second, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "second queued message", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	require.NoError(t, queue.SetAutoRun(ctx, "session-1", true))

	for _, want := range []string{first.Content, second.Content} {
		require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, "session-1"),
			"readiness must deliver the next pending entry")
		select {
		case <-completed:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the queued dispatch worker")
		}
		require.Contains(t, agentMgr.capturedPrompts[len(agentMgr.capturedPrompts)-1], want)
		// The generated turn ends before the next queue boundary; real
		// completion events put the session back in this state.
		setSessionWaiting(t, repo, "session-1")
	}

	require.Len(t, agentMgr.capturedPrompts, 2, "each entry runs once")
	require.Equal(t, 0, queue.GetStatus(ctx, "session-1").Count)
}

func setSessionWaiting(t *testing.T, repo interface {
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	UpdateTaskSession(context.Context, *models.TaskSession) error
}, sessionID string) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
}
