package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestOfficeDefaultWorkflow_FullCycleSmoke is the Phase 6 (ADR-0004) end-to-end
// smoke test for the office-default template. It walks a task through
// Work -> Review -> Approval -> Done by:
//
//   - executing the Work step's on_turn_complete (move_to_step: review),
//   - recording a unanimous reviewer "approved" decisions and re-evaluating
//     Review.on_turn_complete (move_to_step: approval gated on all_approve),
//   - recording a unanimous approver "approved" decisions and re-evaluating
//     Approval.on_turn_complete (move_to_step: done gated on all_approve).
//
// The Review on_enter actions (clear_decisions + queue_run_for_each_participant)
// are also exercised when the engine evaluates the trigger.
func TestOfficeDefaultWorkflow_FullCycleSmoke(t *testing.T) {
	ctx := context.Background()
	tmpl := loadEmbeddedTemplate(t, "office-default")

	steps := compileWorkflow(tmpl)
	store := newSmokeStore(steps)
	queue := &fakeRunQueue{}
	parts := newSmokeParticipants(steps)
	decisions := newFakeDecisionStore()

	registry := MapRegistry{
		ActionAutoStartAgent: noOpCallback{},
		ActionQueueRun: QueueRunCallback{
			Adapter:      queue,
			Primary:      stubPrimary{id: "agent-primary"},
			CEOResolver:  stubCEO{id: "agent-ceo"},
			Participants: parts,
		},
		ActionQueueRunForEachParticipant: QueueRunForEachParticipantCallback{
			Adapter:      queue,
			Participants: parts,
		},
		ActionClearDecisions: ClearDecisionsCallback{Decisions: decisions},
	}
	eng := New(store, registry,
		WithRunQueue(queue),
		WithParticipantStore(parts),
		WithDecisionStore(decisions),
	)

	// 1. Task lands on Work.
	store.setCurrentStep(steps["work"].ID)

	// Work.on_turn_complete -> move to Review.
	res, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnTurnComplete,
		OperationID: "op-work-complete",
	})
	if err != nil {
		t.Fatalf("Work.on_turn_complete: %v", err)
	}
	if !res.Transitioned || res.ToStepID != steps["review"].ID {
		t.Fatalf("expected transition to Review, got %#v", res)
	}
	store.setCurrentStep(steps["review"].ID)

	// Review.on_enter clears decisions and fans out runs to reviewers.
	if _, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnEnter,
		OperationID: "op-review-enter",
	}); err != nil {
		t.Fatalf("Review.on_enter: %v", err)
	}
	if got := countCallsByReason(queue, "review_started"); got != 2 {
		t.Errorf("review_started fan-out calls = %d, want 2", got)
	}

	// Reviewers approve.
	for _, pID := range []string{"reviewer-1", "reviewer-2"} {
		if err := decisions.RecordStepDecision(ctx, DecisionInfo{
			TaskID: "task-1", StepID: steps["review"].ID,
			ParticipantID: pID, Decision: DecisionApproved,
		}); err != nil {
			t.Fatalf("record decision for %s: %v", pID, err)
		}
	}

	// Review.on_turn_complete -> move to Approval (all_approve satisfied).
	res, err = eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnTurnComplete,
		OperationID: "op-review-complete",
	})
	if err != nil {
		t.Fatalf("Review.on_turn_complete: %v", err)
	}
	if !res.Transitioned || res.ToStepID != steps["approval"].ID {
		t.Fatalf("expected transition to Approval, got %#v", res)
	}
	store.setCurrentStep(steps["approval"].ID)

	// Approval.on_enter clears decisions and fans out runs to approvers.
	if _, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnEnter,
		OperationID: "op-approval-enter",
	}); err != nil {
		t.Fatalf("Approval.on_enter: %v", err)
	}
	if got := countCallsByReason(queue, "approval_started"); got != 1 {
		t.Errorf("approval_started fan-out calls = %d, want 1", got)
	}

	// Approver approves.
	if err := decisions.RecordStepDecision(ctx, DecisionInfo{
		TaskID: "task-1", StepID: steps["approval"].ID,
		ParticipantID: "approver-1", Decision: DecisionApproved,
	}); err != nil {
		t.Fatalf("record approver decision: %v", err)
	}

	// Approval.on_turn_complete -> move to Done.
	res, err = eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnTurnComplete,
		OperationID: "op-approval-complete",
	})
	if err != nil {
		t.Fatalf("Approval.on_turn_complete: %v", err)
	}
	if !res.Transitioned || res.ToStepID != steps["done"].ID {
		t.Fatalf("expected transition to Done, got %#v", res)
	}
}

// TestOfficeDefaultWorkflow_RejectRoutesBackToWork verifies that a single
// reviewer rejection sends the task back to Work via the second guarded
// transition (any_reject).
func TestOfficeDefaultWorkflow_RejectRoutesBackToWork(t *testing.T) {
	ctx := context.Background()
	tmpl := loadEmbeddedTemplate(t, "office-default")
	steps := compileWorkflow(tmpl)

	store := newSmokeStore(steps)
	queue := &fakeRunQueue{}
	parts := newSmokeParticipants(steps)
	decisions := newFakeDecisionStore()

	registry := MapRegistry{
		ActionQueueRunForEachParticipant: QueueRunForEachParticipantCallback{
			Adapter:      queue,
			Participants: parts,
		},
		ActionClearDecisions: ClearDecisionsCallback{Decisions: decisions},
	}
	eng := New(store, registry,
		WithRunQueue(queue),
		WithParticipantStore(parts),
		WithDecisionStore(decisions),
	)

	store.setCurrentStep(steps["review"].ID)

	// One reviewer rejects, one approves — any_reject fires first.
	if err := decisions.RecordStepDecision(ctx, DecisionInfo{
		TaskID: "task-1", StepID: steps["review"].ID,
		ParticipantID: "reviewer-1", Decision: DecisionRejected,
	}); err != nil {
		t.Fatalf("record decision: %v", err)
	}

	res, err := eng.HandleTrigger(ctx, HandleInput{
		TaskID: "task-1", SessionID: "sess-1",
		Trigger:     TriggerOnTurnComplete,
		OperationID: "op-review-reject",
	})
	if err != nil {
		t.Fatalf("Review.on_turn_complete: %v", err)
	}
	if !res.Transitioned || res.ToStepID != steps["work"].ID {
		t.Fatalf("expected reject to route to Work, got %#v", res)
	}
}

