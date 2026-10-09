package coordinator

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestStartupRecoveryPass_RecoversStaleRowAndCreatesTask(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-10*time.Minute))
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	svc.StartupRecoveryPass(context.Background(), time.Now())

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if got.TaskID == nil || *got.TaskID != "task-new" {
		t.Fatalf("TaskID = %v, want task-new", got.TaskID)
	}
}

func TestStartupRecoveryPass_LeavesRowsClaimedAtOrAfterCutoffAlone(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	cutoff := time.Now()
	claimDirectly(t, store, p, "recent-tok", sampleSpec(), cutoff.Add(time.Minute))
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	svc.StartupRecoveryPass(context.Background(), cutoff)

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving (untouched)", got.Status)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

func TestStartupRecoveryPass_RecoversMultipleRowsInOrder(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p1 := insertProposal(t, store, c, sampleSpec())
	p2 := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p1, "old-tok-1", sampleSpec(), time.Now().Add(-10*time.Minute))
	claimDirectly(t, store, p2, "old-tok-2", sampleSpec(), time.Now().Add(-5*time.Minute))
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	svc.StartupRecoveryPass(context.Background(), time.Now())

	for _, id := range []string{p1.ID, p2.ID} {
		got, err := store.GetProposal(context.Background(), "ws-1", c.ID, id, false)
		if err != nil {
			t.Fatalf("GetProposal(%s): %v", id, err)
		}
		if got.Status != ProposalStatusApproved {
			t.Fatalf("proposal %s Status = %q, want approved", id, got.Status)
		}
	}
	if len(tasks.createCalls) != 2 {
		t.Fatalf("createCalls = %d, want 2", len(tasks.createCalls))
	}
}

func TestStartupRecoveryPass_StepIneligibleFailsRowAndContinues(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-10*time.Minute))
	svc.SetDecisionDeps(tasks, &fakeStepReader{steps: []*workflowmodels.WorkflowStep{
		{ID: "step-1", IsStartStep: true, Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
		}},
	}}, nil)

	svc.StartupRecoveryPass(context.Background(), time.Now())

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

func TestStartupRecoveryPass_FoundTaskCompletesWithoutCreate(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-10*time.Minute))
	tasks.lookupByExternalID = map[string]*taskmodels.Task{
		proposalExternalID(p.ID): {ID: "task-existing", WorkspaceID: "ws-1"},
	}

	svc.StartupRecoveryPass(context.Background(), time.Now())

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-existing" {
		t.Fatalf("got = %+v, want approved/task-existing", got)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

func TestStartupRecoveryPass_DiscoveryErrorReturnsWithoutPanicking(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-10*time.Minute))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.StartupRecoveryPass(ctx, time.Now())

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving (untouched by a failed discovery query)", got.Status)
	}
}
