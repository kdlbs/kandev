package coordinator

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func (f *resumeFixture) lastChangeReason(t *testing.T, class Action) string {
	t.Helper()
	change, err := f.store.NewestClassChangeTx(context.Background(), f.store.db, f.c.ID, class, SettingRequiresApproval)
	if err != nil || change == nil {
		t.Fatalf("no lowering of %s: %v", class, err)
	}
	return change.Reason
}

// approvedResume raises resume, lets the automatic path approve one proposal
// and returns it.
func (f *resumeFixture) approvedResume(t *testing.T) *Proposal {
	t.Helper()
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	if res := f.try(t, p); res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("res = %+v", res)
	}
	return p
}

func TestLoweringDuty_LowersResumeAfterAFailedAutomaticRow(t *testing.T) {
	f := newResumeFixture(t)
	p := f.approvedResume(t)
	mustExec(t, f.store, `UPDATE coordinator_activity SET outcome = 'failed' WHERE proposal_id = ?`, p.ID)
	mustExec(t, f.store, `UPDATE coordinator_proposals SET automatic_at = ? WHERE id = ?`, automaticNow, p.ID)
	if err := f.svc.LoweringDuty()(context.Background(), f.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.setting(t, ActionResume) != SettingRequiresApproval || f.lastChangeReason(t, ActionResume) != failureLoweredReason {
		t.Fatalf("resume = %s", f.setting(t, ActionResume))
	}
}

func TestLoweringDuty_IgnoresEvidenceNotAfterTheRaise(t *testing.T) {
	f := newResumeFixture(t)
	p := f.approvedResume(t)
	mustExec(t, f.store, `UPDATE coordinator_activity SET outcome = 'failed' WHERE proposal_id = ?`, p.ID)
	mustExec(t, f.store, `UPDATE coordinator_proposals SET automatic_at = ? WHERE id = ?`, automaticNow.Add(-time.Hour), p.ID)
	if err := f.svc.LoweringDuty()(context.Background(), f.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.setting(t, ActionResume) != SettingAutomatic {
		t.Fatal("evidence dated at the raise lowered the class")
	}
}

func TestLoweringDuty_LowersResumeAfterAManagerOverride(t *testing.T) {
	f := newResumeFixture(t)
	p := f.approvedResume(t)
	mustExec(t, f.store, `UPDATE coordinator_activity SET outcome = 'rejected', "authorization" = 'requires_approval' WHERE proposal_id = ?`, p.ID)
	if err := f.svc.LoweringDuty()(context.Background(), f.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.setting(t, ActionResume) != SettingRequiresApproval || f.lastChangeReason(t, ActionResume) != overrideLoweredReason {
		t.Fatalf("resume = %s", f.setting(t, ActionResume))
	}
}

func TestLoweringDuty_LeavesAClassWithNoEvidence(t *testing.T) {
	f := newResumeFixture(t)
	f.approvedResume(t)
	if err := f.svc.LoweringDuty()(context.Background(), f.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.setting(t, ActionResume) != SettingAutomatic {
		t.Fatal("a clean class was lowered")
	}
}

func TestUndo_OfAnAutomaticAgentStartingCreateLowersBothClasses(t *testing.T) {
	store, c, _, svc, _ := undoFixture(t)
	stored, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	pol := svc.policyFor(stored)
	pol.Actions[ActionStartAgent] = SettingAutomatic
	raw, _ := json.Marshal(pol)
	mustExec(t, store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`, string(raw), c.ID)
	mustExec(t, store, `INSERT INTO coordinator_class_changes (id, coordinator_id, class, from_value, to_value, changed_by, changed_at)
		VALUES ('chg-sa', ?, 'start_agent', 'requires_approval', 'automatic', ?, ?)`, c.ID, testRaiser, automaticNow.Add(-time.Hour))
	p, _ := proposeAuto(t, store, c, svc)
	mustExec(t, store, `UPDATE coordinator_proposals SET starts_agent = 1 WHERE id = ?`, p.ID)
	rowID := activityIDFor(t, store, c, p.ID, ActivityApproved)
	if _, err := svc.UndoActivity(authedContext("mgr-2"), c.WorkspaceID, c.ID, rowID); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.GetCoordinatorByID(context.Background(), c.ID)
	got := svc.policyFor(stored).Actions
	if got[ActionCreateTask] != SettingRequiresApproval || got[ActionStartAgent] != SettingRequiresApproval {
		t.Fatalf("actions = %v", got)
	}
}

func TestVisitSet_IncludesACoordinatorWithARaisedClass(t *testing.T) {
	e := newWakeEnv(t)
	raised := newTestCoordinator(t, e.store, "ws-9")
	mustExec(t, e.store, `INSERT INTO coordinator_class_changes (id, coordinator_id, class, from_value, to_value, changed_at)
		VALUES ('chg-1', ?, 'resume', 'requires_approval', 'automatic', ?)`, raised.ID, time.Now().UTC())
	for _, id := range e.svc.backstop.visitSet(context.Background(), time.Now().UTC()) {
		if id == raised.ID {
			return
		}
	}
	t.Fatal("a coordinator with a raised class is not visited")
}

func TestListActivityRows_StartAgentFilterFollowsTheProposal(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionCreateTask, testRaiser)
	f.raise(t, ActionStartAgent, testRaiser)
	starting := f.proposeStarting(t)
	if res := f.try(t, starting); res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("res = %+v", res)
	}
	plain := insertKind(t, f.store, f.c, ProposalKindCreateTask)
	if res := f.try(t, plain); res == nil {
		t.Fatal("plain create not handled")
	}
	rows, err := f.store.ListActivityRows(context.Background(), f.c.ID, ActionStartAgent, nil, 50)
	if err != nil || len(rows) == 0 {
		t.Fatalf("rows = %d err = %v", len(rows), err)
	}
	for _, r := range rows {
		if r.ProposalID == nil || *r.ProposalID != starting.ID {
			t.Fatalf("row of another proposal in the start_agent list: %+v", r)
		}
	}
}
