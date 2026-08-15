package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestArchiveTaskPublishesQueueStatusWithTaskID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-archive-queue", "session-archive-queue", models.TaskSessionStateIdle)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	svc.eventBus = eventBus

	var saw atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(_ context.Context, event *bus.Event) error {
		data, _ := event.Data.(map[string]interface{})
		if data == nil {
			return nil
		}
		if taskID, _ := data["task_id"].(string); taskID == "task-archive-queue" {
			saw.Add(1)
		}
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, err := svc.messageQueue.QueueMessage(ctx, "session-archive-queue", "task-archive-queue", "queued", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-archive-queue"); err != nil || got != 1 {
		t.Fatalf("pending before archive = %d err=%v, want 1", got, err)
	}

	// Archive through the task repository (same purge path production uses).
	// The service helper used by HTTP is heavier; repository purge is enough
	// to exercise notifyTaskQueuePurged + the wired notifier.
	if err := repo.ArchiveTask(ctx, "task-archive-queue"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	// MemoryEventBus.Publish is synchronous, so the subscriber already ran.
	if saw.Load() == 0 {
		t.Fatal("expected message.queue.status_changed with task_id after archive purge")
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-archive-queue"); err != nil || got != 0 {
		t.Fatalf("pending after archive = %d err=%v, want 0", got, err)
	}
}

func TestDeleteSessionCancelsQueuedPromptsAndPublishesStatus(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-session-queue", "session-keep", models.TaskSessionStateIdle)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-drop", TaskID: "task-session-queue",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-drop): %v", err)
	}

	manager := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	eventBus := bus.NewMemoryEventBus(testLogger())
	svc.eventBus = eventBus

	var saw atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(_ context.Context, event *bus.Event) error {
		data, _ := event.Data.(map[string]interface{})
		if data == nil {
			return nil
		}
		if taskID, _ := data["task_id"].(string); taskID == "task-session-queue" {
			saw.Add(1)
		}
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, err := svc.messageQueue.QueueMessage(ctx, "session-drop", "task-session-queue", "orphan me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage drop: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessage(ctx, "session-keep", "task-session-queue", "keep me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage keep: %v", err)
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-session-queue"); err != nil || got != 2 {
		t.Fatalf("pending before delete = %d err=%v, want 2", got, err)
	}

	if err := svc.DeleteSession(ctx, "session-drop"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if got := svc.messageQueue.GetStatus(ctx, "session-drop").Count; got != 0 {
		t.Fatalf("session-drop queue count = %d, want 0", got)
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-session-queue"); err != nil || got != 1 {
		t.Fatalf("pending after delete = %d err=%v, want 1 (kept session only)", got, err)
	}
	// MemoryEventBus.Publish is synchronous, so the subscriber already ran.
	if saw.Load() == 0 {
		t.Fatal("expected message.queue.status_changed with task_id after session delete")
	}
}

func TestDeleteSessionClearsProjectedAgentError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := setupTestRepo(t)
	const taskID = "task-session-error"
	const sessionID = "session-error"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)

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
	t.Cleanup(func() {
		cancel()
		projector.Close()
	})
	if err := projector.Start(ctx); err != nil {
		t.Fatalf("start status summary projector: %v", err)
	}

	if err := eventBus.Publish(ctx, events.TaskSessionErrorChanged, bus.NewEvent(
		events.TaskSessionErrorChanged,
		"test",
		map[string]interface{}{
			"task_id":     taskID,
			"session_id":  sessionID,
			"active":      true,
			"message":     "agent failed",
			"occurred_at": "2026-08-15T10:00:00Z",
			"stamp":       "error-stamp",
		},
	)); err != nil {
		t.Fatalf("publish active error: %v", err)
	}
	beforeDelete, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("load summary before delete: %v", err)
	}
	if summary := beforeDelete[taskID]; summary == nil || summary.ActiveError == nil {
		t.Fatalf("summary before delete = %+v, want active error", summary)
	}

	if err := svc.DeleteSession(ctx, sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	afterDelete, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("load summary after delete: %v", err)
	}
	if summary := afterDelete[taskID]; summary == nil || summary.ActiveError != nil {
		t.Fatalf("summary after delete = %+v, want deleted session error cleared", summary)
	}
}

func TestRecoverableFailureWaitsForSessionDeletionGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-session-error-guard"
	const sessionID = "session-error-guard"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	lock, release := svc.acquireCancelInFlightGuard(sessionID)
	lock.Lock()

	recoveryDone := make(chan struct{})
	go func() {
		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:       taskID,
			SessionID:    sessionID,
			ErrorMessage: "agent failed",
		})
		close(recoveryDone)
	}()

	select {
	case <-recoveryDone:
		t.Fatal("recoverable failure bypassed the session deletion guard")
	case <-time.After(100 * time.Millisecond):
	}

	lock.Unlock()
	release()
	select {
	case <-recoveryDone:
	case <-time.After(time.Second):
		t.Fatal("recoverable failure did not finish after the guard was released")
	}
}

