package coordinator

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func (f *resumeFixture) reject(t *testing.T, p *Proposal) *Proposal {
	t.Helper()
	got, err := f.svc.RejectProposal(context.Background(), f.c.WorkspaceID, f.c.ID, p.ID, RejectProposalRequest{})
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	return got
}

func TestOverride_RejectLowersTheRaisedClass(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	got := f.reject(t, f.propose(t))
	if got.LoweredClass != "resume" || f.setting(t, ActionResume) != SettingRequiresApproval {
		t.Fatalf("lowered = %q setting = %v", got.LoweredClass, f.setting(t, ActionResume))
	}
	dto := NewProposalDTOPhases(got, true, true)
	if dto.ProposalPhase3 == nil || dto.LoweredClass != "resume" {
		t.Fatalf("response does not name the lowered class: %+v", dto.ProposalPhase3)
	}
}

func TestOverride_NothingToLowerReportsNothing(t *testing.T) {
	f := newResumeFixture(t)
	if got := f.reject(t, f.propose(t)); got.LoweredClass != "" {
		t.Fatalf("lowered = %q with resume not automatic", got.LoweredClass)
	}
	f.raise(t, ActionCreateTask, testRaiser)
	plain := insertKind(t, f.store, f.c, ProposalKindCreateTask)
	if got := f.reject(t, plain); got.LoweredClass != "" || f.setting(t, ActionCreateTask) != SettingAutomatic {
		t.Fatalf("a create_task reject lowered %q", got.LoweredClass)
	}
}

func TestOverride_ARaiseAfterTheDecisionIsNotLowered(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	mustExec(t, f.store, `UPDATE coordinator_class_changes SET changed_at = ? WHERE coordinator_id = ?`, automaticNow.Add(time.Hour), f.c.ID)
	if got := f.reject(t, f.propose(t)); got.LoweredClass != "" || f.setting(t, ActionResume) != SettingAutomatic {
		t.Fatalf("a raise dated after the decision was lowered: %q", got.LoweredClass)
	}
}

func TestOverride_ReplyReturnsAndLowers(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	got, err := f.svc.ReplyToProposal(context.Background(), f.c.WorkspaceID, f.c.ID, p.ID, ReplyRequest{Text: "not now"})
	if err != nil {
		t.Fatalf("ReplyToProposal: %v", err)
	}
	if got.LoweredClass != "resume" || f.setting(t, ActionResume) != SettingRequiresApproval {
		t.Fatalf("lowered = %q", got.LoweredClass)
	}
}

func TestOverride_EditedStartAgentApprovalLowersStartAgent(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionCreateTask, testRaiser)
	f.raise(t, ActionStartAgent, testRaiser)
	p := f.proposeStarting(t)
	edits := ApproveProposalRequest{ApproveFieldTitle: json.RawMessage(`"A different title"`)}
	got, err := f.svc.ApproveProposal(context.Background(), f.c.WorkspaceID, f.c.ID, p.ID, edits)
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got.LoweredClass != "start_agent" || f.setting(t, ActionStartAgent) != SettingRequiresApproval || f.setting(t, ActionCreateTask) != SettingAutomatic {
		t.Fatalf("lowered = %q start_agent=%v create_task=%v", got.LoweredClass, f.setting(t, ActionStartAgent), f.setting(t, ActionCreateTask))
	}
}

func TestOverride_UneditedManagerApprovalLowersNothing(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	got, err := f.svc.ApproveProposal(context.Background(), f.c.WorkspaceID, f.c.ID, p.ID, ApproveProposalRequest{})
	if err != nil || got.Status != ProposalStatusApproved || got.LoweredClass != "" || f.setting(t, ActionResume) != SettingAutomatic {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
