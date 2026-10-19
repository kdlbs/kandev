package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

var automaticNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type fakeDecisionLog struct {
	earliest *time.Time
	rows     []Decision
	undone   []string
	err      error
}

func (f *fakeDecisionLog) EarliestDecision(context.Context, string, string) (time.Time, bool, error) {
	if f.err != nil || f.earliest == nil {
		return time.Time{}, false, f.err
	}
	return *f.earliest, true, nil
}

func (f *fakeDecisionLog) Decisions(context.Context, string, string, time.Time, time.Time) ([]Decision, error) {
	return f.rows, f.err
}

func (f *fakeDecisionLog) UndoneTaskIDs(context.Context, string, time.Time, time.Time) ([]string, error) {
	return f.undone, f.err
}

// automaticFixture is a phase 3 service on a fixed clock whose decision log is
// a fake; every condition is met until a test spoils one.
func automaticFixture(t *testing.T) (*Store, *Coordinator, *fakeDecisionTaskService, *Service, *fakeDecisionLog) {
	t.Helper()
	store, c, tasks, svc := approveFixture(t)
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	svc.phase2, svc.phase3 = true, true
	store.now = func() time.Time { return automaticNow }
	earliest := automaticNow.Add(-31 * 24 * time.Hour)
	log := &fakeDecisionLog{earliest: &earliest, rows: decisions(20, 20)}
	svc.SetAutomaticPorts(nil, log)
	recordReview(t, store, c, automaticNow.Add(-time.Hour))
	return store, c, tasks, svc, log
}

func decisions(total, approved int) []Decision {
	out := make([]Decision, total)
	for i := range out {
		outcome := DecisionApproved
		if i >= approved {
			outcome = DecisionApprovedWithEdits
		}
		out[i] = Decision{ProposalID: fmt.Sprintf("p-%d", i), Outcome: outcome, DecidedAt: automaticNow.Add(-time.Hour)}
	}
	return out
}

func recordReview(t *testing.T, store *Store, c *Coordinator, at time.Time) {
	t.Helper()
	err := store.InsertClassReview(context.Background(), ClassReview{
		CoordinatorID: c.ID, Class: ActionCreateTask, ReviewedBy: "mgr-1",
		ReviewedAt: at, WindowStart: at.Add(-evidenceWindow), WindowEnd: at,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func conditionMet(t *testing.T, res EligibilityResult, name string) bool {
	t.Helper()
	for _, c := range res.Conditions {
		if c.Name == name {
			return c.Met
		}
	}
	t.Fatalf("condition %q missing", name)
	return false
}

func TestEligibility_AllMet(t *testing.T) {
	_, c, _, svc, _ := automaticFixture(t)
	res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Eligible || res.FirstUnmet() != "" || len(res.Conditions) != 5 {
		t.Fatalf("res = %+v", res)
	}
}

func TestEligibility_Boundaries(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name      string
		condition string
		mutate    func(*fakeDecisionLog)
		wantMet   bool
	}{
		{"history 30 days", conditionHistory30d, func(l *fakeDecisionLog) { e := automaticNow.Add(-30 * day); l.earliest = &e }, true},
		{"history 29 days", conditionHistory30d, func(l *fakeDecisionLog) { e := automaticNow.Add(-29 * day); l.earliest = &e }, false},
		{"no history", conditionHistory30d, func(l *fakeDecisionLog) { l.earliest = nil }, false},
		{"20 rows", conditionVolume, func(l *fakeDecisionLog) { l.rows = decisions(20, 20) }, true},
		{"19 rows", conditionVolume, func(l *fakeDecisionLog) { l.rows = decisions(19, 19) }, false},
		{"90 percent", conditionUnedited, func(l *fakeDecisionLog) { l.rows = decisions(1000, 900) }, true},
		{"89.9 percent", conditionUnedited, func(l *fakeDecisionLog) { l.rows = decisions(1000, 899) }, false},
		{"no rows", conditionUnedited, func(l *fakeDecisionLog) { l.rows = nil }, false},
		{"no undo", conditionNoUndo, func(l *fakeDecisionLog) { l.undone = nil }, true},
		{"one undone create", conditionNoUndo, func(l *fakeDecisionLog) { l.undone = []string{"task-1"} }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c, _, svc, log := automaticFixture(t)
			tc.mutate(log)
			res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
			if err != nil {
				t.Fatal(err)
			}
			if got := conditionMet(t, res, tc.condition); got != tc.wantMet {
				t.Fatalf("%s met = %v, want %v (%+v)", tc.condition, got, tc.wantMet, res.Conditions)
			}
		})
	}
}

func TestEligibility_ReviewAge(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name    string
		age     time.Duration
		wantMet bool
	}{
		{"7 days", 7 * day, true},
		{"7 days and 1 second", 7*day + time.Second, false},
		{"8 days", 8 * day, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, c, _, svc, _ := automaticFixture(t)
			if _, err := store.db.Exec(`DELETE FROM coordinator_class_reviews`); err != nil {
				t.Fatal(err)
			}
			recordReview(t, store, c, automaticNow.Add(-tc.age))
			res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
			if err != nil {
				t.Fatal(err)
			}
			if got := conditionMet(t, res, conditionReviewed7d); got != tc.wantMet {
				t.Fatalf("reviewed_7d met = %v, want %v", got, tc.wantMet)
			}
		})
	}
}

func TestEligibility_NoReviewIsNotMet(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	if _, err := store.db.Exec(`DELETE FROM coordinator_class_reviews`); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
	if err != nil {
		t.Fatal(err)
	}
	if res.Eligible || res.FirstUnmet() != conditionReviewed7d {
		t.Fatalf("res = %+v", res)
	}
}

