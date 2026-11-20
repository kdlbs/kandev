package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-COORDINATOR-SHADOW-DREAM-002.4
// @covers AC-COORDINATOR-SHADOW-DREAM-003.2
func TestQueuedDreamLaunchWaitsForSessionAndExecutorReadinessBeforePrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "capacity-task", "capacity-session", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "capacity-session", "capacity-task", "capacity-execution")
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "session1", "task1", "dream-execution")
	dreamSession, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	dreamSession.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, dreamSession))

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "capacity-task", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "task1", v1.TaskStateInProgress)
	var agentReady atomic.Bool
	readyCheckEntered := make(chan struct{})
	var readyCheckOnce sync.Once
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		isAgentReadyFn: func(_ context.Context, sessionID string) bool {
			if sessionID == "session1" {
				readyCheckOnce.Do(func() { close(readyCheckEntered) })
			}
			return agentReady.Load()
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.turnService = &repoTurnService{repo: repo}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	_, err = svc.StartCreatedSession(ctx, "capacity-task", "capacity-session", "profile-1", "go", false, false, true, nil, nil)
	require.NoError(t, err)
	agentMgr.mu.Lock()
	priorPromptCount := len(agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()
	priorMessageCount := len(messages.userMessages)

	launch, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: "task1", SessionID: "session1", Intent: IntentStartCreated,
		AgentProfileID: "profile-1", AutoStart: true, NoInitialPrompt: true, SkipMessageRecord: true,
	})
	require.NoError(t, err)
	require.NotNil(t, launch)
	require.Equal(t, activationDispositionQueued, launch.ActivationDisposition)
	require.Equal(t, string(models.TaskSessionStateCreated), launch.State)

	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCreated, session.State)
	require.Equal(t, 0, openTurnCount(t, repo, "session1"), "queued no-prompt startup must not create a turn")
	require.Len(t, messages.userMessages, priorMessageCount, "queued no-prompt startup must not record a user message")
	persistedMessages, err := repo.ListMessages(ctx, "session1")
	require.NoError(t, err)
	require.Empty(t, persistedMessages, "queued no-prompt startup must not persist a message")
	agentMgr.mu.Lock()
	require.Len(t, agentMgr.capturedPrompts, priorPromptCount, "queued no-prompt startup must not dispatch a prompt")
	agentMgr.mu.Unlock()

	// The queued session is admitted after the occupying execution leaves the
	// counted population. Drive that persisted lifecycle transition directly so
	// the test can hold the executor at its readiness boundary.
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "capacity-session", models.TaskSessionStateCompleted, ""))
	svc.sessionCeiling.release("capacity-session")

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- svc.WaitForSessionPromptable(ctx, "task1", "session1")
	}()
	// The deferred launch becomes prompt-ready only after its executor reports
	// WAITING_FOR_INPUT. Agent readiness remains independently controlled below.
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session1", models.TaskSessionStateWaitingForInput, ""))
	select {
	case <-readyCheckEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("readiness wait did not reach the executor readiness boundary")
	}

	agentMgr.mu.Lock()
	require.Empty(t, agentMgr.capturedPrompts, "the evidence prompt must wait for executor readiness")
	agentMgr.mu.Unlock()
	require.Equal(t, 0, openTurnCount(t, repo, "session1"), "waiting for readiness must not open a prompt turn")
	require.Len(t, messages.userMessages, priorMessageCount, "waiting for readiness must not record the evidence prompt")
	persistedMessages, err = repo.ListMessages(ctx, "session1")
	require.NoError(t, err)
	require.Empty(t, persistedMessages, "waiting for readiness must not persist the evidence prompt")

	agentReady.Store(true)
	select {
	case err := <-waitDone:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("promptability wait did not finish after executor readiness")
	}

	_, err = svc.PromptTask(ctx, "task1", "session1", "dream evidence", "", false, nil, false)
	require.NoError(t, err)
	agentMgr.mu.Lock()
	require.Len(t, agentMgr.capturedPrompts, priorPromptCount+1)
	require.Equal(t, "dream evidence", agentMgr.capturedPrompts[priorPromptCount])
	agentMgr.mu.Unlock()
	require.Equal(t, 1, openTurnCount(t, repo, "session1"), "the evidence prompt must create exactly one turn")
}
