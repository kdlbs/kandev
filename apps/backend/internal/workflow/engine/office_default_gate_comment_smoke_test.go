package engine

import (
	"context"
	"errors"
	"testing"
)

// This file is the on_comment counterpart to office_default_smoke_test.go:
// it drives TriggerOnComment against the real, embedded office-default
// template (REQ-OFFICE-GATE-COMMENT-001/002/004) using the same
// loadEmbeddedTemplate/compileWorkflow/smokeStore/smokeParticipants fixtures,
// proving the shipped YAML's on_comment fan-out — not a hand-built action —
// behaves as specified once compiled and evaluated by the real engine.

// newGateCommentEngine wires an engine against the real office-default
// template's compiled steps with only the dependencies a comment-trigger
// fan-out needs: the participant fan-out callback, plus the decision store
// backing skip_decided.
func newGateCommentEngine(store *smokeStore, queue RunQueueAdapter, parts ParticipantStore, decisions DecisionStore) *Engine {
	registry := MapRegistry{
		ActionQueueRunForEachParticipant: QueueRunForEachParticipantCallback{
			Adapter:      queue,
			Participants: parts,
			Decisions:    decisions,
		},
	}
	return New(store, registry,
		WithRunQueue(queue),
		WithParticipantStore(parts),
		WithDecisionStore(decisions),
	)
}

// TestOfficeDefaultWorkflow_OnCommentExcludesAuthorWakesOtherReviewer proves
// AC-OFFICE-GATE-COMMENT-001.1/002.1 against the shipped template: a Review
// comment from one seated reviewer queues a run for the other undecided
// reviewer only, and a comment from a non-seated author (a coordinator)
// wakes both.
func TestOfficeDefaultWorkflow_OnCommentExcludesAuthorWakesOtherReviewer(t *testing.T) {
	ctx := context.Background()
	tmpl := loadEmbeddedTemplate(t, "office-default")
	steps := compileWorkflow(tmpl)

	store := newSmokeStore(steps)
	store.setCurrentStep(steps["review"].ID)
	queue := &fakeRunQueue{}
	parts := newSmokeParticipants(steps)
	decisions := newFakeDecisionStore()
	eng := newGateCommentEngine(store, queue, parts, decisions)

	if _, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnComment,
		OperationID: "op-comment-author",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "rev-A"},
	}); err != nil {
		t.Fatalf("Review.on_comment (author=reviewer): %v", err)
	}
	if got := countCallsByReasonAndStep(queue, "task_comment", steps["review"].ID); got != 1 {
		t.Fatalf("task_comment calls after author's own comment = %d, want 1 (author excluded)", got)
	}
	if queue.calls[0].AgentProfileID != "rev-B" {
		t.Errorf("queued agent = %q, want the other reviewer %q", queue.calls[0].AgentProfileID, "rev-B")
	}
	assertPayloadStageType(t, queue, steps["review"].ID, "review")

	queue.calls = nil
	if _, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnComment,
		OperationID: "op-comment-coordinator",
		Payload:     OnCommentPayload{CommentID: "c-2", AuthorID: "coordinator-1"},
	}); err != nil {
		t.Fatalf("Review.on_comment (author=coordinator): %v", err)
	}
	if got := countCallsByReasonAndStep(queue, "task_comment", steps["review"].ID); got != 2 {
		t.Fatalf("task_comment calls after coordinator's comment = %d, want 2 (both reviewers)", got)
	}
}

// TestOfficeDefaultWorkflow_OnCommentSkipsDecidedReviewer proves the shipped
// template's skip_decided: true actually excludes a reviewer who has
// already recorded a decision at Review (AC-OFFICE-GATE-COMMENT-001.1).
func TestOfficeDefaultWorkflow_OnCommentSkipsDecidedReviewer(t *testing.T) {
	ctx := context.Background()
	tmpl := loadEmbeddedTemplate(t, "office-default")
	steps := compileWorkflow(tmpl)

	store := newSmokeStore(steps)
	store.setCurrentStep(steps["review"].ID)
	queue := &fakeRunQueue{}
	parts := newSmokeParticipants(steps)
	decisions := newFakeDecisionStore()
	eng := newGateCommentEngine(store, queue, parts, decisions)

	if err := decisions.RecordStepDecision(ctx, DecisionInfo{
		TaskID: "task-1", StepID: steps["review"].ID,
		ParticipantID: "reviewer-1", Decision: DecisionApproved,
	}); err != nil {
		t.Fatalf("record reviewer-1 decision: %v", err)
	}

	if _, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnComment,
		OperationID: "op-comment-decided",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "coordinator-1"},
	}); err != nil {
		t.Fatalf("Review.on_comment: %v", err)
	}
	if got := countCallsByReasonAndStep(queue, "task_comment", steps["review"].ID); got != 1 {
		t.Fatalf("task_comment calls = %d, want 1 (decided reviewer skipped)", got)
	}
	if queue.calls[0].AgentProfileID != "rev-B" {
		t.Errorf("queued agent = %q, want the undecided reviewer %q", queue.calls[0].AgentProfileID, "rev-B")
	}
}