func TestEligibility_ReadErrorPropagates(t *testing.T) {
	_, c, _, svc, log := automaticFixture(t)
	log.err = errors.New("boom")
	if _, err := svc.Eligibility(context.Background(), c.ID, automaticNow); err == nil {
		t.Fatal("want error")
	}
}

func TestEligibility_StoreLogExcludesAutomaticRows(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("auto-%d", i)
		seedApproved(t, store, c, id, ActionCreateTask, automaticNow.Add(-31*24*time.Hour+time.Duration(i)*time.Hour), func(r *ActivityRow) { r.Authorization = AuthAutomatic })
	}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("mgr-%d", i)
		seedApproved(t, store, c, id, ActionCreateTask, automaticNow.Add(-time.Duration(i+1)*time.Hour), nil)
	}
	res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
	if err != nil {
		t.Fatal(err)
	}
	if conditionMet(t, res, conditionVolume) || conditionMet(t, res, conditionHistory30d) {
		t.Fatalf("automatic rows counted as decided: %+v", res.Conditions)
	}
}

func TestEligibility_StoreLogCountsUndoneCreate(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	seedApproved(t, store, c, "r1", ActionCreateTask, automaticNow.Add(-2*time.Hour), func(r *ActivityRow) {
		task := "t1"
		undone := automaticNow.Add(-time.Hour)
		r.TargetTaskID = &task
		r.UndoneAt = &undone
	})
	res, err := svc.Eligibility(context.Background(), c.ID, automaticNow)
	if err != nil {
		t.Fatal(err)
	}
	if conditionMet(t, res, conditionNoUndo) {
		t.Fatalf("undone create not counted: %+v", res.Conditions)
	}
}

func raiseBody() string { return policyBody(map[string]string{"create_task": "automatic"}) }

func TestRaise_RefusedNamesFirstUnmet(t *testing.T) {
	store, c, _, svc, log := automaticFixture(t)
	log.rows = decisions(19, 19)
	log.undone = []string{"t"}
	_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(raiseBody()))
	var refused *RaiseRefusedError
	if !errors.As(err, &refused) || refused.Condition != conditionVolume || refused.Class != ActionCreateTask {
		t.Fatalf("err = %v", err)
	}
	stored, err := store.GetCoordinatorByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if svc.policyFor(stored).Actions[ActionCreateTask] == SettingAutomatic {
		t.Fatal("refused raise was stored")
	}
}

func TestRaise_LogErrorRefuses(t *testing.T) {
	_, c, _, svc, log := automaticFixture(t)
	log.err = errors.New("boom")
	_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(raiseBody()))
	var refused *RaiseRefusedError
	if !errors.As(err, &refused) || refused.Condition != decisionLogField {
		t.Fatalf("err = %v", err)
	}
}

func TestRaise_StoresChangeAndBumpsRevision(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	before, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(raiseBody())); err != nil {
		t.Fatal(err)
	}
	after, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if after.PolicyRevision != before.PolicyRevision+1 {
		t.Fatalf("policy_revision %d -> %d", before.PolicyRevision, after.PolicyRevision)
	}
	change, err := store.NewestClassChangeTx(context.Background(), store.ro, c.ID, ActionCreateTask, SettingAutomatic)
	if err != nil || change == nil || change.ChangedBy != "mgr-1" || change.FromValue != SettingRequiresApproval {
		t.Fatalf("change = %+v err = %v", change, err)
	}
}

func TestRaise_UnchangedAutomaticIsNotRechecked(t *testing.T) {
	_, c, _, svc, log := automaticFixture(t)
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(raiseBody())); err != nil {
		t.Fatal(err)
	}
	log.rows = nil
	body := policyBody(map[string]string{"create_task": "automatic", "move": "requires_approval"})
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(body)); err != nil {
		t.Fatalf("unrelated change refused: %v", err)
	}
}

func TestRaise_LoweringIsNeverChecked(t *testing.T) {
	_, c, _, svc, log := automaticFixture(t)
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(raiseBody())); err != nil {
		t.Fatal(err)
	}
	log.err = errors.New("boom")
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(policyBody(nil))); err != nil {
		t.Fatalf("lower refused: %v", err)
	}
}

func TestRaise_OtherClassesRefusedByValidate(t *testing.T) {
	_, c, _, svc, _ := automaticFixture(t)
	for _, class := range AllActions {
		if class == ActionCreateTask {
			continue
		}
		_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(policyBody(map[string]string{string(class): "automatic"})))
		var se *SettingsError
		if !errors.As(err, &se) || se.Field != "policy.actions."+string(class) || se.Code != "automatic_not_available" {
			t.Fatalf("%s: err = %v", class, err)
		}
	}
}

func TestRaise_CreateTaskRefusedWhenPhase3Off(t *testing.T) {
	_, c, _, svc, _ := automaticFixture(t)
	svc.phase3 = false
	_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(raiseBody()))
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != "automatic_not_available" {
		t.Fatalf("err = %v", err)
	}
}

func TestValidate_AutomaticOnlyForRaisableClassWithPhase3(t *testing.T) {
	p := PhaseOnePolicy()
	p.Actions[ActionCreateTask] = SettingAutomatic
	if err := Validate(p, true); err != nil {
		t.Fatalf("phase3 create_task: %v", err)
	}
	if err := Validate(p, false); err == nil {
		t.Fatal("phase 2 accepted automatic")
	}
	p = PhaseOnePolicy()
	p.Actions[ActionMove] = SettingAutomatic
	if err := Validate(p, true); err == nil {
		t.Fatal("move accepted automatic")
	}
}
