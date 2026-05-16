package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// fakeDispatcher records every HandleTrigger call so tests can pin the
// exact trigger + payload + operation id the office service emits.
type fakeDispatcher struct {
	mu    sync.Mutex
	calls []dispatcherCall
	// nextErr lets a test simulate engine.HandleTrigger returning a
	// specific error.
	nextErr error
}

type dispatcherCall struct {
	taskID  string
	trigger engine.Trigger
	payload any
	opID    string
}

func (f *fakeDispatcher) HandleTrigger(
	_ context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, dispatcherCall{taskID, trigger, payload, opID})
	return f.nextErr
}

func (f *fakeDispatcher) Calls() []dispatcherCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]dispatcherCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// TestEngineDispatcher_NoDispatcher_DropsTrigger pins the contract that
// when no dispatcher is wired (e.g. a test that only exercises the
// office service in isolation) comment events do not produce any
// engine calls and do not error.
func TestEngineDispatcher_NoDispatcher_DropsTrigger(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	// Deliberately do NOT call SetWorkflowEngineDispatcher.

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "fix this",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
}

// TestEngineDispatcher_RoutesToDispatcher pins that a comment fires
// engine.HandleTrigger with TriggerOnComment + a typed
// OnCommentPayload when the dispatcher is wired.
func TestEngineDispatcher_RoutesToDispatcher(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)

	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "fix this",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	calls := disp.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 dispatcher call, got %d", len(calls))
	}
	got := calls[0]
	if got.taskID != "task-1" {
		t.Errorf("taskID = %q, want task-1", got.taskID)
	}
	if got.trigger != engine.TriggerOnComment {
		t.Errorf("trigger = %q, want %q", got.trigger, engine.TriggerOnComment)
	}
	payload, ok := got.payload.(engine.OnCommentPayload)
	if !ok {
		t.Fatalf("payload type = %T, want engine.OnCommentPayload", got.payload)
	}
	if payload.CommentID != comment.ID {
		t.Errorf("payload.CommentID = %q, want %q", payload.CommentID, comment.ID)
	}
	if payload.AuthorID != "user-x" {
		t.Errorf("payload.AuthorID = %q, want user-x", payload.AuthorID)
	}
	wantOp := "task_comment:" + comment.ID
	if got.opID != wantOp {
		t.Errorf("operationID = %q, want %q", got.opID, wantOp)
	}
}

// TestEngineDispatcher_NoSession_DropsTrigger pins that when the
// dispatcher returns ErrEngineNoSession the subscriber drops the
// trigger silently — there is no legacy fallback after Phase 4.
func TestEngineDispatcher_NoSession_DropsTrigger(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)

	disp := &fakeDispatcher{nextErr: service.ErrEngineNoSession}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "fix this",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Dispatcher was tried.
	if calls := disp.Calls(); len(calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(calls))
	}
	// No legacy fallback — runs table stays empty.
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("no-session: want 0 runs (no legacy fallback), got %d", len(runs))
	}
}
