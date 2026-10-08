package dashboard_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// TestCreateComment_FanOutIncompleteSuppressesLegacyWakeButNotMention pins
// AC-OFFICE-GATE-COMMENT-001.14/.15: when the engine trigger fails with
// ErrCommentFanOutIncomplete (the gate comment fan-out itself could not
// queue every seat), the legacy assignee wake must stay suppressed even
// though the trigger was not "handled" — falling back to the legacy wake
// would wake the runner, which the fan-out was deliberately built to avoid
// waking on its own gate comment. This differs from every other trigger
// error, which keeps today's legacy-wake fallback
// (TestCreateComment_KeepsLegacyWakeAfterNoopEngineDispatch).
func TestCreateComment_FanOutIncompleteSuppressesLegacyWakeButNotMention(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-fanout-incomplete", "ws-1", "Fan-out incomplete", "todo", 1)

	rt := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	sentinelErr := fmt.Errorf("%w: task task-fanout-incomplete step review role \"reviewer\": boom",
		engine.ErrCommentFanOutIncomplete)
	disp := &recordingEngineDispatcher{handled: false, err: &engine.CommentFanOutIncompleteError{
		WorkflowStepID:           "step-review",
		WorkflowStepTransitionID: 42,
		Err:                      sentinelErr,
	}}
	eb := bus.NewMemoryEventBus(logger.Default())
	var eventData map[string]string
	if _, err := eb.Subscribe(events.OfficeCommentCreated, func(_ context.Context, event *bus.Event) error {
		raw, ok := event.Data.(map[string]string)
		if !ok {
			t.Fatalf("event data type = %T, want map[string]string", event.Data)
		}
		eventData = raw
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	deps.svc.SetReactivityApplier(rt)
	deps.svc.SetWorkflowEngineDispatcher(disp)
	deps.svc.SetEventBus(eb)

	comment := &models.TaskComment{
		ID:         "comment-fanout-incomplete",
		TaskID:     "task-fanout-incomplete",
		AuthorType: "user",
		AuthorID:   "user-1",
		Body:       "please decide",
		CreatedAt:  time.Now().UTC(),
	}

	if err := deps.svc.CreateComment(context.Background(), comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(disp.calls))
	}
	if len(rt.calls) != 1 {
		t.Fatalf("reactivity calls = %d, want 1", len(rt.calls))
	}
	if !rt.calls[0].SkipAssigneeCommentWake {
		t.Fatal("SkipAssigneeCommentWake = false, want true after a fan-out-incomplete error")
	}
	if _, hasFlag := eventData["engine_dispatched"]; hasFlag {
		t.Fatalf("engine_dispatched = %q, want absent (trigger was not handled)", eventData["engine_dispatched"])
	}
	if eventData["workflow_step_id"] != "step-review" || eventData["workflow_step_transition_id"] != "42" {
		t.Fatalf("retry identity = (%q, %q), want (step-review, 42)", eventData["workflow_step_id"], eventData["workflow_step_transition_id"])
	}
}

func TestCreateComment_UntypedFanOutErrorSuppressesUnsafeEventRetry(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-untyped-fanout", "ws-1", "Fan-out incomplete", "todo", 1)
	rt := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	sentinelErr := fmt.Errorf("%w: legacy dispatcher error", engine.ErrCommentFanOutIncomplete)
	deps.svc.SetReactivityApplier(rt)
	deps.svc.SetWorkflowEngineDispatcher(&recordingEngineDispatcher{handled: false, err: sentinelErr})
	eb := bus.NewMemoryEventBus(logger.Default())
	var eventData map[string]string
	if _, err := eb.Subscribe(events.OfficeCommentCreated, func(_ context.Context, event *bus.Event) error {
		eventData = event.Data.(map[string]string)
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	deps.svc.SetEventBus(eb)

	comment := &models.TaskComment{
		ID:         "comment-untyped-fanout",
		TaskID:     "task-untyped-fanout",
		AuthorType: "user",
		AuthorID:   "user-1",
		Body:       "please decide",
		CreatedAt:  time.Now().UTC(),
	}
	if err := deps.svc.CreateComment(context.Background(), comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if eventData["engine_retry_suppressed"] != "true" {
		t.Fatalf("engine_retry_suppressed = %q, want true when origin identity is unavailable", eventData["engine_retry_suppressed"])
	}
	if !rt.calls[0].SkipAssigneeCommentWake {
		t.Fatal("legacy assignee wake was not suppressed after incomplete fan-out")
	}
}

// TestCreateComment_OtherEngineErrorKeepsLegacyWake pins that a
// non-sentinel engine error keeps today's behavior: legacy assignee wake
// still fires (SkipAssigneeCommentWake stays false), unlike the
// ErrCommentFanOutIncomplete case above.
func TestCreateComment_OtherEngineErrorKeepsLegacyWake(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-other-engine-error", "ws-1", "Other engine error", "todo", 1)

	rt := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	disp := &recordingEngineDispatcher{handled: false, err: errors.New("infrastructure error")}
	eb := bus.NewMemoryEventBus(logger.Default())
	var eventData map[string]string
	if _, err := eb.Subscribe(events.OfficeCommentCreated, func(_ context.Context, event *bus.Event) error {
		raw, ok := event.Data.(map[string]string)
		if !ok {
			t.Fatalf("event data type = %T, want map[string]string", event.Data)
		}
		eventData = raw
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	deps.svc.SetReactivityApplier(rt)
	deps.svc.SetWorkflowEngineDispatcher(disp)
	deps.svc.SetEventBus(eb)

	comment := &models.TaskComment{
		ID:         "comment-other-engine-error",
		TaskID:     "task-other-engine-error",
		AuthorType: "user",
		AuthorID:   "user-1",
		Body:       "please take a look",
		CreatedAt:  time.Now().UTC(),
	}

	if err := deps.svc.CreateComment(context.Background(), comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("reactivity calls = %d, want 1", len(rt.calls))
	}
	if rt.calls[0].SkipAssigneeCommentWake {
		t.Fatal("SkipAssigneeCommentWake = true, want false for a non-sentinel engine error")
	}
	if _, hasFlag := eventData["engine_dispatched"]; hasFlag {
		t.Fatalf("engine_dispatched = %q, want absent", eventData["engine_dispatched"])
	}
}

// TestCreateComment_AssigneeReadFailureStillReachesEngineWithAuthorIdentity
// pins AC-OFFICE-GATE-COMMENT-002.7: isSelfComment fails open (a task whose
// execution fields cannot be read is never treated as a self-comment), so a
// runner-authored comment on an unreadable task still reaches the engine
// trigger — with the runner's own agent id intact as the payload's
// AuthorID, which is what lets the engine-level author-exclusion filter
// (excludeCommentAuthor) still exclude that seat downstream. This is
// existing isSelfComment behavior (out of scope to change), asserted here
// because REQ-OFFICE-GATE-COMMENT-001's author exclusion depends on it.
func TestCreateComment_AssigneeReadFailureStillReachesEngineWithAuthorIdentity(t *testing.T) {
	deps := newTestDeps(t)
	// Deliberately no insertTestTask call: GetTaskExecutionFields on a
	// nonexistent task id returns ErrTaskNotFound, simulating the
	// assignee-read failure.
	const taskID = "task-no-execution-fields"

	rt := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	disp := &recordingEngineDispatcher{handled: true}
	deps.svc.SetReactivityApplier(rt)
	deps.svc.SetWorkflowEngineDispatcher(disp)

	comment := &models.TaskComment{
		ID:         "comment-runner-authored",
		TaskID:     taskID,
		AuthorType: "agent",
		AuthorID:   "agent-runner",
		Body:       "status update",
		CreatedAt:  time.Now().UTC(),
	}

	if err := deps.svc.CreateComment(context.Background(), comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1 (isSelfComment must fail open, not short-circuit)", len(disp.calls))
	}
	payload, ok := disp.calls[0].payload.(engine.OnCommentPayload)
	if !ok {
		t.Fatalf("payload type = %T, want engine.OnCommentPayload", disp.calls[0].payload)
	}
	if payload.AuthorID != "agent-runner" {
		t.Fatalf("payload.AuthorID = %q, want agent-runner", payload.AuthorID)
	}
}
