package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// realLogFixture is the automatic fixture on the store-backed decision log, so
// per-class evidence comes from coordinator_activity rows.
func realLogFixture(t *testing.T) (*Store, *Coordinator, *Service) {
	t.Helper()
	store, c, _, svc, _ := automaticFixture(t)
	svc.SetAutomaticPorts(nil, nil)
	return store, c, svc
}

// seedClassRow inserts one activity row of the kind class; for start_agent
// the row belongs to a proposal whose starts_agent flag is set.
func seedClassRow(t *testing.T, store *Store, c *Coordinator, class Action, id string, outcome ActivityOutcome, auth ActivityAuthorization, edited bool, at time.Time) {
	t.Helper()
	row := validRow(c.ID)
	row.ID, row.Outcome, row.Authorization, row.Edited = id, outcome, auth, edited
	row.CreatedAt, row.UpdatedAt = at, at
	row.ActionClass = class
	if class == ActionStartAgent {
		row.ActionClass = ActionCreateTask
		p := insertProposal(t, store, c, ProposalSpec{Title: "t-" + id, WorkflowID: "wf", StepID: "step-1", RepositoryID: "repo"})
		mustExec(t, store, `UPDATE coordinator_proposals SET starts_agent = 1 WHERE id = ?`, p.ID)
		row.ProposalID = &p.ID
	}
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
}

// seedEarnedClass seeds the evidence that makes class eligible under the
// default settings except for the review.
func seedEarnedClass(t *testing.T, store *Store, c *Coordinator, class Action, rows int) {
	t.Helper()
	seedClassRow(t, store, c, class, "old-"+string(class), ActivityApproved, AuthRequiresApproval, false, automaticNow.Add(-31*24*time.Hour))
	for i := 0; i < rows; i++ {
		seedClassRow(t, store, c, class, fmt.Sprintf("row-%s-%d", class, i), ActivityApproved, AuthRequiresApproval, false, automaticNow.Add(-time.Hour))
	}
}

func eligibilityOf(t *testing.T, svc *Service, c *Coordinator, class Action) EligibilityResult {
	t.Helper()
	view, err := svc.GetEligibility(context.Background(), c.WorkspaceID, c.ID, string(class))
	if err != nil {
		t.Fatalf("GetEligibility(%s): %v", class, err)
	}
	return view.Result
}

func TestEligibility_PerClassGateInterleavings(t *testing.T) {
	type spoil func(t *testing.T, store *Store, c *Coordinator, class Action)
	cases := map[string]struct {
		spoil spoil
		unmet string
	}{
		"all earned": {spoil: func(*testing.T, *Store, *Coordinator, Action) {}},
		"history too young": {unmet: conditionHistory30d, spoil: func(t *testing.T, store *Store, _ *Coordinator, _ Action) {
			mustExec(t, store, `UPDATE coordinator_activity SET created_at = ? WHERE id LIKE 'old-%'`, automaticNow.Add(-time.Hour))
		}},
		"too few rows": {unmet: conditionVolume, spoil: func(t *testing.T, store *Store, _ *Coordinator, _ Action) {
			mustExec(t, store, `DELETE FROM coordinator_activity WHERE id LIKE 'row-%' AND id NOT LIKE '%-0' AND id NOT LIKE '%-1'`)
		}},
		"too many edits": {unmet: conditionUnedited, spoil: func(t *testing.T, store *Store, _ *Coordinator, _ Action) {
			mustExec(t, store, `UPDATE coordinator_activity SET edited = 1 WHERE id LIKE 'row-%-1%'`)
		}},
		"failed automatic action": {unmet: conditionNoUndo, spoil: func(t *testing.T, store *Store, c *Coordinator, class Action) {
			if class == ActionCreateTask {
				t.Skip("create_task no_undo reads undone tasks")
			}
			seedClassRow(t, store, c, class, "failed-1", ActivityFailed, AuthAutomatic, false, automaticNow.Add(-time.Hour))
		}},
		"no review": {unmet: conditionReviewed7d, spoil: func(t *testing.T, store *Store, _ *Coordinator, _ Action) {
			mustExec(t, store, `DELETE FROM coordinator_class_reviews`)
		}},
	}
	for _, class := range []Action{ActionCreateTask, ActionResume, ActionStartAgent} {
		for name, tc := range cases {
			t.Run(string(class)+"/"+name, func(t *testing.T) {
				store, c, svc := realLogFixture(t)
				mustExec(t, store, `DELETE FROM coordinator_class_reviews`)
				seedEarnedClass(t, store, c, class, 20)
				recordClassReviewFor(t, store, c, class, automaticNow.Add(-time.Hour))
				tc.spoil(t, store, c, class)
				res := eligibilityOf(t, svc, c, class)
				if got := res.FirstUnmet(); got != tc.unmet {
					t.Fatalf("first unmet = %q, want %q (%+v)", got, tc.unmet, res.Conditions)
				}
				if res.Eligible != (tc.unmet == "") {
					t.Fatalf("eligible = %v", res.Eligible)
				}
			})
		}
	}
}

