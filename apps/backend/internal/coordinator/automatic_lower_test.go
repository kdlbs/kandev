package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func countClassChanges(t *testing.T, store *Store, c *Coordinator, to Setting) int {
	t.Helper()
	var n int
	err := store.db.GetContext(context.Background(), &n, store.db.Rebind(
		`SELECT COUNT(*) FROM coordinator_class_changes WHERE coordinator_id = ? AND to_value = ?`), c.ID, string(to))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func undoFixture(t *testing.T) (*Store, *Coordinator, *fakeDecisionTaskService, *Service, *identityAuthorizer) {
	t.Helper()
	store, c, tasks, svc, az := raisedFixture(t)
	az.allowed["mgr-2"] = true
	svc.SetUndoDeps(&fakeUndoTasks{tasks: map[string]*UndoTask{}, steps: map[string]*UndoStep{}, admitted: true})
	return store, c, tasks, svc, az
}

func activityIDFor(t *testing.T, store *Store, c *Coordinator, proposalID string, outcome ActivityOutcome) string {
	t.Helper()
	for _, r := range listActivity(t, store, c.ID) {
		if r.ProposalID != nil && *r.ProposalID == proposalID && r.Outcome == outcome {
			return r.ID
		}
	}
	t.Fatalf("no %s row for %s", outcome, proposalID)
	return ""
}

func TestLowerClass_NoOpWhenNotAutomatic(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	before, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if err := svc.LowerClass(context.Background(), c.ID, ActionCreateTask, "x"); err != nil {
		t.Fatal(err)
	}
	after, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if after.PolicyRevision != before.PolicyRevision || countClassChanges(t, store, c, SettingRequiresApproval) != 0 {
		t.Fatal("lower of a non-automatic class wrote")
	}
}

func TestLowerClass_WritesChangeAndBumpsRevision(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	before, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if err := svc.LowerClass(context.Background(), c.ID, ActionCreateTask, "because"); err != nil {
		t.Fatal(err)
	}
	after, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if after.PolicyRevision != before.PolicyRevision+1 || settingOf(t, store, svc, c) != SettingRequiresApproval {
		t.Fatalf("revision %d -> %d", before.PolicyRevision, after.PolicyRevision)
	}
	change, _ := store.NewestClassChangeTx(context.Background(), store.ro, c.ID, ActionCreateTask, SettingRequiresApproval)
	if change == nil || change.ChangedBy != "" || change.Reason != "because" || change.FromValue != SettingAutomatic {
		t.Fatalf("change = %+v", change)
	}
}

func TestUndo_OfAutomaticCreateLowersTheClass(t *testing.T) {
	store, c, _, svc, _ := undoFixture(t)
	p, _ := proposeAuto(t, store, c, svc)
	rowID := activityIDFor(t, store, c, p.ID, ActivityApproved)
	if _, err := svc.UndoActivity(authedContext("mgr-2"), c.WorkspaceID, c.ID, rowID); err != nil {
		t.Fatal(err)
	}
	if settingOf(t, store, svc, c) != SettingRequiresApproval {
		t.Fatal("class not lowered by the undo")
	}
	change, _ := store.NewestClassChangeTx(context.Background(), store.ro, c.ID, ActionCreateTask, SettingRequiresApproval)
	if change == nil || change.Reason != undoLoweredReason {
		t.Fatalf("change = %+v", change)
	}
}

func TestUndo_OfManagerApprovalAfterFailedAutomaticDoesNotLower(t *testing.T) {
	store, c, tasks, svc, _ := undoFixture(t)
	tasks.createErr = errors.New("create failed")
	p, _ := proposeAuto(t, store, c, svc)
	tasks.createErr = nil
	if _, err := svc.ApproveProposal(authedContext("mgr-2"), c.WorkspaceID, c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	rowID := activityIDFor(t, store, c, p.ID, ActivityApproved)
	if _, err := svc.UndoActivity(authedContext("mgr-2"), c.WorkspaceID, c.ID, rowID); err != nil {
		t.Fatal(err)
	}
	if settingOf(t, store, svc, c) != SettingAutomatic {
		t.Fatal("undo of a manager-approved create lowered the class")
	}
	rows, err := store.DecidedRows(context.Background(), c.ID, automaticNow.Add(-time.Hour), automaticNow.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Outcome != string(ActivityApproved) {
		t.Fatalf("decided rows = %+v err = %v", rows, err)
	}
}

func TestLoweringDuty_RetriesAFailedLowerOnce(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	p, _ := proposeAuto(t, store, c, svc)
	mustExec(t, store, `UPDATE coordinator_activity SET undone_at = ? WHERE proposal_id = ?`, automaticNow, p.ID)
	duty := svc.LoweringDuty()
	if err := duty(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if settingOf(t, store, svc, c) != SettingRequiresApproval {
		t.Fatal("duty did not lower")
	}
	if err := duty(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if n := countClassChanges(t, store, c, SettingRequiresApproval); n != 1 {
		t.Fatalf("lower changes = %d, want 1", n)
	}
}

func TestLoweringDuty_IgnoresManagerClaimedUndo(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	tasks.createErr = errors.New("create failed")
	p, _ := proposeAuto(t, store, c, svc)
	tasks.createErr = nil
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'pending' WHERE id = ?`, p.ID)
	if _, err := svc.ApproveProposal(authedContext(testRaiser), c.WorkspaceID, c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, store, `UPDATE coordinator_activity SET undone_at = ? WHERE proposal_id = ? AND outcome = 'approved'`, automaticNow, p.ID)
	if err := svc.LoweringDuty()(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if settingOf(t, store, svc, c) != SettingAutomatic {
		t.Fatal("duty lowered for a manager-claimed task")
	}
}

func TestLoweringDuty_SkipsWhenNotAutomaticOrPhase3Off(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	if err := svc.LoweringDuty()(context.Background(), c.ID); err != nil || countClassChanges(t, store, c, SettingRequiresApproval) != 0 {
		t.Fatalf("err = %v", err)
	}
	svc.phase3 = false
	if err := svc.LoweringDuty()(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAutomatic_VolumeIgnoresAutomaticApprovalsAfterLowerAndReRaise(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	for i := 0; i < 20; i++ {
		seedApproved(t, store, c, fmt.Sprintf("auto-%d", i), ActionCreateTask, automaticNow.Add(-time.Duration(i+1)*time.Hour), func(r *ActivityRow) { r.Authorization = AuthAutomatic })
	}
	for i := 0; i < 5; i++ {
		seedApproved(t, store, c, fmt.Sprintf("mgr-%d", i), ActionCreateTask, automaticNow.Add(-31*24*time.Hour+time.Duration(i)*time.Hour), nil)
	}
	if err := svc.LowerClass(context.Background(), c.ID, ActionCreateTask, "manager"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SaveSettings(authedContext(testRaiser), c.WorkspaceID, c.ID, []byte(raiseBody()))
	var refused *RaiseRefusedError
	if !errors.As(err, &refused) || refused.Condition != conditionVolume {
		t.Fatalf("err = %v", err)
	}
}
