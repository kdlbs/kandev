package coordinator

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func startingGraph() []*wfmodels.WorkflowStep {
	auto := wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}
	return []*wfmodels.WorkflowStep{
		{ID: "start", IsStartStep: true},
		{ID: "runner", AllowManualMove: true, Events: auto},
		{ID: "done", AllowManualMove: true, CompleteTaskOnEnter: true, Events: auto},
		{ID: "locked"},
	}
}

func startFixture(t *testing.T, startAgent string) proposalTestFixture {
	t.Helper()
	f := newProposalTestFixture(t)
	f.svc.phase2 = true
	f.svc.proposalSteps = fakeWorkflowStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{f.workflowID: startingGraph()}}
	mustSave(t, f.svc, f.workspaceID, f.coordinator.ID, policyBody(map[string]string{"start_agent": startAgent}))
	return f
}

func TestProposeTask_StartAgentDeniedKeepsPhase1Rule(t *testing.T) {
	f := startFixture(t, "denied")
	req := f.baseRequest()
	req.StepID = "runner"
	_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
	assertFieldError(t, err, "step_id")
}

func TestProposeTask_StartAgentRequiresApprovalRelaxesOnlyTheAgentClauses(t *testing.T) {
	f := startFixture(t, "requires_approval")
	req := f.baseRequest()
	req.StepID = "runner"
	p, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
	if err != nil || !p.StartsAgent {
		t.Fatalf("p=%+v err=%v, want stored with starts_agent", p, err)
	}
	req.StepID = "start"
	if p, _, err = f.svc.ProposeTask(context.Background(), f.coordinator.ID, req); err != nil || p.StartsAgent {
		t.Fatalf("p=%+v err=%v, want starts_agent false for a quiet step", p, err)
	}
	for _, step := range []string{"done", "locked", "missing"} {
		req.StepID = step
		_, _, err = f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "step_id")
	}
}

func TestProposeTask_StandingOrderCitations(t *testing.T) {
	f := startFixture(t, "denied")
	ctx := context.Background()
	order, err := f.svc.AddStandingOrder(ctx, f.workspaceID, f.coordinator.ID, AddStandingOrderInput{Text: "Prefer small tasks"})
	if err != nil {
		t.Fatal(err)
	}
	req := f.baseRequest()
	req.StandingOrderIDs = []string{"a", "b", "c", "d", "e", "f"}
	_, _, err = f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	assertFieldError(t, err, "standing_order_ids")
	req.StandingOrderIDs = []string{order.ID, order.ID}
	_, _, err = f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	assertFieldError(t, err, "standing_order_ids")

	req.StandingOrderIDs = []string{order.ID}
	p, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	if err != nil || len(p.StandingOrderIDs) != 1 {
		t.Fatalf("p=%+v err=%v, want the citation stored", p, err)
	}
	orders, _ := f.svc.ListStandingOrders(ctx, f.workspaceID, f.coordinator.ID, false)
	if len(orders) != 1 || orders[0].LastAppliedAt == nil {
		t.Fatalf("orders = %+v, want last_applied_at set by the propose", orders)
	}

	if _, err = f.svc.RetireStandingOrder(ctx, f.workspaceID, f.coordinator.ID, order.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err = f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	assertFieldError(t, err, "standing_order_ids")
}

func startApproveFixture(t *testing.T) (*Store, *Coordinator, *fakeDecisionTaskService, *Service, *Proposal) {
	t.Helper()
	store, c, tasks, svc := approveFixture(t)
	svc.SetDecisionDeps(tasks, &fakeStepReader{steps: startingGraph()}, nil)
	spec := sampleSpec()
	spec.StepID = "runner"
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: spec, StartsAgent: true}
	if err := store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	return store, c, tasks, svc, p
}

func TestApproveProposal_StartsAgentCreateCarriesAutoStartMarker(t *testing.T) {
	_, c, tasks, svc, p := startApproveFixture(t)
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(tasks.createCalls) != 1 || !taskmodels.HasAutoStartOnCreateIntent(tasks.createCalls[0].Metadata) {
		t.Fatalf("createCalls = %+v, want one carrying auto_start_on_create", tasks.createCalls)
	}
}

func TestApproveProposal_PlainCreateCarriesNoAutoStartMarker(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	if tasks.createCalls[0].Metadata != nil {
		t.Fatalf("Metadata = %v, want none on a quiet create", tasks.createCalls[0].Metadata)
	}
}

func TestApproveProposal_StartsAgentRefusedWhileStartAgentDenied(t *testing.T) {
	_, c, tasks, svc, p := startApproveFixture(t)
	svc.phase2 = true
	mustSave(t, svc, "ws-1", c.ID, policyBody(map[string]string{"start_agent": "denied"}))
	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	var denied *PolicyDeniedError
	if !errors.As(err, &denied) || denied.Action != ActionStartAgent {
		t.Fatalf("err = %v, want policy_denied start_agent", err)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatal("no create may run after a policy refusal")
	}
}

func TestProposeTask_FlagOffIgnoresStandingOrderIDs(t *testing.T) {
	f := startFixture(t, "denied")
	f.svc.phase2 = false
	ctx := context.Background()
	order, err := f.svc.AddStandingOrder(ctx, f.workspaceID, f.coordinator.ID, AddStandingOrderInput{Text: "Prefer small tasks"})
	if err != nil {
		t.Fatal(err)
	}
	req := f.baseRequest()
	req.StandingOrderIDs = []string{"a", "a", "missing"}
	if _, _, err = f.svc.ProposeTask(ctx, f.coordinator.ID, req); err != nil {
		t.Fatalf("flag off must ignore standing_order_ids, got %v", err)
	}
	req.StandingOrderIDs = []string{order.ID}
	p, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	if err != nil || len(p.StandingOrderIDs) != 0 {
		t.Fatalf("p=%+v err=%v, want no stored citation", p, err)
	}
	orders, _ := f.svc.ListStandingOrders(ctx, f.workspaceID, f.coordinator.ID, false)
	if len(orders) != 1 || orders[0].LastAppliedAt != nil {
		t.Fatalf("orders = %+v, want last_applied_at untouched with the flag off", orders)
	}
}