func TestDeleteSessionRetainsOtherProjectedAgentErrorAfterRestart(t *testing.T) {
	taskID := "task-session-errors"
	deletedSessionID := "session-error-newer"
	retainedSessionID := "session-error-older"
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, retainedSessionID, models.TaskSessionStateCompleted)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: deletedSessionID, TaskID: taskID, State: models.TaskSessionStateCompleted,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", deletedSessionID, err)
	}
	retainedError := models.LastAgentError{
		Message:    retainedSessionID + " failed",
		OccurredAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC),
	}
	deletedError := models.LastAgentError{
		Message:    deletedSessionID + " failed",
		OccurredAt: time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC),
	}
	for sessionID, lastError := range map[string]models.LastAgentError{
		retainedSessionID: retainedError,
		deletedSessionID:  deletedError,
	} {
		if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, lastError); err != nil {
			t.Fatalf("set last agent error for %s: %v", sessionID, err)
		}
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	svc.eventBus = eventBus

	newProjector := func(parent context.Context) context.CancelFunc {
		ctx, cancel := context.WithCancel(parent)
		projector := statussummary.NewProjector(statussummary.ProjectorConfig{
			Store:              repo,
			EventBus:           eventBus,
			ResolveWorkspace:   func(context.Context, string) (string, error) { return "ws1", nil },
			CountQueuedPrompts: svc.messageQueue.CountPendingByTask,
		})
		t.Cleanup(func() {
			cancel()
			projector.Close()
		})
		if err := projector.Start(ctx); err != nil {
			t.Fatalf("start status summary projector: %v", err)
		}
		return cancel
	}

	publishActiveError := func(sessionID, stamp, occurredAt string) {
		t.Helper()
		if err := eventBus.Publish(ctx, events.TaskSessionErrorChanged, bus.NewEvent(
			events.TaskSessionErrorChanged,
			"test",
			map[string]interface{}{
				"task_id":     taskID,
				"session_id":  sessionID,
				"active":      true,
				"message":     sessionID + " failed",
				"occurred_at": occurredAt,
				"stamp":       stamp,
			},
		)); err != nil {
			t.Fatalf("publish active error for %s: %v", sessionID, err)
		}
	}

	cancelFirst := newProjector(ctx)
	publishActiveError(retainedSessionID, "error-older", "2026-08-15T10:00:00Z")
	publishActiveError(deletedSessionID, "error-newer", "2026-08-15T11:00:00Z")
	beforeRestart, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("load summary before restart: %v", err)
	}
	if summary := beforeRestart[taskID]; summary == nil || summary.ActiveError == nil || summary.ActiveError.SessionID != deletedSessionID {
		t.Fatalf("summary before restart = %+v, want newer deleted-session error", summary)
	}
	cancelFirst()

	newProjector(ctx)
	if err := svc.DeleteSession(ctx, deletedSessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	afterDelete, err := repo.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		t.Fatalf("load summary after delete: %v", err)
	}
	summary := afterDelete[taskID]
	if summary == nil || summary.ActiveError == nil || summary.ActiveError.SessionID != retainedSessionID {
		t.Fatalf("summary after delete = %+v, want retained session error", summary)
	}
}

func TestQueueStatusNotifyUsesDetachedContext(t *testing.T) {
	// Production: ArchiveTask commits then calls notify with the request ctx.
	// A cancelled request must not starve the badge-zero publish/handler work.
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-notify-detach", "session-notify-detach", models.TaskSessionStateIdle)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	svc.eventBus = eventBus

	var saw atomic.Int32
	var handlerSawCancelled atomic.Bool
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(handlerCtx context.Context, event *bus.Event) error {
		if handlerCtx.Err() != nil {
			handlerSawCancelled.Store(true)
		}
		data, _ := event.Data.(map[string]interface{})
		if taskID, _ := data["task_id"].(string); taskID == "task-notify-detach" {
			saw.Add(1)
		}
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Replace notifier with one that forces a cancelled ctx into publish,
	// simulating client disconnect between commit and notify.
	repo.SetTaskQueuePurgeNotifier(func(_ context.Context, taskID string) {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		svc.publishTaskQueueStatusEvent(cancelled, taskID, "")
	})

	if _, err := svc.messageQueue.QueueMessage(ctx, "session-notify-detach", "task-notify-detach", "queued", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}
	if err := repo.ArchiveTask(ctx, "task-notify-detach"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	// MemoryEventBus.Publish is synchronous, so the subscriber already ran.
	if saw.Load() == 0 {
		t.Fatal("expected message.queue.status_changed when notify publish uses cancelled ctx")
	}
	if handlerSawCancelled.Load() {
		t.Fatal("handler received cancelled context; publishTaskQueueStatusEvent must detach")
	}
}
