package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRestoreQueuedMessageSurvivesCancelledDispatchContext(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-restore", "session-restore", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	queued, err := svc.messageQueue.QueueMessage(context.Background(), "session-restore", "task-restore", "prompt", "", "user", false, nil)
	if err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	taken, ok := svc.messageQueue.TakeQueued(context.Background(), "session-restore")
	if !ok || taken.ID != queued.ID {
		t.Fatalf("take queued message = %#v, %v", taken, ok)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.restoreQueuedMessage(ctx, taken)

	if got := svc.messageQueue.GetStatus(context.Background(), "session-restore").Count; got != 1 {
		t.Fatalf("restored queue count = %d, want 1", got)
	}
	if len(recorded.events) != 1 || recorded.events[0].subject != events.MessageQueueStatusChanged {
		t.Fatalf("queue restore status events = %d, want one queue status event", len(recorded.events))
	}
	if err := recorded.events[0].ctx.Err(); err != nil {
		t.Fatalf("queue restore status context error = %v, want nil", err)
	}
}

func TestTransferQueuedSessionStatePublishesBothSessionStatuses(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-transfer", "session-old", models.TaskSessionStateRunning)
	session, err := repo.GetTaskSession(context.Background(), "session-old")
	if err != nil {
		t.Fatalf("load old session: %v", err)
	}
	session.ID = "session-new"
	if err := repo.CreateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("create new session: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	if _, err := svc.messageQueue.QueueMessage(context.Background(), "session-old", "task-transfer", "prompt", "", "user", false, nil); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	if err := svc.transferQueuedSessionState(context.Background(), "task-transfer", "session-old", "session-new"); err != nil {
		t.Fatalf("transfer queued state: %v", err)
	}

	if got := svc.messageQueue.GetStatus(context.Background(), "session-old").Count; got != 0 {
		t.Fatalf("old queue count = %d, want 0", got)
	}
	if got := svc.messageQueue.GetStatus(context.Background(), "session-new").Count; got != 1 {
		t.Fatalf("new queue count = %d, want 1", got)
	}
	if len(recorded.events) != 2 {
		t.Fatalf("queue transfer status events = %d, want 2", len(recorded.events))
	}
	seen := map[string]bool{}
	for _, event := range recorded.events {
		if event.subject != events.MessageQueueStatusChanged {
			t.Fatalf("queue transfer event subject = %q, want %q", event.subject, events.MessageQueueStatusChanged)
		}
		seen[event.event.Data.(map[string]interface{})["session_id"].(string)] = true
	}
	if !seen["session-old"] || !seen["session-new"] {
		t.Fatalf("queue transfer status sessions = %#v, want both sessions", seen)
	}
}
