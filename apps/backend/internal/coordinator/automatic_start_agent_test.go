package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func (f *resumeFixture) proposeStarting(t *testing.T) *Proposal {
	t.Helper()
	f.svc.SetDecisionDeps(f.tasks, &fakeStepReader{steps: startingGraph()}, nil)
	spec := sampleSpec()
	spec.StepID = "runner"
	p := &Proposal{CoordinatorID: f.c.ID, WorkspaceID: f.c.WorkspaceID, Spec: spec, StartsAgent: true}
	if err := f.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutomaticStartAgent_NeedsBothClasses(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionCreateTask, testRaiser)
	p := f.proposeStarting(t)
	if res := f.try(t, p); res != nil {
		t.Fatalf("res = %+v, want phase 1 result while start_agent is not automatic", res)
	}
	if len(f.tasks.createCalls) != 0 || reload(t, f.store, f.c, p.ID).Status != ProposalStatusPending {
		t.Fatal("an agent-starting proposal was handled with only create_task raised")
	}
	f.raise(t, ActionStartAgent, testRaiser)
	p = f.proposeStarting(t)
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusApproved || res.TaskID != "task-new" {
		t.Fatalf("res = %+v", res)
	}
	if len(f.tasks.createCalls) != 1 || f.tasks.createCalls[0].Metadata[models.MetaKeyAutoStartOnCreate] != true {
		t.Fatalf("create calls = %+v, want the auto-start marker", f.tasks.createCalls)
	}
}

func TestAutomaticStartAgent_HeldByAdmission(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionCreateTask, testRaiser)
	f.raise(t, ActionStartAgent, testRaiser)
	f.fc.mode = "disabled"
	p := f.proposeStarting(t)
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusPending || res.Note != "automatic action held (containment); a manager will decide" {
		t.Fatalf("res = %+v", res)
	}
	if len(f.tasks.createCalls) != 0 {
		t.Fatal("a held proposal created a task")
	}
}

func TestAutomaticStartAgent_PlainCreateIsNotHeldByAdmission(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionCreateTask, testRaiser)
	f.fc.mode = "disabled"
	p := insertKind(t, f.store, f.c, ProposalKindCreateTask)
	if res := f.try(t, p); res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("res = %+v, want a plain create approved without admission", res)
	}
}

func TestAutomaticStartAgent_FailureLowersStartAgentOnly(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionCreateTask, testRaiser)
	f.raise(t, ActionStartAgent, testRaiser)
	f.tasks.createErr = errors.New("create failed")
	p := f.proposeStarting(t)
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusFailed {
		t.Fatalf("res = %+v", res)
	}
	if f.setting(t, ActionStartAgent) != SettingRequiresApproval {
		t.Fatal("a failed agent-starting create did not lower start_agent")
	}
	if f.setting(t, ActionCreateTask) != SettingAutomatic {
		t.Fatal("a failed agent-starting create lowered create_task")
	}
}
