package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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

// errCancelledResumeDispatch is the shape a queued dispatch returns when Stop
// cancels the resume attempt that was bringing the session up for it.
var errCancelledResumeDispatch = fmt.Errorf(
	"failed to ensure session is running: %w: attempt-7", ErrResumeAttemptCancelled,
)

func assertCancelledResumeRestored(
	t *testing.T,
	ctx context.Context,
	queue *messagequeue.Service,
	sessionID, queueID string,
) {
	t.Helper()
	pending, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending dispatch claims after a cancelled resume = %#v, want none "+
			"(a left-over claim is parked as unknown_queue_dispatch_outcome on the next boot)", pending)
	}
	status := queue.GetStatus(ctx, sessionID)
	if len(status.Entries) != 1 || status.Entries[0].ID != queueID {
		t.Fatalf("queue after a cancelled resume = %#v, want the original entry %s back in the queue",
			status.Entries, queueID)
	}
}

// TestCancelledResumeRestoresOrdinaryQueuedMessage pins
// AC-TASKS-RESUME-PROMPT-QUEUE-003.1: a queued message whose dispatch ends with
// ErrResumeAttemptCancelled before anything reached the agent goes back to the
// queue instead of being left claimed and logged as dropped.
func TestCancelledResumeRestoresOrdinaryQueuedMessage(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = db.Close() })
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "follow-up prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	service := &Service{logger: testLogger(), messageQueue: queue}

	service.finishQueuedMessageExecution(
		ctx, "session-1", "session-1", reserved, nil, false, false, false, errCancelledResumeDispatch,
	)

	assertCancelledResumeRestored(t, ctx, queue, "session-1", source.ID)
}

// TestCancelledResumeRestoresExactReservation pins the exact-session half of
// AC-TASKS-RESUME-PROMPT-QUEUE-003.1: a dispatch that carries the session
// incarnation restores onto that same incarnation, and the entry can be
// reserved again by the next delivery trigger.
func TestCancelledResumeRestoresExactReservation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step1")
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	identity, err := queue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	if err != nil {
		t.Fatalf("resolve queue identity: %v", err)
	}
	if identity.SessionIncarnationID == "" {
		t.Fatalf("queue identity has no incarnation: %#v", identity)
	}
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "follow-up prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	service := &Service{logger: testLogger(), messageQueue: queue}
	reservation := service.newQueuedDispatchReservation(ctx, "session-1", reserved.ID, identity, reserved)

	service.finishQueuedMessageExecution(
		ctx, "session-1", "session-1", reserved, reservation, false, false, false, errCancelledResumeDispatch,
	)

	status := queue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("queue after a cancelled resume = %#v, want the original entry %s back in the queue",
			status.Entries, source.ID)
	}
	again, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || again.ID != source.ID {
		t.Fatalf("re-reserve after a cancelled resume = %#v, ok=%t; the entry is still claimed", again, ok)
	}
}
