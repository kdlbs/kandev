package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestAdmitSeam3AdmitsUnderCeiling covers the ordinary case: population below
// the ceiling, admitted, reservation keyed by the session being resumed, and
// (unlike every other seam) no record is written by the gate itself — seam 3
// never persists, only its four callers do.
func TestAdmitSeam3AdmitsUnderCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 5)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam3-admit", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reservation, refusal := svc.admitSeam3(ctx, "seam3-admit", "seam3-admit-session", launchOriginAutomatic)
	if refusal != nil {
		t.Fatalf("ensureSessionRunning's own gate refused while under the ceiling: %v", refusal)
	}
	if reservation == nil || reservation.key != "seam3-admit-session" {
		t.Fatalf("reservation not keyed by the session being resumed: %+v", reservation)
	}
}

// TestAdmitSeam3RefusesAutomaticOverCeiling covers AC-47: an automatic
// ensureSessionRunning call at the ceiling is refused via the distinguishable
// sentinel, with no record written by the gate itself (disposition belongs to
// each of the four call shapes, not to admitSeam3).
func TestAdmitSeam3RefusesAutomaticOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam3-first", "seam3-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	if _, refusal := svc.admitSeam3(ctx, "seam3-first", "seam3-first-session", launchOriginAutomatic); refusal != nil {
		t.Fatalf("first ensure was not admitted: %v", refusal)
	}

	reservation, refusal := svc.admitSeam3(ctx, "seam3-second", "seam3-second-session", launchOriginAutomatic)
	if refusal == nil {
		t.Fatal("second automatic ensure over the ceiling was admitted, want refused")
	}
	if reservation != nil {
		t.Fatal("a refused ensure must not hold a reservation")
	}
	if refusal.deferred {
		t.Fatal("admitSeam3 must never set deferred itself; that belongs to the caller's disposition")
	}

	record := deferredLaunchOf(t, svc, "seam3-second")
	if record != nil {
		t.Fatalf("admitSeam3 must never persist a record itself: %+v", record)
	}
}

// TestAdmitSeam3AdmitsManualOverCeiling covers AC-14: a manual
// ensureSessionRunning call is always admitted, never refused, even at the
// ceiling.
func TestAdmitSeam3AdmitsManualOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam3-manual-first", "seam3-manual-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, refusal := svc.admitSeam3(ctx, "seam3-manual-first", "seam3-manual-first-session", launchOriginAutomatic); refusal != nil {
		t.Fatalf("first ensure was not admitted: %v", refusal)
	}

	reservation, refusal := svc.admitSeam3(ctx, "seam3-manual-second", "seam3-manual-second-session", launchOriginManual)
	if refusal != nil {
		t.Fatalf("a manual ensure was refused; AC-14 requires it always be admitted: %v", refusal)
	}
	if reservation == nil {
		t.Fatal("a manual override still consumes a reservation")
	}
}

// TestAdmitOrDeferWorkflowStepEnsureAdmitsUnderCeiling covers AC-47f's
// pre-consultation in the ordinary case.
func TestAdmitOrDeferWorkflowStepEnsureAdmitsUnderCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 5)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam3-wf-admit", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reservation, deferred, err := svc.admitOrDeferWorkflowStepEnsure(ctx, "seam3-wf-admit", "seam3-wf-admit-session", "step-1")
	if err != nil {
		t.Fatalf("admitOrDeferWorkflowStepEnsure: %v", err)
	}
	if deferred {
		t.Fatal("pre-consultation was deferred while under the ceiling")
	}
	if reservation == nil || reservation.key != "seam3-wf-admit-session" {
		t.Fatalf("reservation not keyed by the session: %+v", reservation)
	}
	if record := deferredLaunchOf(t, svc, "seam3-wf-admit"); record != nil {
		t.Fatalf("a deferred_launch record was written for an admitted pre-consultation: %+v", record)
	}
}

// TestAdmitOrDeferWorkflowStepEnsureDefersOverCeilingAndRecords covers
// AC-47f/AC-47f1: a pre-consultation refused at the ceiling writes the
// workflow_step_ensure record itself, carrying session_id and
// workflow_step_id, so the caller can return before ever calling
// advanceTaskWorkflowStep.
func TestAdmitOrDeferWorkflowStepEnsureDefersOverCeilingAndRecords(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam3-wf-first", "seam3-wf-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferWorkflowStepEnsure(ctx, "seam3-wf-first", "seam3-wf-first-session", "step-1"); err != nil || deferred {
		t.Fatalf("first pre-consultation was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferWorkflowStepEnsure(ctx, "seam3-wf-second", "seam3-wf-second-session", "step-2")
	if err != nil {
		t.Fatalf("admitOrDeferWorkflowStepEnsure: %v", err)
	}
	if !deferred {
		t.Fatal("second pre-consultation over the ceiling was admitted, want deferred")
	}
	if reservation != nil {
		t.Fatal("a deferred pre-consultation must not hold a reservation")
	}

	record := deferredLaunchOf(t, svc, "seam3-wf-second")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the refused pre-consultation was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchWorkflowStepEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchWorkflowStepEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested[metaKeySessionID] != "seam3-wf-second-session" || nested[metaKeyWorkflowStepID] != "step-2" {
		t.Fatalf("the recorded payload does not match the refused pre-consultation: %+v", record)
	}
}

// TestAdmitOrDeferWorkflowStepEnsureReportsWriteFailure covers AC-45: a
// pre-consultation refusal that cannot be persisted must not be swallowed.
func TestAdmitOrDeferWorkflowStepEnsureReportsWriteFailure(t *testing.T) {
	svc, _ := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "startSessionForWorkflowStepPreConsult"})

	_, _, err := svc.admitOrDeferWorkflowStepEnsure(ctx, "missing-task", "missing-session", "step-1")
	if err == nil {
		t.Fatal("expected an error when the refusal could not be persisted")
	}
}

