package engine

import (
	"context"
	"reflect"
	"testing"
)

// payloadCapturingCallback records the ActionInput it was invoked with so
// tests can pin trigger -> payload plumbing.
type payloadCapturingCallback struct {
	captured ActionInput
	called   bool
}

func (c *payloadCapturingCallback) Execute(_ context.Context, in ActionInput) (ActionResult, error) {
	c.called = true
	c.captured = in
	return ActionResult{}, nil
}

// triggerStore is a minimal TransitionStore that lets tests register a step
// with arbitrary trigger->actions mappings.
type triggerStore struct {
	state   MachineState
	step    StepSpec
	applied map[string]bool
}

func (s *triggerStore) LoadState(_ context.Context, _, _ string) (MachineState, error) {
	return s.state, nil
}
func (s *triggerStore) LoadStep(_ context.Context, _, _ string) (StepSpec, error) {
	return s.step, nil
}
func (s *triggerStore) LoadNextStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (s *triggerStore) LoadPreviousStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (s *triggerStore) ApplyTransition(_ context.Context, _, _, _, _ string, _ Trigger) error {
	return nil
}
func (s *triggerStore) PersistData(_ context.Context, _ string, _ map[string]any) error { return nil }
func (s *triggerStore) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}
func (s *triggerStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}

func newTriggerStoreWithActions(trigger Trigger, actions []Action) *triggerStore {
	return &triggerStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf", CurrentStepID: "step-1"},
		step: StepSpec{
			ID: "step-1", WorkflowID: "wf", Position: 0,
			Events: map[Trigger][]Action{trigger: actions},
		},
		applied: map[string]bool{},
	}
}

func TestHandleTrigger_NewTriggers_DispatchToCallbacks(t *testing.T) {
	cases := []struct {
		name    string
		trigger Trigger
		payload any
	}{
		{"on_comment", TriggerOnComment, OnCommentPayload{CommentID: "c-1", Body: "hi"}},
		{"on_blocker_resolved", TriggerOnBlockerResolved, OnBlockerResolvedPayload{ResolvedBlockerIDs: []string{"b1"}}},
		{"on_children_completed", TriggerOnChildrenCompleted, OnChildrenCompletedPayload{
			ChildSummaries: []ChildSummary{{TaskID: "ct-1", Status: "done"}},
		}},
		{"on_approval_resolved", TriggerOnApprovalResolved, OnApprovalResolvedPayload{ApprovalID: "a-1", Status: "approved"}},
		{"on_heartbeat", TriggerOnHeartbeat, OnHeartbeatPayload{}},
		{"on_budget_alert", TriggerOnBudgetAlert, OnBudgetAlertPayload{BudgetPct: 90, Scope: "task"}},
		{"on_agent_error", TriggerOnAgentError, OnAgentErrorPayload{FailedAgentID: "a", FailedSessionID: "s", ErrorMessage: "boom"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cb := &payloadCapturingCallback{}
			store := newTriggerStoreWithActions(tc.trigger, []Action{
				{Kind: ActionSetWorkflowData, SetWorkflowData: &SetWorkflowDataAction{Key: "k", Value: "v"}},
			})
			eng := New(store, MapRegistry{ActionSetWorkflowData: cb})
			_, err := eng.HandleTrigger(context.Background(), HandleInput{
				TaskID: "t1", SessionID: "s1", Trigger: tc.trigger, Payload: tc.payload,
			})
			if err != nil {
				t.Fatalf("HandleTrigger(%s) error: %v", tc.trigger, err)
			}
			if !cb.called {
				t.Fatalf("expected callback to be invoked for trigger %s", tc.trigger)
			}
			if cb.captured.Trigger != tc.trigger {
				t.Fatalf("captured trigger = %s, want %s", cb.captured.Trigger, tc.trigger)
			}
			if !reflect.DeepEqual(cb.captured.Payload, tc.payload) {
				t.Fatalf("captured payload mismatch for %s: got %#v, want %#v", tc.trigger, cb.captured.Payload, tc.payload)
			}
		})
	}
}

func TestHandleTrigger_PayloadIsNilForKanbanTriggers(t *testing.T) {
	cb := &payloadCapturingCallback{}
	store := newTriggerStoreWithActions(TriggerOnEnter, []Action{
		{Kind: ActionSetWorkflowData, SetWorkflowData: &SetWorkflowDataAction{Key: "k", Value: "v"}},
	})
	eng := New(store, MapRegistry{ActionSetWorkflowData: cb})
	_, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter, // no Payload
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.captured.Payload != nil {
		t.Fatalf("expected nil payload for kanban trigger, got %#v", cb.captured.Payload)
	}
}
