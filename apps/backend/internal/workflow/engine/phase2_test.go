package engine

import (
	"context"
	"testing"
)

func TestPhase2_NewTriggerConstantsAreDistinct(t *testing.T) {
	triggers := []Trigger{
		TriggerOnEnter, TriggerOnTurnStart, TriggerOnTurnComplete, TriggerOnExit,
		TriggerOnComment, TriggerOnBlockerResolved, TriggerOnChildrenCompleted,
		TriggerOnApprovalResolved, TriggerOnHeartbeat, TriggerOnBudgetAlert,
		TriggerOnAgentError, TriggerOnPRMerged, TriggerOnPRClosed,
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
	for _, v := range []string{"work", "review", "approval", "custom", "hold"} {
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