func TestEligibility_ClassEvidenceIsSeparate(t *testing.T) {
	store, c, svc := realLogFixture(t)
	seedEarnedClass(t, store, c, ActionResume, 20)
	if got := eligibilityOf(t, svc, c, ActionStartAgent).FirstUnmet(); got != conditionHistory30d {
		t.Fatalf("start_agent first unmet = %q, want history (resume rows must not count)", got)
	}
	if got := eligibilityOf(t, svc, c, ActionCreateTask).FirstUnmet(); got != conditionHistory30d {
		t.Fatalf("create_task first unmet = %q, want history", got)
	}
}

func TestEligibility_StartAgentRowAlsoCountsForItsKindClass(t *testing.T) {
	store, c, svc := realLogFixture(t)
	seedEarnedClass(t, store, c, ActionStartAgent, 20)
	mustExec(t, store, `DELETE FROM coordinator_class_reviews`)
	recordClassReviewFor(t, store, c, ActionCreateTask, automaticNow.Add(-time.Hour))
	if res := eligibilityOf(t, svc, c, ActionCreateTask); !res.Eligible {
		t.Fatalf("create_task not eligible from starts_agent rows: %+v", res.Conditions)
	}
}

func TestEligibility_SystemLowerCountsAndManagerLowerDoesNot(t *testing.T) {
	store, c, svc := realLogFixture(t)
	seedEarnedClass(t, store, c, ActionResume, 20)
	recordClassReviewFor(t, store, c, ActionResume, automaticNow.Add(-time.Hour))
	ins := func(changedBy, reason string) {
		t.Helper()
		err := store.InsertClassChangeTx(context.Background(), store.db, ClassChange{
			CoordinatorID: c.ID, Class: ActionResume, FromValue: SettingAutomatic, ToValue: SettingRequiresApproval,
			ChangedBy: changedBy, Reason: reason, ChangedAt: automaticNow.Add(-time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	ins("mgr-1", "")
	ins("", "") // a manager's save with auth off: no identity, no reason
	ins("", overrideLoweredReason)
	ins("", raiserLoweredReason)
	if res := eligibilityOf(t, svc, c, ActionResume); !res.Eligible {
		t.Fatalf("manager, override and raiser-lost lowers counted: %+v", res.Conditions)
	}
	ins("", failureLoweredReason)
	if got := eligibilityOf(t, svc, c, ActionResume).FirstUnmet(); got != conditionNoUndo {
		t.Fatalf("first unmet = %q, want no_undo", got)
	}
}

func TestEligibility_SettingsInOneDayThreeDecisionsExample(t *testing.T) {
	store, c, svc := realLogFixture(t)
	mustExec(t, store, `DELETE FROM coordinator_class_reviews`)
	eligibility := `{"evidence_window_hours":24,"min_decisions":3,"unedited_floor_percent":90,"review_max_age_hours":168,"daily_limit":10}`
	mustExec(t, store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`,
		`{"version":1,"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"requires_approval","stop":"denied"},"eligibility":`+eligibility+`}`, c.ID)
	seedClassRow(t, store, c, ActionResume, "old", ActivityApproved, AuthRequiresApproval, false, automaticNow.Add(-25*time.Hour))
	for i := 0; i < 3; i++ {
		seedClassRow(t, store, c, ActionResume, fmt.Sprintf("r-%d", i), ActivityApproved, AuthRequiresApproval, false, automaticNow.Add(-time.Duration(i+1)*time.Hour))
	}
	recordClassReviewFor(t, store, c, ActionResume, automaticNow.Add(-time.Minute))
	if res := eligibilityOf(t, svc, c, ActionResume); !res.Eligible {
		t.Fatalf("not eligible: %+v", res.Conditions)
	}
}

func TestEligibility_UneditedTestKeepsZeroRowsUnmet(t *testing.T) {
	store, c, svc := realLogFixture(t)
	mustExec(t, store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`,
		`{"version":1,"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"},"eligibility":{"evidence_window_hours":24,"min_decisions":3,"unedited_floor_percent":50,"review_max_age_hours":168,"daily_limit":10}}`, c.ID)
	res := eligibilityOf(t, svc, c, ActionResume)
	for _, cond := range res.Conditions {
		if cond.Name == conditionUnedited && cond.Met {
			t.Fatal("unedited_rate met with no decided rows")
		}
	}
}

func TestRaise_ResumeAndStartAgentGatedPerClass(t *testing.T) {
	for _, class := range []Action{ActionResume, ActionStartAgent} {
		t.Run(string(class), func(t *testing.T) {
			store, c, svc := realLogFixture(t)
			body := policyBody(map[string]string{string(class): "automatic"})
			_, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(body))
			var refused *RaiseRefusedError
			if !errors.As(err, &refused) || refused.Class != class || refused.Condition != conditionHistory30d {
				t.Fatalf("err = %v, want 409 history", err)
			}
			seedEarnedClass(t, store, c, class, 20)
			recordClassReviewFor(t, store, c, class, automaticNow.Add(-time.Hour))
			if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(body)); err != nil {
				t.Fatalf("raise: %v", err)
			}
			got, err := store.GetCoordinatorByID(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if svc.policyFor(got).Actions[class] != SettingAutomatic {
				t.Fatalf("class not automatic: %v", svc.policyFor(got).Actions)
			}
		})
	}
}

func TestGetEligibility_StartAgentAndResumeRoutesAcceptedOtherRefused(t *testing.T) {
	_, c, svc := realLogFixture(t)
	for _, class := range []string{"resume", "start_agent"} {
		view, err := svc.GetEligibility(context.Background(), c.WorkspaceID, c.ID, class)
		if err != nil || view.Settings != DefaultEligibility() {
			t.Fatalf("%s: view = %+v err = %v", class, view, err)
		}
	}
	var classErr *ClassError
	if _, err := svc.GetEligibility(context.Background(), c.WorkspaceID, c.ID, "message"); !errors.As(err, &classErr) {
		t.Fatalf("message: err = %v", err)
	}
}

func TestClassReview_WindowFollowsStoredSettingsAndCountsPerClass(t *testing.T) {
	store, c, svc := realLogFixture(t)
	mustExec(t, store, `DELETE FROM coordinator_class_reviews`)
	mustExec(t, store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`,
		`{"version":1,"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"},"eligibility":{"evidence_window_hours":48,"min_decisions":3,"unedited_floor_percent":90,"review_max_age_hours":168,"daily_limit":10}}`, c.ID)
	seedClassRow(t, store, c, ActionResume, "in", ActivityApproved, AuthRequiresApproval, false, automaticNow.Add(-47*time.Hour))
	seedClassRow(t, store, c, ActionResume, "out", ActivityApproved, AuthRequiresApproval, false, automaticNow.Add(-49*time.Hour))
	seedClassRow(t, store, c, ActionStartAgent, "sa", ActivityRejected, AuthRequiresApproval, false, automaticNow.Add(-time.Hour))
	review, err := svc.RecordClassReview(authedContext("mgr-1"), c.WorkspaceID, c.ID, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if !review.WindowStart.Equal(automaticNow.Add(-48*time.Hour)) || review.RowCount != 1 || review.Class != ActionResume {
		t.Fatalf("review = %+v", review)
	}
	sa, err := svc.RecordClassReview(authedContext("mgr-1"), c.WorkspaceID, c.ID, "start_agent")
	if err != nil || sa.RowCount != 1 {
		t.Fatalf("start_agent review = %+v err = %v", sa, err)
	}
	again, err := svc.RecordClassReview(authedContext("mgr-1"), c.WorkspaceID, c.ID, "resume")
	if err != nil || again.ID == review.ID || countReviews(t, store, c) != 3 {
		t.Fatalf("repeat review did not append: %+v err = %v", again, err)
	}
}

func TestDecidedRows_StartAgentJoinIgnoresRowsWithoutProposal(t *testing.T) {
	store, c, _ := realLogFixture(t)
	seedActivity(t, store, c.ID, "plain", ActionCreateTask, ActivityApproved, automaticNow.Add(-time.Hour))
	n, err := store.CountDecidedRowsTx(context.Background(), store.db, c.ID, ActionStartAgent, automaticNow.Add(-24*time.Hour), automaticNow)
	if err != nil || n != 0 {
		t.Fatalf("count = %d err = %v, want 0 for a row with no starts_agent proposal", n, err)
	}
	if strings.Contains(decidedFrom(ActionResume), "JOIN") {
		t.Fatal("resume must not join proposals")
	}
}