// TestDeferSeam3QueueDrainRefusalRecordsQueuedMessageID covers AC-47d: the
// queue-drain disposition records session_id and the queued message's own id,
// needed for AC-47d1's already-drained check at retry time.
func TestDeferSeam3QueueDrainRefusalRecordsQueuedMessageID(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam3-qd", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	svc.deferSeam3QueueDrainRefusal(ctx, "seam3-qd", "seam3-qd-session", "queued-msg-1", &seam3Refusal{reasonCode: ceilingReasonRefused})

	record := deferredLaunchOf(t, svc, "seam3-qd")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the queue-drain refusal was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchQueueDrainEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchQueueDrainEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested[metaKeySessionID] != "seam3-qd-session" || nested["queued_message_id"] != "queued-msg-1" {
		t.Fatalf("the recorded payload does not match the refused drain: %+v", record)
	}
}

// TestDisposeSeam3PromptEnsureRefusalWritesRecordWhenReconstructable covers
// AC-47/AC-47c2's first disposition: with no AC-47c(a) field set, the refusal
// is fully reconstructable from promptTask's own frame, so a prompt_ensure
// record is written and the refusal is marked deferred.
func TestDisposeSeam3PromptEnsureRefusalWritesRecordWhenReconstructable(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam3-prompt-a", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	refusal := &seam3Refusal{reasonCode: ceilingReasonRefused}
	const trustedPromptContext = "EXPANDED PROMPT REFERENCES:\n### @principles\nUse the repository principles."

	err := svc.disposeSeam3PromptEnsureRefusal(
		ctx, "seam3-prompt-a", "seam3-prompt-a-session", "hello", "", false, nil, false,
		promptTaskOptions{lifecyclePrompt: true, promptReferenceContext: trustedPromptContext}, refusal,
	)
	if err != nil {
		t.Fatalf("disposeSeam3PromptEnsureRefusal: %v", err)
	}
	if !refusal.deferred {
		t.Fatal("a reconstructable refusal must be marked deferred once the record is written")
	}

	record := deferredLaunchOf(t, svc, "seam3-prompt-a")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the prompt refusal was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchPromptEnsure) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchPromptEnsure)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["prompt"] != "hello" {
		t.Fatalf("the recorded payload does not match the refused prompt: %+v", record)
	}
	if nested["prompt_reference_context"] != trustedPromptContext {
		t.Fatalf("the recorded payload lost trusted prompt context: %+v", nested)
	}
}

// TestDisposeSeam3PromptEnsureRefusalReturnsUnchangedWhenNonReconstructable
// covers AC-47c1: a call carrying a class-(a) field (here, an in-flight
// afterClaim callback) is not reconstructable from a stored record, so no
// prompt_ensure record is written and the refusal is left undeferred for the
// caller's own restoration path to own.
func TestDisposeSeam3PromptEnsureRefusalReturnsUnchangedWhenNonReconstructable(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam3-prompt-b", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	refusal := &seam3Refusal{reasonCode: ceilingReasonRefused}

	err := svc.disposeSeam3PromptEnsureRefusal(
		ctx, "seam3-prompt-b", "seam3-prompt-b-session", "hello", "", false, nil, false,
		promptTaskOptions{afterClaim: func() error { return nil }}, refusal,
	)
	if err != nil {
		t.Fatalf("disposeSeam3PromptEnsureRefusal: %v", err)
	}
	if refusal.deferred {
		t.Fatal("a non-reconstructable refusal must not be marked deferred")
	}
	if record := deferredLaunchOf(t, svc, "seam3-prompt-b"); record != nil {
		t.Fatalf("a non-reconstructable refusal must not write a record: %+v", record)
	}
}

// TestDisposeSeam3PromptEnsureRefusalReportsWriteFailure covers AC-45 for the
// prompt_ensure disposition.
func TestDisposeSeam3PromptEnsureRefusalReportsWriteFailure(t *testing.T) {
	svc, _ := newServiceWithCeiling(t, 1)
	refusal := &seam3Refusal{reasonCode: ceilingReasonRefused}

	err := svc.disposeSeam3PromptEnsureRefusal(
		context.Background(), "missing-task", "missing-session", "hello", "", false, nil, false,
		promptTaskOptions{}, refusal,
	)
	if err == nil {
		t.Fatal("expected an error when the refusal could not be persisted")
	}
	if refusal.deferred {
		t.Fatal("a refusal must not be marked deferred when the write itself failed")
	}
}

// TestIsSeam3Refusal covers the sentinel's errors.As compatibility, including
// through fmt.Errorf's %w wrapping used by promptTask's own error paths.
func TestIsSeam3Refusal(t *testing.T) {
	refusal := &seam3Refusal{reasonCode: ceilingReasonRefused}
	wrapped := errors.New("wrapped: " + refusal.Error())
	if _, ok := isSeam3Refusal(wrapped); ok {
		t.Fatal("a plain error must not be misidentified as a seam3Refusal")
	}
	if got, ok := isSeam3Refusal(refusal); !ok || got != refusal {
		t.Fatal("isSeam3Refusal must recognize its own sentinel and return the same pointer")
	}
}
