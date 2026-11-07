package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func (f *resumeFixture) lowerRows(t *testing.T, class Action) []ClassChange {
	t.Helper()
	var n int
	if err := f.store.db.GetContext(context.Background(), &n, f.store.db.Rebind(
		`SELECT COUNT(*) FROM coordinator_class_changes WHERE coordinator_id = ? AND class = ? AND to_value = 'requires_approval' AND reason = ?`),
		f.c.ID, string(class), failureLoweredReason); err != nil {
		t.Fatal(err)
	}
	return make([]ClassChange, n)
}

func TestAutomaticResume_FailureSettlesAutomaticAndLowersTheClass(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	f.resumer.err = errors.New("no runtime")
	p := f.propose(t)
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusFailed {
		t.Fatalf("res = %+v", res)
	}
	if f.setting(t, ActionResume) != SettingRequiresApproval || len(f.lowerRows(t, ActionResume)) != 1 {
		t.Fatalf("failure did not lower resume: %v", f.setting(t, ActionResume))
	}
	var failed *ActivityRow
	rows := listActivity(t, f.store, f.c.ID)
	for i := range rows {
		if rows[i].Outcome == ActivityFailed {
			failed = &rows[i]
		}
	}
	if failed == nil || failed.Authorization != AuthAutomatic || failed.ActorUserID == nil || *failed.ActorUserID != testRaiser || failed.Edited {
		t.Fatalf("failed row = %+v", failed)
	}
}

func TestAutomaticResume_StaleClaimSettlesFailedAndLowers(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	at := automaticNow.Add(-time.Hour)
	mustExec(t, f.store, `UPDATE coordinator_proposals SET status = 'approving', claimed_at = ?, claim_token = 'x',
		decided_automatically = 1, claimed_automatically = 1, automatic_at = ?, decided_by = ? WHERE id = ?`, at, at, testRaiser, p.ID)
	row, err := f.store.GetProposalAnyKind(context.Background(), p.WorkspaceID, p.CoordinatorID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The raise precedes the stale attempt's date only when it is older than it.
	mustExec(t, f.store, `UPDATE coordinator_class_changes SET changed_at = ? WHERE coordinator_id = ?`, at.Add(-time.Hour), f.c.ID)
	if _, err := f.svc.settleStaleKind(context.Background(), row, f.svc.kindExecutor(ProposalKindResume), automaticNow); err != nil {
		t.Fatal(err)
	}
	if f.setting(t, ActionResume) != SettingRequiresApproval {
		t.Fatal("a stale automatic resume claim did not lower the class")
	}
	if got := reload(t, f.store, f.c, p.ID); got.Status != ProposalStatusFailed {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestAutomaticFailureHook_GuardedByTheAttemptDate(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser) // raised at automaticNow-1h
	earlier := automaticNow.Add(-2 * time.Hour)
	f.svc.afterAutomaticFailure(context.Background(), &automaticAttempt{coordinatorID: f.c.ID, classes: []Action{ActionResume}, automaticAt: &earlier})
	if f.setting(t, ActionResume) != SettingAutomatic {
		t.Fatal("an attempt that preceded the raise lowered it")
	}
	later := automaticNow
	f.svc.afterAutomaticFailure(context.Background(), &automaticAttempt{coordinatorID: f.c.ID, classes: []Action{ActionCreateTask, ActionResume}, automaticAt: &later})
	if f.setting(t, ActionResume) != SettingRequiresApproval {
		t.Fatal("a failure after the raise did not lower it")
	}
	f.svc.afterAutomaticFailure(context.Background(), nil)
}

func TestAutomaticResume_ReRaiseAfterReadIsDetectedUnderTheLock(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	var once sync.Once
	f.az.onCall = func() {
		once.Do(func() {
			if _, err := f.svc.LowerClass(context.Background(), f.c.ID, ActionResume, "manager", LowerGuard{}); err != nil {
				t.Error(err)
			}
			f.raise(t, ActionResume, testRaiser)
		})
	}
	res := f.try(t, p)
	if res != nil {
		t.Fatalf("res = %+v, want phase 1 result after a lower and re-raise", res)
	}
	if got := reload(t, f.store, f.c, p.ID); got.Status != ProposalStatusPending || got.DecidedAutomatically || f.resumer.calls != 0 {
		t.Fatalf("proposal = %+v", got)
	}
}

func TestAutomaticResume_TwoCallersOneDecider(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	var wg sync.WaitGroup
	results := make([]*AutomaticResult, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := f.svc.TryAutomaticApproval(context.Background(), p)
			if err != nil {
				t.Error(err)
			}
			results[i] = res
		}()
	}
	wg.Wait()
	if f.resumer.calls != 1 {
		t.Fatalf("resume ran %d times, want exactly one decider", f.resumer.calls)
	}
}

func TestAutomaticResume_FailedProposalIsNeverClaimedAutomatically(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	mustExec(t, f.store, `UPDATE coordinator_proposals SET status = 'failed' WHERE id = ?`, p.ID)
	res := f.try(t, p)
	if got := reload(t, f.store, f.c, p.ID); got.Status != ProposalStatusFailed || got.DecidedAutomatically || f.resumer.calls != 0 {
		t.Fatalf("res = %+v proposal = %+v", res, got)
	}
}

func TestAutomaticResume_PausedUnderTheLockLeavesItPending(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	p := f.propose(t)
	f.az.onCall = func() {
		mustExec(t, f.store, `UPDATE coordinators SET paused_at = ? WHERE id = ?`, automaticNow, f.c.ID)
	}
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusPending || res.Note != pausedNote || f.resumer.calls != 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestAutomaticResume_InvalidRaiserLowersAndLeavesPending(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, "gone")
	p := f.propose(t)
	res := f.try(t, p)
	if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
		t.Fatalf("res = %+v", res)
	}
	if f.setting(t, ActionResume) != SettingRequiresApproval || f.resumer.calls != 0 {
		t.Fatal("invalid raiser did not lower the class")
	}
}