// loadEmbeddedTemplate loads a single workflow template from the embedded YAML.
func loadEmbeddedTemplate(t *testing.T, id string) *wfmodels.WorkflowTemplate {
	t.Helper()
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	for _, tmpl := range templates {
		if tmpl.ID == id {
			return tmpl
		}
	}
	t.Fatalf("template %q not found", id)
	return nil
}

// compileWorkflow compiles all steps in a template, returning a map keyed by
// the template's step id. Step IDs in move_to_step configs are kept as-is —
// the template authors them with stable ids.
func compileWorkflow(tmpl *wfmodels.WorkflowTemplate) map[string]StepSpec {
	out := make(map[string]StepSpec, len(tmpl.Steps))
	for _, def := range tmpl.Steps {
		step := &wfmodels.WorkflowStep{
			ID:        def.ID,
			Name:      def.Name,
			Position:  def.Position,
			Prompt:    def.Prompt,
			Events:    def.Events,
			StageType: def.StageType,
		}
		spec := CompileStep(step)
		out[def.ID] = spec
	}
	return out
}

// smokeStore is an in-memory TransitionStore that exposes mutable current-step
// pointers so tests can drive an end-to-end walk.
type smokeStore struct {
	steps         map[string]StepSpec
	currentStepID string
	applied       map[string]bool
	transitions   []string
}

func newSmokeStore(steps map[string]StepSpec) *smokeStore {
	return &smokeStore{
		steps:   steps,
		applied: map[string]bool{},
	}
}

func (s *smokeStore) setCurrentStep(id string) { s.currentStepID = id }

func (s *smokeStore) LoadState(_ context.Context, _, _ string) (MachineState, error) {
	return MachineState{
		TaskID:        "task-1",
		SessionID:     "sess-1",
		WorkflowID:    "wf",
		CurrentStepID: s.currentStepID,
	}, nil
}

func (s *smokeStore) LoadStep(_ context.Context, _, stepID string) (StepSpec, error) {
	step, ok := s.steps[stepID]
	if !ok {
		return StepSpec{}, fmt.Errorf("smokeStore: step %q not found", stepID)
	}
	return step, nil
}

func (s *smokeStore) LoadNextStep(_ context.Context, _ string, currentPosition int) (StepSpec, error) {
	for _, step := range s.steps {
		if step.Position == currentPosition+1 {
			return step, nil
		}
	}
	return StepSpec{}, errors.New("no next step")
}

func (s *smokeStore) LoadPreviousStep(_ context.Context, _ string, currentPosition int) (StepSpec, error) {
	for _, step := range s.steps {
		if step.Position == currentPosition-1 {
			return step, nil
		}
	}
	return StepSpec{}, errors.New("no previous step")
}

func (s *smokeStore) ApplyTransition(_ context.Context, _, _, fromStepID, toStepID string, _ Trigger) error {
	s.transitions = append(s.transitions, fromStepID+"->"+toStepID)
	s.currentStepID = toStepID
	return nil
}

func (s *smokeStore) PersistData(_ context.Context, _ string, _ map[string]any) error { return nil }

func (s *smokeStore) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}

func (s *smokeStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}

// smokeParticipants seeds two reviewers + one approver against the office-
// default's step ids ("review", "approval"). All decisions are required.
type smokeParticipants struct {
	byStep map[string][]ParticipantInfo
}

func newSmokeParticipants(steps map[string]StepSpec) *smokeParticipants {
	return &smokeParticipants{
		byStep: map[string][]ParticipantInfo{
			steps["review"].ID: {
				{ID: "reviewer-1", StepID: steps["review"].ID, Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
				{ID: "reviewer-2", StepID: steps["review"].ID, Role: "reviewer", AgentProfileID: "rev-B", DecisionRequired: true},
			},
			steps["approval"].ID: {
				{ID: "approver-1", StepID: steps["approval"].ID, Role: "approver", AgentProfileID: "app-A", DecisionRequired: true},
			},
		},
	}
}

func (p *smokeParticipants) ListStepParticipants(_ context.Context, stepID, _ string) ([]ParticipantInfo, error) {
	return p.byStep[stepID], nil
}

type stubPrimary struct{ id string }

func (s stubPrimary) PrimaryAgentProfileID(_ context.Context, _ string) (string, error) {
	return s.id, nil
}

type stubCEO struct{ id string }

func (s stubCEO) ResolveCEOAgentProfileID(_ context.Context, _ string) (string, error) {
	return s.id, nil
}

// noOpCallback is used to satisfy ActionAutoStartAgent during the smoke walk;
// the engine simply needs *some* callback registered for the kind even though
// the action carries no behaviour relevant to transition resolution.
type noOpCallback struct{}

func (noOpCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	return ActionResult{}, nil
}

func countCallsByReason(q *fakeRunQueue, reason string) int {
	count := 0
	for _, c := range q.calls {
		if c.Reason == reason {
			count++
		}
	}
	return count
}
