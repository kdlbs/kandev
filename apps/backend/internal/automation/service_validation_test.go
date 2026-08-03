package automation

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAddTrigger_RejectsCronTheSchedulerCannotRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "x", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	for _, expr := range []string{"60 * * * *", "*/0 * * * *", "10-5 * * * *", "not a cron"} {
		cfg := json.RawMessage(`{"cron_expression":"` + expr + `"}`)
		if _, err := svc.AddTrigger(ctx, &AddTriggerRequest{
			AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true,
		}); err == nil {
			t.Errorf("expected %q to be rejected", expr)
		}
	}
}

// The backend parser accepts named fields the editor's regex refuses, so
// validation must not be stricter than the scheduler either.
func TestAddTrigger_AcceptsWhatTheSchedulerAccepts(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "x", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	for _, expr := range []string{"0 9 * * 1-5", "@daily", "@every 2h30m", "0 9,17 * * *", ""} {
		cfg := json.RawMessage(`{"cron_expression":"` + expr + `"}`)
		if _, err := svc.AddTrigger(ctx, &AddTriggerRequest{
			AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true,
		}); err != nil {
			t.Errorf("expected %q to be accepted, got %v", expr, err)
		}
	}
}

type stubWorkflowLocator struct{ workspaceID string }

func (s stubWorkflowLocator) WorkflowWorkspaceID(context.Context, string) (string, error) {
	return s.workspaceID, nil
}

// The editor filtering foreign workflows out of a dropdown is not a boundary —
// a request naming one directly has to be refused server-side.
func TestCreateAutomation_RejectsAForeignWorkflow(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-other"})
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "cross", WorkflowID: "wf-foreign", WorkflowStepID: "s-1",
	})
	if err == nil {
		t.Fatal("expected a workflow from another workspace to be rejected")
	}
}

func TestCreateAutomation_AcceptsItsOwnWorkflow(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-1"})
	ctx := context.Background()

	if _, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "local", WorkflowID: "wf-local", WorkflowStepID: "s-1",
	}); err != nil {
		t.Fatalf("expected a same-workspace workflow to be accepted, got %v", err)
	}
}

// The update path has the same boundary as create and is the one a long-lived
// automation actually travels: it is edited far more often than it is created.
// A cron the scheduler cannot parse was accepted at creation and only rejected
// on the first edit. In between, the automation sat there looking configured
// and never fired — the worst shape for this failure, because nothing on screen
// said anything was wrong.
func TestCreateAutomation_RejectsAnUnparseableSchedule(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "bad cron",
		Triggers: []CreateTriggerSpec{{
			Type:    TriggerTypeScheduled,
			Config:  json.RawMessage(`{"cron_expression":"not-a-cron"}`),
			Enabled: true,
		}},
	})
	if err == nil {
		t.Fatal("expected an unparseable cron to be rejected at creation, as it is on edit")
	}
}

func TestCreateAutomation_AcceptsAScheduleTheSchedulerParses(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "good cron",
		Triggers: []CreateTriggerSpec{{
			Type:    TriggerTypeScheduled,
			Config:  json.RawMessage(`{"cron_expression":"0 9 * * MON-FRI"}`),
			Enabled: true,
		}},
	}); err != nil {
		t.Fatalf("expected a named-weekday cron to be accepted, got %v", err)
	}
}

func TestUpdateAutomation_RejectsAForeignWorkflow(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-1"})
	ctx := context.Background()

	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "local", WorkflowID: "wf-local", WorkflowStepID: "s-1",
	})
	if err != nil {
		t.Fatalf("seed automation: %v", err)
	}

	// Now the locator reports every workflow as belonging elsewhere, which is
	// what a request naming another workspace's workflow looks like.
	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-other"})
	foreign := "wf-foreign"
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		WorkflowID: &foreign,
	}); err == nil {
		t.Fatal("expected a workflow from another workspace to be rejected on update")
	}
}

func TestUpdateAutomation_AcceptsItsOwnWorkflow(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-1"})
	ctx := context.Background()

	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "local", WorkflowID: "wf-local", WorkflowStepID: "s-1",
	})
	if err != nil {
		t.Fatalf("seed automation: %v", err)
	}

	own := "wf-other-local"
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		WorkflowID: &own,
	}); err != nil {
		t.Fatalf("expected a same-workspace workflow to be accepted on update, got %v", err)
	}
}
