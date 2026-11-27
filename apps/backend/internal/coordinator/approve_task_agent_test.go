package coordinator

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type taskAgentFixture struct {
	store *Store
	c     *Coordinator
	tasks *fakeDecisionTaskService
	svc   *Service
}

// newTaskAgentFixture is approveFixture with profile readers and a task pair
// on the coordinator, and a step that names no agent of its own.
func newTaskAgentFixture(t *testing.T, agents map[string]*settingsmodels.AgentProfile, executors map[string]*taskmodels.ExecutorProfile, pairAgent, pairExecutor string) *taskAgentFixture {
	t.Helper()
	store, c, tasks, svc := approveFixture(t)
	svc.validator = newValidatorForTest(agents, executors)
	tasks.stepByID = map[string]*wfmodels.WorkflowStep{"step-1": {ID: "step-1", WorkflowID: "wf-1"}}
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	if _, err := store.db.ExecContext(context.Background(), store.db.Rebind(`UPDATE coordinators SET task_agent_profile_id = ?, task_executor_profile_id = ? WHERE id = ?`), pairAgent, pairExecutor, c.ID); err != nil {
		t.Fatal(err)
	}
	return &taskAgentFixture{store, c, tasks, svc}
}

func (f *taskAgentFixture) approve(t *testing.T) (*Proposal, error) {
	t.Helper()
	p := insertProposal(t, f.store, f.c, sampleSpec())
	return f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
}

func okProfiles() (map[string]*settingsmodels.AgentProfile, map[string]*taskmodels.ExecutorProfile) {
	return map[string]*settingsmodels.AgentProfile{
		"pair-agent": {ID: "pair-agent", Name: "Pair Agent", WorkspaceID: "ws-1"},
		"ws-agent":   {ID: "ws-agent", Name: "WS Agent", WorkspaceID: "ws-1"},
		"pass":       {ID: "pass", WorkspaceID: "ws-1", CLIPassthrough: true},
	}, map[string]*taskmodels.ExecutorProfile{"pair-exec": {ID: "pair-exec"}}
}

func TestApprove_StampsCoordinatorPairWhenNothingNamesAnAgent(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	got, err := f.approve(t)
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("approve: %v %+v", err, got)
	}
	md := f.tasks.createCalls[0].Metadata
	if md[taskmodels.MetaKeyAgentProfileID] != "pair-agent" || md[taskmodels.MetaKeyExecutorProfileID] != "pair-exec" {
		t.Fatalf("metadata = %v", md)
	}
}

func TestApprove_StepOrWorkflowAgentAddsNothing(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	f.tasks.stepByID["step-1"].AgentProfileID = "step-agent"
	if _, err := f.approve(t); err != nil {
		t.Fatal(err)
	}
	if md := f.tasks.createCalls[0].Metadata; len(md) != 0 {
		t.Fatalf("metadata = %v, want none", md)
	}

	f = newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	f.tasks.workflows["wf-1"].AgentProfileID = "wf-agent"
	if _, err := f.approve(t); err != nil {
		t.Fatal(err)
	}
	if md := f.tasks.createCalls[0].Metadata; len(md) != 0 {
		t.Fatalf("workflow default: metadata = %v, want none", md)
	}
}

func TestApprove_UsableWorkspaceDefaultAddsNothingWhenNoAgentStarts(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	ws := "ws-agent"
	f.tasks.workspaces = map[string]*taskmodels.Workspace{"ws-1": {ID: "ws-1", DefaultAgentProfileID: &ws}}
	if _, err := f.approve(t); err != nil {
		t.Fatal(err)
	}
	if md := f.tasks.createCalls[0].Metadata; len(md) != 0 {
		t.Fatalf("metadata = %v, want none", md)
	}
}

func TestApprove_PassthroughWorkspaceDefaultFallsThroughToPair(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	ws := "pass"
	f.tasks.workspaces = map[string]*taskmodels.Workspace{"ws-1": {ID: "ws-1", DefaultAgentProfileID: &ws}}
	if _, err := f.approve(t); err != nil {
		t.Fatal(err)
	}
	if md := f.tasks.createCalls[0].Metadata; md[taskmodels.MetaKeyAgentProfileID] != "pair-agent" {
		t.Fatalf("metadata = %v", md)
	}
}

