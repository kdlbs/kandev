package coordinator

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// resumeFixture is the raised automatic fixture whose service can resume a
// session and pass the action admission checks.
type resumeFixture struct {
	store    *Store
	c        *Coordinator
	svc      *Service
	resumer  *fakeResumer
	fc       *fakeContainment
	ledger   *fakeSpendLedger
	tasks    *fakeDecisionTaskService
	targetID string
	az       *identityAuthorizer
}

func newResumeFixture(t *testing.T) *resumeFixture {
	t.Helper()
	store, c, tasks, svc, _ := automaticFixture(t)
	az := &identityAuthorizer{allowed: map[string]bool{testRaiser: true}}
	svc.authz = az
	svc.SetAutomaticIdentities(fakeIdentities{known: map[string]bool{testRaiser: true}})
	f := &resumeFixture{store: store, c: c, svc: svc, tasks: tasks, resumer: &fakeResumer{started: true}, fc: contained(), ledger: &fakeSpendLedger{}, targetID: "task-0", az: az}
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 10}, nil }
	conv := newFakeConversationTasks()
	conv.tasks[ceilingConvTask] = spendConvTask(ceilingConvTask, c.ID, false)
	svc.conversationTasks = conv
	svc.SetSpendDeps(f.ledger, &fakeActiveTurns{turns: map[string]string{}}, &fakeCanceller{})
	svc.SetContainment(newChecker(f.fc))
	svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: &TargetTask{ID: f.targetID, WorkspaceID: c.WorkspaceID,
		Primary: &TargetSession{ID: "s1", State: "IDLE", HasExecutorRecord: true}}}, Resumer: f.resumer})
	mustExec(t, store, `UPDATE coordinators SET autonomy_enabled = ?, conversation_task_id = ?, cost_ceiling_subcents = ? WHERE id = ?`,
		true, ceilingConvTask, 1000, c.ID)
	return f
}

