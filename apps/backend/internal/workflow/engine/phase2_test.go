package engine

import (
	"context"
	"errors"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestPhase2_NewTriggerConstantsAreDistinct(t *testing.T) {
	triggers := []Trigger{
		TriggerOnEnter, TriggerOnTurnStart, TriggerOnTurnComplete, TriggerOnExit,
		TriggerOnComment, TriggerOnBlockerResolved, TriggerOnChildrenCompleted,
		TriggerOnApprovalResolved, TriggerOnHeartbeat, TriggerOnBudgetAlert,
		TriggerOnAgentError,
	}
	seen := make(map[Trigger]bool, len(triggers))
	for _, tr := range triggers {
		if tr == "" {
			t.Fatalf("trigger constant must not be empty string")
		}
		if seen[tr] {
			t.Fatalf("duplicate trigger constant %q", tr)
		}
		seen[tr] = true
	}
}

func TestPhase2_NewActionKindsAreDistinct(t *testing.T) {
	kinds := []ActionKind{
		ActionMoveToNext, ActionMoveToPrevious, ActionMoveToStep,
		ActionEnablePlanMode, ActionDisablePlanMode, ActionAutoStartAgent,
		ActionResetAgentContext, ActionSetWorkflowData,
		ActionQueueRun, ActionClearDecisions, ActionQueueRunForEachParticipant,
	}
	seen := make(map[ActionKind]bool, len(kinds))
	for _, k := range kinds {
		if k == "" {
			t.Fatalf("action kind constant must not be empty string")
		}
		if seen[k] {
			t.Fatalf("duplicate action kind %q", k)
		}
		seen[k] = true
	}
}

func TestQueueRunAction_RoundTripsThroughActionStruct(t *testing.T) {
	a := Action{
		Kind: ActionQueueRun,
		QueueRun: &QueueRunAction{
			Target:  "primary",
			TaskID:  "this",
			Reason:  "task_assigned",
			Payload: map[string]any{"comment_id": "c-1"},
		},
	}
	if a.QueueRun == nil || a.QueueRun.Target != "primary" {
		t.Fatalf("QueueRun struct did not round-trip")
	}
	if a.QueueRun.Payload["comment_id"] != "c-1" {
		t.Fatalf("QueueRun payload did not round-trip")
	}
}

func TestQueueRunForEachParticipantAction_Construction(t *testing.T) {
	a := Action{
		Kind: ActionQueueRunForEachParticipant,
		QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
			Role:   string(wfmodels.ParticipantRoleReviewer),
			Reason: "review_started",
		},
	}
	if a.QueueRunForEachParticipant == nil {
		t.Fatalf("expected QueueRunForEachParticipant to be set")
	}
	if a.QueueRunForEachParticipant.Role != "reviewer" {
		t.Fatalf("unexpected role: %q", a.QueueRunForEachParticipant.Role)
	}
}

func TestClearDecisionsAction_Construction(t *testing.T) {
	a := Action{
		Kind:           ActionClearDecisions,
		ClearDecisions: &ClearDecisionsAction{},
	}
	if a.ClearDecisions == nil {
		t.Fatalf("expected ClearDecisions to be set")
	}
}

func TestPhase2Placeholders_ReturnNotYetWired(t *testing.T) {
	cases := []struct {
		name string
		cb   ActionCallback
	}{
		{"queue_run", PlaceholderQueueRunCallback{}},
		{"clear_decisions", PlaceholderClearDecisionsCallback{}},
		{"queue_run_for_each_participant", PlaceholderQueueRunForEachParticipantCallback{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.cb.Execute(context.Background(), ActionInput{})
			if !errors.Is(err, ErrActionNotYetWired) {
				t.Fatalf("expected ErrActionNotYetWired, got %v", err)
			}
		})
	}
}

func TestPhase2Placeholders_NotRegisteredInDefaultRegistry(t *testing.T) {
	// Empty MapRegistry must not resolve the new action kinds. This pins the
	// invariant that the Phase 2 actions are declared but inert until the
	// engine integration slice wires them up.
	r := MapRegistry{}
	for _, kind := range []ActionKind{
		ActionQueueRun, ActionClearDecisions, ActionQueueRunForEachParticipant,
	} {
		if _, ok := r.Get(kind); ok {
			t.Fatalf("action %q must not be registered by default", kind)
		}
	}
}

func TestValidParticipantRole(t *testing.T) {
	valid := []string{"reviewer", "approver", "watcher", "collaborator"}
	for _, v := range valid {
		if !ValidParticipantRole(v) {
			t.Fatalf("expected %q to be a valid role", v)
		}
	}
	for _, v := range []string{"", "ceo", "owner"} {
		if ValidParticipantRole(v) {
			t.Fatalf("expected %q to be rejected", v)
		}
	}
}

func TestValidStageType(t *testing.T) {
	for _, v := range []string{"work", "review", "approval", "custom"} {
		if !ValidStageType(v) {
			t.Fatalf("expected stage_type %q to be valid", v)
		}
	}
	for _, v := range []string{"", "ship", "release"} {
		if ValidStageType(v) {
			t.Fatalf("expected stage_type %q to be rejected", v)
		}
	}
}

func TestValidWorkflowStyle(t *testing.T) {
	for _, v := range []string{"kanban", "office", "custom"} {
		if !ValidWorkflowStyle(v) {
			t.Fatalf("expected workflow style %q to be valid", v)
		}
	}
	for _, v := range []string{"", "agile", "scrum"} {
		if ValidWorkflowStyle(v) {
			t.Fatalf("expected workflow style %q to be rejected", v)
		}
	}
}

func TestExistingEngineHandleTrigger_UnaffectedByNewKinds(t *testing.T) {
	// Pin the invariant that the existing dispatch path does not "see" the
	// new kinds — a step compiled today cannot accidentally surface them.
	store := newFakeStoreForTest()
	eng := New(store, MapRegistry{})
	res, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID:    "t1",
		SessionID: "s1",
		Trigger:   TriggerOnEnter,
	})
	if err != nil {
		t.Fatalf("HandleTrigger returned unexpected error: %v", err)
	}
	if res.Transitioned {
		t.Fatalf("expected no transition for trigger with no actions")
	}
}

// newFakeStoreForTest provides a minimal TransitionStore for trigger smoke tests.
func newFakeStoreForTest() *fakeStore {
	return &fakeStore{
		state: MachineState{
			TaskID:        "t1",
			SessionID:     "s1",
			WorkflowID:    "wf",
			CurrentStepID: "s1",
		},
		stepsByID: map[string]StepSpec{
			"s1": {ID: "s1", WorkflowID: "wf", Position: 0, Events: map[Trigger][]Action{}},
		},
		applied: map[string]bool{},
	}
}
