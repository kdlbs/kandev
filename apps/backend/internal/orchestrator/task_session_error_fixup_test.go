package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type recoverableFailureReadErrorRepo struct {
	sessionExecutorStore
	err  error
	once sync.Once
}

func (r *recoverableFailureReadErrorRepo) GetTaskSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	failed := false
	r.once.Do(func() { failed = true })
	if failed {
		return nil, r.err
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

func TestHandleRecoverableFailureContinuesAfterTransientSessionReadError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-session-error-read"
	const sessionID = "session-error-read"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)

	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	svc.repo = &recoverableFailureReadErrorRepo{
		sessionExecutorStore: svc.repo,
		err:                  errors.New("temporary session read failure"),
	}

	svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: "exec-read-error",
		ErrorMessage:     "agent failed while loading session",
	})

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.Message != "agent failed while loading session" {
		t.Fatalf("last agent error = %#v, want persisted recovery error", lastError)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want WAITING_FOR_INPUT", session.State)
	}
	if len(messageCreator.sessionMessages) == 0 {
		t.Fatal("recoverable failure did not create a recovery status message")
	}
	waitForStopCall(t, agentManager)
}

type blockingTaskSessionErrorBus struct {
	bus.EventBus
	taskID    string
	sessionID string
	entered   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (b *blockingTaskSessionErrorBus) Publish(
	ctx context.Context,
	subject string,
	event *bus.Event,
) error {
	if subject == events.TaskSessionErrorChanged {
		data, _ := event.Data.(map[string]interface{})
		active, _ := data["active"].(bool)
		if data["task_id"] == b.taskID && data["session_id"] == b.sessionID && active {
			b.once.Do(func() {
				close(b.entered)
				<-b.release
			})
		}
	}
	return b.EventBus.Publish(ctx, subject, event)
}

func TestConcurrentSessionDeletionDoesNotRepublishDeletedSessionError(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-session-error-delete-race"
	const firstSessionID = "session-error-delete-first"
	const secondSessionID = "session-error-delete-second"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, firstSessionID, models.TaskSessionStateCompleted)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: secondSessionID, TaskID: taskID, State: models.TaskSessionStateCompleted,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", secondSessionID, err)
	}
	for sessionID, lastError := range map[string]models.LastAgentError{
		firstSessionID: {
			Message:    firstSessionID + " failed",
			OccurredAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC),
		},
		secondSessionID: {
			Message:    secondSessionID + " failed",
			OccurredAt: time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC),
		},
	} {
		if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, lastError); err != nil {
			t.Fatalf("set last agent error for %s: %v", sessionID, err)
		}
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	svc.eventBus = eventBus
	projector := statussummary.NewProjector(statussummary.ProjectorConfig{
		Store:              repo,
		EventBus:           eventBus,
		ResolveWorkspace:   func(context.Context, string) (string, error) { return "ws1", nil },
		CountQueuedPrompts: svc.messageQueue.CountPendingByTask,
	})
	projectorCtx, stopProjector := context.WithCancel(ctx)
	t.Cleanup(func() {
		stopProjector()
		projector.Close()
	})
	if err := projector.Start(projectorCtx); err != nil {
		t.Fatalf("start status summary projector: %v", err)
	}
	for sessionID, stamp := range map[string]string{
		firstSessionID:  "error-first",
		secondSessionID: "error-second",
	} {
		if err := eventBus.Publish(ctx, events.TaskSessionErrorChanged, bus.NewEvent(
			events.TaskSessionErrorChanged,
			"test",
			map[string]interface{}{
				"task_id":     taskID,
				"session_id":  sessionID,
				"active":      true,
				"message":     sessionID + " failed",
				"occurred_at": "2026-08-15T10:00:00Z",
				"stamp":       stamp,
			},
		)); err != nil {
			t.Fatalf("publish active error for %s: %v", sessionID, err)
		}
	}

	errorBus := &blockingTaskSessionErrorBus{
		EventBus:  eventBus,
		taskID:    taskID,
		sessionID: secondSessionID,
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	svc.eventBus = errorBus
	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.DeleteSession(ctx, firstSessionID) }()
	select {
	case <-errorBus.entered:
	case <-time.After(time.Second):
		t.Fatal("first deletion did not reach retained-error selection barrier")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- svc.DeleteSession(ctx, secondSessionID) }()
	secondFinishedBeforeRelease := false
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second DeleteSession: %v", err)
		}
		secondFinishedBeforeRelease = true
	case <-time.After(100 * time.Millisecond):
	}
	close(errorBus.release)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first DeleteSession: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first deletion did not finish after releasing barrier")
	}
	if !secondFinishedBeforeRelease {
		select {
		case err := <-secondDone:
			if err != nil {
				t.Fatalf("second DeleteSession: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("second deletion did not finish after releasing barrier")
		}
	}

	summaries, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("load summary after concurrent deletion: %v", err)
	}
	if summary := summaries[taskID]; summary == nil || summary.ActiveError != nil {
		t.Fatalf("summary after concurrent deletion = %+v, want no deleted-session error", summary)
	}
}