// TestOfficeDefaultWorkflow_OnCommentApprovalWakesOnlyApprover proves
// AC-OFFICE-GATE-COMMENT-001.2: a comment at Approval fans out over
// approvers via the shipped stage_type: approval payload, never touching
// the Review step's reviewer seats (a different step in the same template).
func TestOfficeDefaultWorkflow_OnCommentApprovalWakesOnlyApprover(t *testing.T) {
	ctx := context.Background()
	tmpl := loadEmbeddedTemplate(t, "office-default")
	steps := compileWorkflow(tmpl)

	store := newSmokeStore(steps)
	store.setCurrentStep(steps["approval"].ID)
	queue := &fakeRunQueue{}
	parts := newSmokeParticipants(steps)
	decisions := newFakeDecisionStore()
	eng := newGateCommentEngine(store, queue, parts, decisions)

	if _, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnComment,
		OperationID: "op-comment-approval",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "coordinator-1"},
	}); err != nil {
		t.Fatalf("Approval.on_comment: %v", err)
	}
	if got := countCallsByReasonAndStep(queue, "task_comment", steps["approval"].ID); got != 1 {
		t.Fatalf("task_comment calls for approval step = %d, want 1", got)
	}
	if queue.calls[0].AgentProfileID != "app-A" {
		t.Errorf("queued agent = %q, want the approver %q", queue.calls[0].AgentProfileID, "app-A")
	}
	assertPayloadStageType(t, queue, steps["approval"].ID, "approval")
}

// dedupingFlakyRunQueue is a RunQueueAdapter test double modelling the real
// run queue's idempotency-key dedup: a second QueueRun call carrying an
// IdempotencyKey already seen is a silent no-op (matching the coalescing
// internal/runs/service.Service already provides), while failOnceForAgent
// names one agent profile whose first call fails, simulating a transient
// per-seat enqueue failure ahead of a subscriber redispatch.
type dedupingFlakyRunQueue struct {
	failOnceForAgent string
	failed           bool
	calls            []QueueRunRequest
	seenKeys         map[string]bool
}

func (q *dedupingFlakyRunQueue) QueueRun(_ context.Context, req QueueRunRequest) (QueueOutcome, error) {
	if q.seenKeys == nil {
		q.seenKeys = map[string]bool{}
	}
	if q.seenKeys[req.IdempotencyKey] {
		return QueueOutcomeQueued, nil
	}
	if req.AgentProfileID == q.failOnceForAgent && !q.failed {
		q.failed = true
		return "", errors.New("transient queue failure")
	}
	q.seenKeys[req.IdempotencyKey] = true
	q.calls = append(q.calls, req)
	return QueueOutcomeQueued, nil
}

// TestOfficeDefaultWorkflow_OnCommentPartialFailureRedispatchQueuesOncePerSeat
// proves AC-OFFICE-GATE-COMMENT-001.13: one seat's enqueue failure does not
// abort the fan-out to its sibling (AC-C1 collect-and-continue), the failed
// trigger is retryable (the engine never marks the operation applied when
// processActions returns an error), and a same-comment redispatch with the
// same OperationID ends with exactly one queued run per undecided seat —
// the already-succeeded seat's retry is a no-op via its stable idempotency
// key, not a duplicate.
func TestOfficeDefaultWorkflow_OnCommentPartialFailureRedispatchQueuesOncePerSeat(t *testing.T) {
	ctx := context.Background()
	tmpl := loadEmbeddedTemplate(t, "office-default")
	steps := compileWorkflow(tmpl)

	store := newSmokeStore(steps)
	store.setCurrentStep(steps["review"].ID)
	queue := &dedupingFlakyRunQueue{failOnceForAgent: "rev-B"}
	parts := newSmokeParticipants(steps)
	decisions := newFakeDecisionStore()
	eng := newGateCommentEngine(store, queue, parts, decisions)

	in := HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnComment,
		OperationID: "op-comment-redispatch",
		Payload:     OnCommentPayload{CommentID: "c-1", AuthorID: "coordinator-1"},
	}

	_, err := eng.HandleTrigger(ctx, in)
	if err == nil {
		t.Fatal("first dispatch: expected an error from the failing seat, got nil")
	}
	if !errors.Is(err, ErrCommentFanOutIncomplete) {
		t.Fatalf("first dispatch error = %v, want it to wrap ErrCommentFanOutIncomplete", err)
	}
	if len(queue.calls) != 1 || queue.calls[0].AgentProfileID != "rev-A" {
		t.Fatalf("after first dispatch, calls = %+v, want exactly one call for rev-A", queue.calls)
	}

	applied, err := store.IsOperationApplied(ctx, in.OperationID)
	if err != nil {
		t.Fatalf("check operation applied: %v", err)
	}
	if applied {
		t.Fatal("operation marked applied despite a fan-out error; redispatch would be silently dropped")
	}

	// Subscriber redispatch: same comment, same OperationID.
	if _, err := eng.HandleTrigger(ctx, in); err != nil {
		t.Fatalf("redispatch: %v", err)
	}

	if len(queue.calls) != 2 {
		t.Fatalf("total task_comment calls after redispatch = %d, want 2 (one per seat, no duplicate)", len(queue.calls))
	}
	seenAgents := map[string]int{}
	for _, c := range queue.calls {
		if c.Reason != "task_comment" || c.WorkflowStepID != steps["review"].ID {
			t.Fatalf("unexpected queued call outside the review task_comment fan-out: %+v", c)
		}
		seenAgents[c.AgentProfileID]++
	}
	if seenAgents["rev-A"] != 1 || seenAgents["rev-B"] != 1 {
		t.Fatalf("per-agent call counts = %+v, want exactly one call each for rev-A and rev-B", seenAgents)
	}
}
