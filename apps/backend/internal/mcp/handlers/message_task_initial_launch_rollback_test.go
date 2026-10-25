package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.3
func TestPeerMessageInitialLaunch_RollbackFencesTaskOnlyProgress(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	initialTurn, err := svc.StartTurn(ctx, session.ID)
	require.NoError(t, err)
	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "before-message"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc, repo)
	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	older, err := orch.queue.QueueMessageWithMetadataForSession(
		ctx, identity, "older queued work", "", messagequeue.QueuedByAgent, false, nil,
		map[string]interface{}{"position": 0},
	)
	require.NoError(t, err)

	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	startErr := errors.New("runtime configuration failed")
	orch.peerStartFunc = func(
		_ context.Context,
		_ messagequeue.QueueSessionIdentity,
		_, _ string,
		_, _, _ bool,
		_ []v1.MessageAttachment,
		_ []v1.EntityReference,
	) (*executor.TaskExecution, error) {
		close(startEntered)
		<-releaseStart
		return nil, startErr
	}
	t.Cleanup(func() {
		select {
		case <-releaseStart:
		default:
			close(releaseStart)
		}
	})
	dispatchDone := make(chan error, 1)
	go func() {
		_, dispatchErr := h.dispatchTaskMessage(
			ctx, target.ID, session, "follow-up that loses task ownership",
			map[string]interface{}{"sender_task_id": sender.ID}, false, false,
		)
		dispatchDone <- dispatchErr
	}()
	select {
	case <-startEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("peer message did not reach the controlled launch boundary")
	}

	winningTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	winningTask.State = v1.TaskStateFailed
	winningTask.WorkflowStepID = "winning-step"
	require.NoError(t, repo.UpdateTask(ctx, winningTask))
	winnerQueueEntry, err := orch.queue.QueueMessageWithMetadataForSession(
		ctx, identity, "winning queued work", "", messagequeue.QueuedByAgent, false, nil,
		map[string]interface{}{"position": 1},
	)
	require.NoError(t, err)
	close(releaseStart)

	select {
	case dispatchErr := <-dispatchDone:
		require.ErrorIs(t, dispatchErr, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("peer-message dispatch did not return after the launch failed")
	}

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateFailed, updatedTask.State)
	assert.Equal(t, "winning-step", updatedTask.WorkflowStepID)
	updatedSession, err := svc.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateCreated, updatedSession.State)
	activeTurn, err := svc.GetActiveTurn(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	assert.Equal(t, initialTurn.ID, activeTurn.ID)
	queueEntries := orch.queue.GetStatus(ctx, session.ID).Entries
	require.Len(t, queueEntries, 2)
	assert.Equal(t, older.ID, queueEntries[0].ID)
	assert.Equal(t, winnerQueueEntry.ID, queueEntries[1].ID)
	messages, err := svc.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, messages, "a failed follow-up must not remain as an undispatched user message")
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.3
func TestPeerMessageInitialLaunch_RollbackFencesSiblingSessionProgressDuringPreparation(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	require.NoError(t, repo.UpdateTask(ctx, task))
	sibling := &models.TaskSession{
		ID: "sibling-launch-session", TaskID: target.ID, State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "sibling-before", IsPrimary: false,
		Metadata: map[string]interface{}{"owner": "before"},
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sibling))
	require.NoError(t, repo.UpdateSessionMetadata(ctx, sibling.ID, sibling.Metadata))
	h, orch := newMessageTaskHandler(t, svc, repo)
	siblingIdentity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, sibling.ID)
	require.NoError(t, err)
	olderEntry, err := orch.queue.QueueMessageWithMetadataForSession(
		ctx, siblingIdentity, "older sibling work", "", messagequeue.QueuedByAgent, false, nil, nil,
	)
	require.NoError(t, err)

	var preparationErr error
	orch.onTurnStart = func(ctx context.Context, _, _ string) error {
		current, err := svc.GetTaskSession(ctx, sibling.ID)
		if err != nil {
			return err
		}
		current.AgentProfileID = "sibling-winning-profile"
		current.Metadata = map[string]interface{}{"owner": "sibling launch"}
		if err := repo.UpdateTaskSession(ctx, current); err != nil {
			return err
		}
		if err := repo.UpdateSessionMetadata(ctx, sibling.ID, current.Metadata); err != nil {
			return err
		}
		_, preparationErr = orch.queue.QueueMessageWithMetadataForSession(
			ctx, siblingIdentity, "sibling winning work", "", messagequeue.QueuedByAgent, false, nil, nil,
		)
		return preparationErr
	}
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	startErr := errors.New("runtime configuration failed")
	orch.peerStartFunc = func(
		_ context.Context,
		_ messagequeue.QueueSessionIdentity,
		_, _ string,
		_, _, _ bool,
		_ []v1.MessageAttachment,
		_ []v1.EntityReference,
	) (*executor.TaskExecution, error) {
		close(startEntered)
		<-releaseStart
		return nil, startErr
	}
	t.Cleanup(func() {
		select {
		case <-releaseStart:
		default:
			close(releaseStart)
		}
	})
	dispatchDone := make(chan error, 1)
	go func() {
		_, dispatchErr := h.dispatchTaskMessage(
			ctx, target.ID, session, "follow-up that loses launch ownership",
			map[string]interface{}{"sender_task_id": sender.ID}, false, false,
		)
		dispatchDone <- dispatchErr
	}()
	select {
	case <-startEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("peer message did not reach the controlled launch boundary")
	}
	require.NoError(t, preparationErr)
	close(releaseStart)
	select {
	case dispatchErr := <-dispatchDone:
		require.ErrorIs(t, dispatchErr, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("peer-message dispatch did not return after the launch failed")
	}

	updatedSibling, err := svc.GetTaskSession(ctx, sibling.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateWaitingForInput, updatedSibling.State)
	assert.Equal(t, "sibling-winning-profile", updatedSibling.AgentProfileID)
	assert.Equal(t, "sibling launch", updatedSibling.Metadata["owner"])
	entries := orch.queue.GetStatus(ctx, sibling.ID).Entries
	require.Len(t, entries, 2)
	assert.Equal(t, olderEntry.ID, entries[0].ID)
	assert.Equal(t, "sibling winning work", entries[1].Content)
}