func TestTaskAgent_WorkspaceAdditionOnlyWhenAgentStarts(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	ws := "ws-agent"
	f.tasks.workspaces = map[string]*taskmodels.Workspace{"ws-1": {ID: "ws-1", DefaultAgentProfileID: &ws}}
	out, err := f.svc.newTaskAgentReads().taskAgent(context.Background(), "ws-1", f.c.ID, sampleSpec(), true)
	if err != nil || out.Source != RunsWithSourceWorkspace || len(out.Meta) != 1 || out.Meta[taskmodels.MetaKeyAgentProfileID] != "ws-agent" {
		t.Fatalf("out = %+v err=%v", out, err)
	}
}

func TestApprove_RefusesUnusablePair(t *testing.T) {
	a, e := okProfiles()
	cases := []struct {
		name, agent, exec, want string
	}{
		{"agent missing", "gone", "pair-exec", refusalAgentMissing},
		{"agent passthrough", "pass", "pair-exec", refusalAgentPassthrough},
		{"executor missing", "pair-agent", "gone", refusalExecutorMissing},
		{"agent wins", "gone", "gone", refusalAgentMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTaskAgentFixture(t, a, e, tc.agent, tc.exec)
			got, err := f.approve(t)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != ProposalStatusFailed || got.Error == nil || *got.Error != tc.want {
				t.Fatalf("got %s %v, want failed %q", got.Status, got.Error, tc.want)
			}
			if len(f.tasks.createCalls) != 0 {
				t.Fatalf("create ran %d times", len(f.tasks.createCalls))
			}
		})
	}
}

func TestApprove_ChainReadErrorLeavesRowApproving(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	f.tasks.stepErr = map[string]error{"step-1": errors.New("boom")}
	p := insertProposal(t, f.store, f.c, sampleSpec())
	_, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
	if err == nil {
		t.Fatal("want error")
	}
	if len(f.tasks.createCalls) != 0 {
		t.Fatal("create must not run")
	}
	row, gerr := f.store.GetProposal(context.Background(), "ws-1", f.c.ID, p.ID, false)
	if gerr != nil || row.Status != ProposalStatusApproving {
		t.Fatalf("row = %+v %v", row, gerr)
	}
}

func TestApprove_StepGoneFailsWithIneligibleText(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	f.tasks.stepErr = map[string]error{"step-1": wfmodels.ErrWorkflowStepNotFound}
	got, err := f.approve(t)
	if err != nil || got.Status != ProposalStatusFailed || *got.Error != "the target step is no longer eligible" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestRunsWith_ReadAndList(t *testing.T) {
	a, e := okProfiles()
	f := newTaskAgentFixture(t, a, e, "pair-agent", "pair-exec")
	p := insertProposal(t, f.store, f.c, sampleSpec())
	got, err := f.svc.GetProposal(context.Background(), "ws-1", f.c.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := RunsWith{AgentProfileID: "pair-agent", AgentProfileName: "Pair Agent", Source: RunsWithSourceCoordinator}
	if got.RunsWith == nil || *got.RunsWith != want {
		t.Fatalf("runs_with = %+v, want %+v", got.RunsWith, want)
	}

	f.tasks.stepByID["step-1"].AgentProfileID = "unnamed-profile"
	rows, err := f.svc.ListProposals(context.Background(), "ws-1", f.c.ID, ListProposalsPending)
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	if r := rows[0].RunsWith; r == nil || r.Source != RunsWithSourceStep || r.AgentProfileName != "unnamed-profile" {
		t.Fatalf("runs_with = %+v, want step with id as name", r)
	}

	f.tasks.stepByID["step-1"].AgentProfileID = ""
	f.tasks.stepErr = map[string]error{"step-1": errors.New("read failed")}
	got, _ = f.svc.GetProposal(context.Background(), "ws-1", f.c.ID, p.ID)
	if got.RunsWith != nil {
		t.Fatalf("failed read: runs_with = %+v, want nil", got.RunsWith)
	}
	delete(f.tasks.stepErr, "step-1")
	_, _ = f.store.db.ExecContext(context.Background(), f.store.db.Rebind(`UPDATE coordinators SET task_agent_profile_id = 'gone' WHERE id = ?`), f.c.ID)
	got, _ = f.svc.GetProposal(context.Background(), "ws-1", f.c.ID, p.ID)
	if got.RunsWith == nil || got.RunsWith.Source != RunsWithSourceNone || got.RunsWith.AgentProfileID != "" {
		t.Fatalf("unusable pair: runs_with = %+v, want none", got.RunsWith)
	}
}
