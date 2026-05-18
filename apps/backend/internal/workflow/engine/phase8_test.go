package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeTaskCreator records every CreateChildTask invocation.
type fakeTaskCreator struct {
	calls []struct {
		ParentID string
		Spec     ChildTaskSpec
	}
	returnedID string
	err        error
}

func (f *fakeTaskCreator) CreateChildTask(_ context.Context, parentID string, spec ChildTaskSpec) (string, error) {
	f.calls = append(f.calls, struct {
		ParentID string
		Spec     ChildTaskSpec
	}{ParentID: parentID, Spec: spec})
	if f.err != nil {
		return "", f.err
	}
	if f.returnedID != "" {
		return f.returnedID, nil
	}
	return "child-1", nil
}

func newCreateChildTaskInput(spec *CreateChildTaskAction) ActionInput {
	return ActionInput{
		Trigger: TriggerOnTurnComplete,
		State:   MachineState{TaskID: "parent-1", SessionID: "sess-1"},
		Step:    StepSpec{ID: "step-work"},
		Action: Action{
			Kind:            ActionCreateChildTask,
			CreateChildTask: spec,
		},
		OperationID: "op-create-1",
	}
}

func TestCreateChildTaskCallback_HappyPath(t *testing.T) {
	creator := &fakeTaskCreator{returnedID: "child-1"}
	cb := CreateChildTaskCallback{Creator: creator}
	spec := &CreateChildTaskAction{
		Title:          "Fix bug",
		Description:    "Investigate and fix the panic in /api/foo",
		WorkflowID:     "wf-kanban",
		StepID:         "step-1",
		AgentProfileID: "profile-claude",
	}
	if _, err := cb.Execute(context.Background(), newCreateChildTaskInput(spec)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 CreateChildTask call, got %d", len(creator.calls))
	}
	got := creator.calls[0]
	if got.ParentID != "parent-1" {
		t.Errorf("parent_id = %q, want parent-1", got.ParentID)
	}
	if got.Spec.Title != "Fix bug" {
		t.Errorf("title = %q, want Fix bug", got.Spec.Title)
	}
	if got.Spec.WorkflowID != "wf-kanban" {
		t.Errorf("workflow_id = %q, want wf-kanban", got.Spec.WorkflowID)
	}
	if got.Spec.StepID != "step-1" {
		t.Errorf("step_id = %q, want step-1", got.Spec.StepID)
	}
	if got.Spec.AgentProfileID != "profile-claude" {
		t.Errorf("agent_profile_id = %q, want profile-claude", got.Spec.AgentProfileID)
	}
}

func TestCreateChildTaskCallback_RequiresCreator(t *testing.T) {
	cb := CreateChildTaskCallback{}
	_, err := cb.Execute(context.Background(), newCreateChildTaskInput(&CreateChildTaskAction{Title: "x"}))
	if err == nil {
		t.Fatalf("expected error when creator is nil")
	}
	if !errors.Is(err, ErrActionNotYetWired) {
		t.Errorf("expected ErrActionNotYetWired, got: %v", err)
	}
}

func TestCreateChildTaskCallback_RequiresTitle(t *testing.T) {
	creator := &fakeTaskCreator{}
	cb := CreateChildTaskCallback{Creator: creator}
	_, err := cb.Execute(context.Background(), newCreateChildTaskInput(&CreateChildTaskAction{}))
	if err == nil {
		t.Fatalf("expected error when title is empty")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("expected title error, got: %v", err)
	}
	if len(creator.calls) != 0 {
		t.Errorf("expected no CreateChildTask calls when title missing, got %d", len(creator.calls))
	}
}

func TestCreateChildTaskCallback_RequiresTaskID(t *testing.T) {
	creator := &fakeTaskCreator{}
	cb := CreateChildTaskCallback{Creator: creator}
	in := newCreateChildTaskInput(&CreateChildTaskAction{Title: "X"})
	in.State.TaskID = ""
	_, err := cb.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error when state.TaskID is empty")
	}
}

// fakeWorkflowSwitcher records every SwitchTaskWorkflow invocation.
type fakeWorkflowSwitcher struct {
	calls []struct {
		TaskID, WorkflowID, StepID string
	}
	resolvedStep string
	err          error
}

func (f *fakeWorkflowSwitcher) SwitchTaskWorkflow(_ context.Context, taskID, wfID, stepID string) (string, error) {
	f.calls = append(f.calls, struct {
		TaskID, WorkflowID, StepID string
	}{TaskID: taskID, WorkflowID: wfID, StepID: stepID})
	if f.err != nil {
		return "", f.err
	}
	if f.resolvedStep != "" {
		return f.resolvedStep, nil
	}
	if stepID != "" {
		return stepID, nil
	}
	return "first-step", nil
}

// dispatchRecorder captures DispatchTriggerFn invocations for assertions.
type dispatchRecorder struct {
	calls []struct {
		TaskID, SessionID, OperationID string
		Trigger                        Trigger
	}
	err error
}

func (d *dispatchRecorder) fn() DispatchTriggerFn {
	return func(_ context.Context, taskID, sessionID string, trigger Trigger, opID string) error {
		d.calls = append(d.calls, struct {
			TaskID, SessionID, OperationID string
			Trigger                        Trigger
		}{TaskID: taskID, SessionID: sessionID, OperationID: opID, Trigger: trigger})
		return d.err
	}
}

