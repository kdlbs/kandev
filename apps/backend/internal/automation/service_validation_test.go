package automation

import (
	"context"
	"encoding/json"
	"errors"
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

type stubWorkflowStepLocator struct {
	belongs bool
	err     error
}

func (s stubWorkflowStepLocator) WorkflowStepBelongs(context.Context, string, string, string) (bool, error) {
	return s.belongs, s.err
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

func TestCreateAutomation_RejectsAStepFromADifferentWorkflow(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-1"})
	svc.SetWorkflowStepLocator(stubWorkflowStepLocator{belongs: false})
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "cross-step", WorkflowID: "wf-local", WorkflowStepID: "step-foreign",
	})
	if err == nil {
		t.Fatal("expected a workflow step from another workflow to be rejected")
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

func TestUpdateAutomation_RejectsAStepFromADifferentWorkflow(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "local", WorkflowID: "wf-local", WorkflowStepID: "step-local",
	})
	if err != nil {
		t.Fatalf("seed automation: %v", err)
	}

	svc.SetWorkflowLocator(stubWorkflowLocator{workspaceID: "ws-1"})
	svc.SetWorkflowStepLocator(stubWorkflowStepLocator{belongs: false})
	foreignStep := "step-foreign"
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		WorkflowStepID: &foreignStep,
	}); err == nil {
		t.Fatal("expected a workflow step from another workflow to be rejected on update")
	}
}

// fakeAgentProfileLookup implements AgentProfileLookup: live names the profile
// IDs that resolve, err short-circuits every call to simulate a driver
// failure, and asked records what was actually looked up so the empty-ID
// short-circuit can be asserted rather than inferred from a nil error.
type fakeAgentProfileLookup struct {
	live  map[string]bool
	err   error
	asked []string
}

func (f *fakeAgentProfileLookup) AgentProfileExists(_ context.Context, profileID string) (bool, error) {
	f.asked = append(f.asked, profileID)
	if f.err != nil {
		return false, f.err
	}
	return f.live[profileID], nil
}

// Disabling an automation before its profile row is deleted only ever covered
// bindings that were valid to start with. A create naming a profile ID that
// never existed reaches the same end state — an enabled automation bound to a
// profile that is not there — with no deletion involved at all.
func TestCreateAutomation_RejectsAProfileThatDoesNotExist(t *testing.T) {
	svc := newTestService(t)
	svc.SetAgentProfileLookup(&fakeAgentProfileLookup{live: map[string]bool{"agent-live": true}})
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "orphan", AgentProfileID: "agent-ghost",
	})
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("expected ErrAgentProfileNotFound, got %v", err)
	}
}

func TestCreateAutomation_AcceptsAProfileThatExists(t *testing.T) {
	svc := newTestService(t)
	svc.SetAgentProfileLookup(&fakeAgentProfileLookup{live: map[string]bool{"agent-live": true}})
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "fine", AgentProfileID: "agent-live",
	})
	if err != nil {
		t.Fatalf("expected a live profile to be accepted, got %v", err)
	}
	if a.AgentProfileID != "agent-live" {
		t.Fatalf("expected the profile binding to be stored, got %q", a.AgentProfileID)
	}
}

// Rebinding is the path a long-lived automation actually travels — it is
// edited far more often than it is created — and the profile a stale editor
// tab still offers may have been deleted since that tab was opened.
func TestUpdateAutomation_RejectsAProfileThatDoesNotExist(t *testing.T) {
	svc := newTestService(t)
	svc.SetAgentProfileLookup(&fakeAgentProfileLookup{live: map[string]bool{"agent-live": true}})
	ctx := context.Background()

	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "seed", AgentProfileID: "agent-live",
	})
	if err != nil {
		t.Fatalf("seed automation: %v", err)
	}

	ghost := "agent-ghost"
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		AgentProfileID: &ghost,
	}); !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("expected ErrAgentProfileNotFound on update, got %v", err)
	}

	// The rejected rebind must not have landed: a create-side guard that lets
	// the update through would leave exactly the orphan it was added to stop.
	got, err := svc.GetAutomation(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload automation: %v", err)
	}
	if got.AgentProfileID != "agent-live" {
		t.Fatalf("expected the original binding to survive a rejected rebind, got %q", got.AgentProfileID)
	}
}

func TestUpdateAutomation_AcceptsAProfileThatExists(t *testing.T) {
	svc := newTestService(t)
	svc.SetAgentProfileLookup(&fakeAgentProfileLookup{live: map[string]bool{
		"agent-live": true, "agent-other": true,
	}})
	ctx := context.Background()

	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "seed", AgentProfileID: "agent-live",
	})
	if err != nil {
		t.Fatalf("seed automation: %v", err)
	}

	other := "agent-other"
	updated, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		AgentProfileID: &other,
	})
	if err != nil {
		t.Fatalf("expected a live profile to be accepted on update, got %v", err)
	}
	if updated.AgentProfileID != "agent-other" {
		t.Fatalf("expected the rebind to land, got %q", updated.AgentProfileID)
	}
}

// An unset agent_profile_id is a separate defect from a dangling one, and this
// is not the change that fixes it. Nothing on the firing path substitutes a
// workspace default — the launch dies at executor.PrepareSession with
// ErrNoAgentProfileID — but the editor's save button still permits it, so
// rejecting it here would refuse data the UI currently produces. It
// short-circuits before the lookup rather than being answered by it, so the
// choice cannot be quietly reversed by a lookup that happens to say "yes" to
// the empty string.
func TestAgentProfileValidation_LeavesAnUnsetProfileAlone(t *testing.T) {
	lookup := &fakeAgentProfileLookup{live: map[string]bool{"agent-live": true}}
	svc := newTestService(t)
	svc.SetAgentProfileLookup(lookup)
	ctx := context.Background()

	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "no profile",
	})
	if err != nil {
		t.Fatalf("expected an unset agent_profile_id to be accepted on create, got %v", err)
	}

	cleared := ""
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		AgentProfileID: &cleared,
	}); err != nil {
		t.Fatalf("expected clearing agent_profile_id to be accepted on update, got %v", err)
	}

	if len(lookup.asked) != 0 {
		t.Fatalf("expected the empty profile ID never to reach the lookup, it asked for %v", lookup.asked)
	}
}

// Without a lookup wired the check is skipped, not failed closed — unlike
// RepositoryLookup, which guards cross-workspace access and so must refuse
// when unconfigured. This one applies to nearly every automation, so failing
// closed would turn a single missing wire into "nothing can be saved".
func TestAgentProfileValidation_SkippedWithoutALookup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "unwired", AgentProfileID: "agent-ghost",
	}); err != nil {
		t.Fatalf("expected creation to succeed with no lookup wired, got %v", err)
	}
}

// A driver failure is not a missing profile. Collapsing the two would let one
// flaky read reject a binding that is perfectly good — and report to the user
// that their profile does not exist, which would send them looking in the
// wrong place entirely.
func TestAgentProfileValidation_SurfacesALookupFailureAsItself(t *testing.T) {
	boom := errors.New("database is locked")
	svc := newTestService(t)
	svc.SetAgentProfileLookup(&fakeAgentProfileLookup{err: boom})
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "flaky", AgentProfileID: "agent-live",
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected the lookup failure to be surfaced, got %v", err)
	}
	if errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatal("a lookup failure must not be reported as a missing profile")
	}
}
