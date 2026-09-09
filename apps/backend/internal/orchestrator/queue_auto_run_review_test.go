package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

type pauseFailingQueueRepository struct {
	messagequeue.Repository
	err error
}

func (r *pauseFailingQueueRepository) PauseAutoRunIfPending(context.Context, string) (bool, error) {
	return false, r.err
}

func TestCancelAgent_ContinuesReconciliationWhenAutoRunPauseFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-cancel-pause-failure", "session-cancel-pause-failure", "step1")
	svc := createEngineService(t, repo, cancelCompletionStepGetter(true, false), &mockAgentManager{})
	svc.messageQueue = messagequeue.NewService(&pauseFailingQueueRepository{
		Repository: messagequeue.NewMemoryRepository(),
		err:        errors.New("persist Auto-run policy"),
	}, messagequeue.DefaultMaxPerSession, testLogger())
	creator := &mockMessageCreator{}
	svc.messageCreator = creator

	_, err := svc.messageQueue.QueueMessage(
		ctx, "session-cancel-pause-failure", "task-cancel-pause-failure", "queued", "", "user", false, nil,
	)
	require.NoError(t, err)
	require.NoError(t, svc.CancelAgent(ctx, "session-cancel-pause-failure"))

	task, err := repo.GetTask(ctx, "task-cancel-pause-failure")
	require.NoError(t, err)
	require.Equal(t, "step2", task.WorkflowStepID, "policy persistence must not skip workflow reconciliation")
	require.Len(t, creator.sessionMessages, 1, "policy persistence must not skip the cancellation status")
	require.Equal(t, "Turn cancelled by user", creator.sessionMessages[0].content)
}

func TestAutoStartStepPrompt_LeavesHandoffQueuedWhenAutoRunPaused(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-paused-handoff"
		sessionID = "session-paused-handoff"
		handoff   = "Wait for my approval"
		autoStart = "Run the next workflow step"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-paused-handoff")

	queued, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, handoff, "", "user", false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, sessionID, false))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	err = svc.autoStartStepPrompt(
		ctx,
		taskID,
		session,
		&wfmodels.WorkflowStep{ID: "step-next", WorkflowID: "wf1", Name: "Next"},
		autoStart,
		false,
		false,
	)
	require.NoError(t, err)

	status := svc.messageQueue.GetStatus(ctx, sessionID)
	require.False(t, status.AutoRun)
	require.Len(t, status.Entries, 1)
	require.Equal(t, queued.ID, status.Entries[0].ID)
	require.Len(t, agentMgr.capturedPrompts, 1)
	require.True(t, strings.Contains(agentMgr.capturedPrompts[0], autoStart))
	require.False(t, strings.Contains(agentMgr.capturedPrompts[0], handoff))
}

func TestPublishTaskQueueStatusEventIncludesQueuePolicy(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-policy-event", "session-policy-event", models.TaskSessionStateIdle)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-policy-event", false))
	svc.messageQueue.SetMergeEnabled(false)

	svc.publishTaskQueueStatusEvent(ctx, "task-policy-event", "session-policy-event")

	require.Len(t, eventBus.events, 1)
	data, ok := eventBus.events[0].event.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, false, data["auto_run"])
	require.Equal(t, false, data["merge_enabled"])
}