func newSwitchWorkflowInput(spec *SwitchWorkflowAction) ActionInput {
	return ActionInput{
		Trigger: TriggerOnTurnComplete,
		State:   MachineState{TaskID: "task-1", SessionID: "sess-1"},
		Step:    StepSpec{ID: "old-step"},
		Action: Action{
			Kind:           ActionSwitchWorkflow,
			SwitchWorkflow: spec,
		},
		OperationID: "op-switch-1",
	}
}

func TestSwitchWorkflowCallback_HappyPath(t *testing.T) {
	switcher := &fakeWorkflowSwitcher{resolvedStep: "new-first-step"}
	rec := &dispatchRecorder{}
	cb := SwitchWorkflowCallback{Switcher: switcher, Dispatch: rec.fn()}
	spec := &SwitchWorkflowAction{WorkflowID: "wf-office"}
	if _, err := cb.Execute(context.Background(), newSwitchWorkflowInput(spec)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(switcher.calls) != 1 {
		t.Fatalf("expected 1 SwitchTaskWorkflow call, got %d", len(switcher.calls))
	}
	if switcher.calls[0].TaskID != "task-1" || switcher.calls[0].WorkflowID != "wf-office" {
		t.Errorf("switcher call = %+v", switcher.calls[0])
	}
	// Dispatch fires on_exit (before swap) then on_enter (after swap).
	if len(rec.calls) != 2 {
		t.Fatalf("expected 2 dispatch calls (on_exit + on_enter), got %d", len(rec.calls))
	}
	if rec.calls[0].Trigger != TriggerOnExit {
		t.Errorf("first dispatch = %s, want on_exit", rec.calls[0].Trigger)
	}
	if rec.calls[1].Trigger != TriggerOnEnter {
		t.Errorf("second dispatch = %s, want on_enter", rec.calls[1].Trigger)
	}
	if rec.calls[0].TaskID != "task-1" || rec.calls[0].SessionID != "sess-1" {
		t.Errorf("dispatch call missing ids: %+v", rec.calls[0])
	}
	// Idempotency keys must be distinct so on_exit and on_enter both apply.
	if rec.calls[0].OperationID == rec.calls[1].OperationID {
		t.Errorf("on_exit and on_enter share operation id %q", rec.calls[0].OperationID)
	}
}

func TestSwitchWorkflowCallback_RequiresSwitcher(t *testing.T) {
	cb := SwitchWorkflowCallback{}
	_, err := cb.Execute(context.Background(), newSwitchWorkflowInput(&SwitchWorkflowAction{WorkflowID: "wf"}))
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got: %v", err)
	}
}

func TestSwitchWorkflowCallback_RequiresWorkflowID(t *testing.T) {
	switcher := &fakeWorkflowSwitcher{}
	cb := SwitchWorkflowCallback{Switcher: switcher}
	_, err := cb.Execute(context.Background(), newSwitchWorkflowInput(&SwitchWorkflowAction{}))
	if err == nil {
		t.Fatalf("expected error when workflow_id is empty")
	}
	if len(switcher.calls) != 0 {
		t.Errorf("expected no switcher calls when workflow_id empty")
	}
}

func TestSwitchWorkflowCallback_OmitsDispatchWhenNil(t *testing.T) {
	switcher := &fakeWorkflowSwitcher{resolvedStep: "step"}
	cb := SwitchWorkflowCallback{Switcher: switcher, Dispatch: nil}
	if _, err := cb.Execute(context.Background(), newSwitchWorkflowInput(&SwitchWorkflowAction{WorkflowID: "wf"})); err != nil {
		t.Fatalf("unexpected error with nil dispatch: %v", err)
	}
	if len(switcher.calls) != 1 {
		t.Errorf("expected switch to still happen even without dispatch, got %d calls", len(switcher.calls))
	}
}

// TestEngine_CreateChildTaskWiredViaOption asserts the Option/getter
// surface lets a caller probe whether the engine has a TaskCreator wired.
func TestEngine_CreateChildTaskWiredViaOption(t *testing.T) {
	creator := &fakeTaskCreator{}
	store := &fakeStore{state: MachineState{}, applied: map[string]bool{}}
	eng := New(store, MapRegistry{}, WithTaskCreator(creator))
	if eng.TaskCreatorAdapter() != creator {
		t.Errorf("WithTaskCreator did not wire creator")
	}
}

// TestEngine_SwitchWorkflowWiredViaOption asserts the WorkflowSwitcher
// option round-trips through the engine accessor.
func TestEngine_SwitchWorkflowWiredViaOption(t *testing.T) {
	switcher := &fakeWorkflowSwitcher{}
	store := &fakeStore{state: MachineState{}, applied: map[string]bool{}}
	eng := New(store, MapRegistry{}, WithWorkflowSwitcher(switcher))
	if eng.WorkflowSwitcherAdapter() != switcher {
		t.Errorf("WithWorkflowSwitcher did not wire switcher")
	}
}

// TestPayload_ChildSummaryCarriesPRLinks: the engine ChildSummary type
// has been extended in Phase 8 to carry PR URLs joined from
// github_task_prs. Lock the field on the type so subscriber code can
// rely on it.
func TestPayload_ChildSummaryCarriesPRLinks(t *testing.T) {
	payload := OnChildrenCompletedPayload{
		ChildSummaries: []ChildSummary{
			{
				TaskID:  "child-1",
				Status:  "completed",
				Summary: "Implemented feature",
				PRLinks: []string{"https://github.com/owner/repo/pull/42"},
			},
		},
	}
	if len(payload.ChildSummaries) != 1 {
		t.Fatalf("expected 1 child summary")
	}
	if got := payload.ChildSummaries[0].PRLinks; len(got) != 1 || got[0] == "" {
		t.Errorf("expected PRLinks populated, got %#v", got)
	}
}