// raise sets class to automatic as testRaiser, with the change row a real
// raise writes.
func (f *resumeFixture) raise(t *testing.T, class Action, by string) string {
	t.Helper()
	stored, err := f.store.GetCoordinatorByID(context.Background(), f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := f.svc.policyFor(stored)
	p.Actions[class] = SettingAutomatic
	raw, _ := json.Marshal(p)
	mustExec(t, f.store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`, string(raw), f.c.ID)
	var earlier int
	if err := f.store.db.QueryRow(f.store.db.Rebind(`SELECT COUNT(*) FROM coordinator_class_changes WHERE coordinator_id = ? AND to_value = 'automatic'`), f.c.ID).Scan(&earlier); err != nil {
		t.Fatal(err)
	}
	err = f.store.withCoordinatorLock(context.Background(), f.c.ID, func(tx coordinatorExec) error {
		return f.store.InsertClassChangeTx(context.Background(), tx, ClassChange{
			CoordinatorID: f.c.ID, Class: class, FromValue: SettingRequiresApproval, ToValue: SettingAutomatic, ChangedBy: by,
			ChangedAt: automaticNow.Add(-time.Hour + time.Duration(earlier)*time.Second),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	change, err := f.store.NewestClassChangeTx(context.Background(), f.store.db, f.c.ID, class, SettingAutomatic)
	if err != nil || change == nil {
		t.Fatalf("raise row: %v", err)
	}
	return change.ID
}

func (f *resumeFixture) propose(t *testing.T) *Proposal {
	t.Helper()
	spec := fmt.Sprintf(`{"task_id":%q,"rationale":"stuck"}`, f.targetID)
	target := f.targetID
	p := &Proposal{CoordinatorID: f.c.ID, WorkspaceID: f.c.WorkspaceID, Kind: ProposalKindResume, TargetTaskID: &target, RawSpec: spec}
	if err := f.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *resumeFixture) try(t *testing.T, p *Proposal) *AutomaticResult {
	t.Helper()
	res, err := f.svc.TryAutomaticApproval(context.Background(), p)
	if err != nil {
		t.Fatalf("TryAutomaticApproval: %v", err)
	}
	return res
}

func (f *resumeFixture) setting(t *testing.T, class Action) Setting {
	t.Helper()
	stored, err := f.store.GetCoordinatorByID(context.Background(), f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f.svc.policyFor(stored).Actions[class]
}

func TestAutomaticResume_ApprovesAndRecordsAutomaticRow(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusApproved || res.TaskID != f.targetID {
		t.Fatalf("res = %+v", res)
	}
	if f.resumer.calls != 1 {
		t.Fatalf("resume calls = %d", f.resumer.calls)
	}
	got := reload(t, f.store, f.c, p.ID)
	if !got.DecidedAutomatically || !got.ClaimedAutomatically || got.AutomaticAt == nil || got.DecidedBy == nil || *got.DecidedBy != testRaiser {
		t.Fatalf("proposal = %+v", got)
	}
	rows := listActivity(t, f.store, f.c.ID)
	var approved *ActivityRow
	for i := range rows {
		if rows[i].Outcome == ActivityApproved {
			approved = &rows[i]
		}
	}
	if approved == nil || approved.ActionClass != ActionResume || approved.Authorization != AuthAutomatic ||
		approved.ActorUserID == nil || *approved.ActorUserID != testRaiser || approved.Edited {
		t.Fatalf("approved row = %+v", approved)
	}
}

func TestAutomaticResume_NilWhenNotAutomatic(t *testing.T) {
	f := newResumeFixture(t)
	p := f.propose(t)
	if res := f.try(t, p); res != nil {
		t.Fatalf("res = %+v, want phase 1 result", res)
	}
	if got := reload(t, f.store, f.c, p.ID); got.Status != ProposalStatusPending || f.resumer.calls != 0 {
		t.Fatalf("proposal = %+v", got)
	}
}

func TestAutomaticResume_AdmissionHoldsLeaveItPendingAndUncounted(t *testing.T) {
	cases := map[string]struct {
		spoil func(f *resumeFixture, t *testing.T)
		token string
	}{
		"autonomy off": {token: "autonomy_off", spoil: func(f *resumeFixture, t *testing.T) {
			mustExec(t, f.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.c.ID)
		}},
		"containment": {token: "containment", spoil: func(f *resumeFixture, _ *testing.T) { f.fc.mode = "disabled" }},
		"spend unmeasured": {token: "spend_unmeasured", spoil: func(f *resumeFixture, _ *testing.T) {
			f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{}, fmt.Errorf("boom") }
		}},
		"ceiling reached": {token: "ceiling_reached", spoil: func(f *resumeFixture, _ *testing.T) {
			f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 1000}, nil }
		}},
		"read error": {token: "read_error", spoil: func(f *resumeFixture, _ *testing.T) { f.svc.containment = nil }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newResumeFixture(t)
			f.raise(t, ActionResume, testRaiser)
			tc.spoil(f, t)
			heldBefore := counterValue(automaticApprovalTotal, automaticHeld)
			tokenBefore := heldCount(tc.token)
			p := f.propose(t)
			res := f.try(t, p)
			want := fmt.Sprintf("automatic action held (%s); a manager will decide", tc.token)
			if res == nil || res.Status != ProposalStatusPending || res.Note != want {
				t.Fatalf("res = %+v, want note %q", res, want)
			}
			got := reload(t, f.store, f.c, p.ID)
			if got.Status != ProposalStatusPending || got.DecidedAutomatically || f.resumer.calls != 0 {
				t.Fatalf("held proposal was touched: %+v", got)
			}
			if f.setting(t, ActionResume) != SettingAutomatic {
				t.Fatal("a hold lowered the class")
			}
			n, err := f.store.CountAutomaticDecidedTx(context.Background(), f.store.db, f.c.ID, automaticNow.Add(-automaticLimitWindow))
			if err != nil || n != 0 {
				t.Fatalf("held attempt counted toward the limit: %d %v", n, err)
			}
			if counterValue(automaticApprovalTotal, automaticHeld) != heldBefore+1 || heldCount(tc.token) != tokenBefore+1 {
				t.Fatal("hold counters did not move")
			}
		})
	}
}

func heldCount(reason string) int64 { return counterValue(automaticHeldTotal, reason) }

func counterValue(m *expvar.Map, key string) int64 {
	if v, ok := m.Get(key).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}
