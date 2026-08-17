package engine_adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/steptelemetry"
)

type fakeFirstStepResolver struct {
	stepID string
	err    error
}

func (f *fakeFirstStepResolver) ResolveStartStep(_ context.Context, _ string) (string, error) {
	return f.stepID, f.err
}

type fakeMover struct {
	calls []struct {
		TaskID, WorkflowID, StepID string
		Position                   int
		Attribution                steptelemetry.Attribution
	}
	err error
}

func (f *fakeMover) AddTaskToWorkflow(ctx context.Context, taskID, workflowID, stepID string, position int) error {
	f.calls = append(f.calls, struct {
		TaskID, WorkflowID, StepID string
		Position                   int
		Attribution                steptelemetry.Attribution
	}{TaskID: taskID, WorkflowID: workflowID, StepID: stepID, Position: position, Attribution: steptelemetry.FromContext(ctx)})
	return f.err
}

func TestWorkflowSwitcherAdapter_ExplicitStep(t *testing.T) {
	mover := &fakeMover{}
	resolver := &fakeFirstStepResolver{stepID: "should-not-be-called"}
	a := NewWorkflowSwitcherAdapter(resolver, mover)
	got, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "step-explicit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "step-explicit" {
		t.Errorf("resolved step = %q, want step-explicit", got)
	}
	if len(mover.calls) != 1 || mover.calls[0].StepID != "step-explicit" {
		t.Errorf("mover call = %+v", mover.calls)
	}
}

func TestWorkflowSwitcherAdapter_BlankStepUsesResolver(t *testing.T) {
	mover := &fakeMover{}
	resolver := &fakeFirstStepResolver{stepID: "first-step"}
	a := NewWorkflowSwitcherAdapter(resolver, mover)
	got, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first-step" {
		t.Errorf("resolved step = %q, want first-step", got)
	}
	if len(mover.calls) != 1 || mover.calls[0].StepID != "first-step" {
		t.Errorf("mover step = %s, want first-step", mover.calls[0].StepID)
	}
}

func TestWorkflowSwitcherAdapter_RequiresTaskID(t *testing.T) {
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "", "wf-2", "step-1")
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected task_id error, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_RequiresWorkflowID(t *testing.T) {
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "", "step-1")
	if err == nil || !strings.Contains(err.Error(), "workflow_id") {
		t.Fatalf("expected workflow_id error, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_BubblesResolverError(t *testing.T) {
	resolverErr := errors.New("resolve boom")
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{err: resolverErr}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err == nil || !errors.Is(err, resolverErr) {
		t.Fatalf("expected resolver error to bubble, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_ResolverReturnsEmpty(t *testing.T) {
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{stepID: ""}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err == nil || !strings.Contains(err.Error(), "no runnable first step") {
		t.Fatalf("expected first-step error, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_BubblesMoverError(t *testing.T) {
	moveErr := errors.New("update boom")
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{stepID: "s"}, &fakeMover{err: moveErr})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err == nil || !errors.Is(err, moveErr) {
		t.Fatalf("expected mover error to bubble, got: %v", err)
	}
}

// TestWorkflowSwitcherAdapter_PrefersCausalSessionAttribution proves the
// fix for Review round 2's must-fix #1: SwitchWorkflowCallback.Execute
// (the sole production caller) wraps ctx with the causal session's
// attribution before calling SwitchTaskWorkflow, because a switch_workflow
// action is always genuinely caused by a session's turn (Execute validates
// SessionID is non-empty). The adapter must forward the ACTOR side of that
// attribution to the mover rather than recomputing actor_kind from the
// (session-less) authn seam, which would silently record actor_kind=system
// for a session-caused transition.
//
// The preset's Trigger is deliberately wrong (TriggerManualMove, never
// workflow_attached in production) so that asserting the mover received
// TriggerWorkflowAttached proves the adapter actually HARDCODES the trigger
// — the doc comment on workflowAttachedAttribution's own claim — rather
// than merely round-tripping whatever the caller happened to preset. Review
// round 3 mutation-verified the earlier version of this test (a preset that
// already matched what was asserted) could not fail even if the hardcode
// were replaced with a pass-through.
func TestWorkflowSwitcherAdapter_PrefersCausalSessionAttribution(t *testing.T) {
	mover := &fakeMover{}
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{stepID: "s"}, mover)
	presetCtx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerManualMove,
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   "sess-1",
		SessionID: "sess-1",
	})
	if _, err := a.SwitchTaskWorkflow(presetCtx, "task-1", "wf-2", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mover.calls) != 1 {
		t.Fatalf("expected 1 mover call, got %d", len(mover.calls))
	}
	got := mover.calls[0].Attribution
	if got.Trigger != steptelemetry.TriggerWorkflowAttached {
		t.Errorf("trigger = %q, want %q (must be hardcoded, not passed through from a wrong preset)", got.Trigger, steptelemetry.TriggerWorkflowAttached)
	}
	if got.ActorKind != steptelemetry.ActorAgent {
		t.Errorf("actor_kind = %q, want %q (the resolved session caused this transition)", got.ActorKind, steptelemetry.ActorAgent)
	}
	if got.ActorID != "sess-1" {
		t.Errorf("actor_id = %q, want sess-1", got.ActorID)
	}
	if got.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", got.SessionID)
	}
}

// TestWorkflowSwitcherAdapter_FallsBackToAuthnSeamWithoutPreset covers the
// no-preset case: a caller that doesn't set attribution (e.g. a future,
// non-engine caller of this adapter) still gets a sensible default from the
// existing authn seam, exactly like genesisAttribution/detachAttribution do
// in the sqlite repository.
func TestWorkflowSwitcherAdapter_FallsBackToAuthnSeamWithoutPreset(t *testing.T) {
	mover := &fakeMover{}
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{stepID: "s"}, mover)
	if _, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mover.calls) != 1 {
		t.Fatalf("expected 1 mover call, got %d", len(mover.calls))
	}
	got := mover.calls[0].Attribution
	if got.Trigger != steptelemetry.TriggerWorkflowAttached {
		t.Errorf("trigger = %q, want %q", got.Trigger, steptelemetry.TriggerWorkflowAttached)
	}
	if got.ActorKind != steptelemetry.ActorSystem {
		t.Errorf("actor_kind = %q, want %q (no identity on ctx, no preset attribution)", got.ActorKind, steptelemetry.ActorSystem)
	}
	if got.ActorID != "" {
		t.Errorf("actor_id = %q, want empty", got.ActorID)
	}
}
