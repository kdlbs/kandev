package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const defaultsEligibilityJSON = `{"evidence_window_hours":720,"min_decisions":20,"unedited_floor_percent":90,"review_max_age_hours":168,"daily_limit":10}`

// withEligibility splices an eligibility member into a policyBody.
func withEligibility(body, member string) string {
	return strings.TrimSuffix(body, `}}}`) + `},"eligibility":` + member + `}}`
}

func eligibilityBody(overrides map[string]string, member string) string {
	return withEligibility(policyBody(overrides), member)
}

func eligibilityChangeCount(t *testing.T, store *Store, c *Coordinator) int {
	t.Helper()
	var n int
	if err := store.db.GetContext(context.Background(), &n, store.db.Rebind(`SELECT COUNT(*) FROM coordinator_eligibility_changes WHERE coordinator_id = ?`), c.ID); err != nil {
		t.Fatal(err)
	}
	return n
}

func classChangeCount(t *testing.T, store *Store, c *Coordinator) int {
	t.Helper()
	var n int
	if err := store.db.GetContext(context.Background(), &n, store.db.Rebind(`SELECT COUNT(*) FROM coordinator_class_changes WHERE coordinator_id = ?`), c.ID); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSaveSettings_EligibilityRefusals(t *testing.T) {
	cases := map[string]struct {
		body  string
		field string
	}{
		"below minimum":      {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"evidence_window_hours":720`, `"evidence_window_hours":23`, 1)), "policy.eligibility.evidence_window_hours"},
		"above maximum":      {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"daily_limit":10`, `"daily_limit":101`, 1)), "policy.eligibility.daily_limit"},
		"floor below 50":     {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"unedited_floor_percent":90`, `"unedited_floor_percent":49`, 1)), "policy.eligibility.unedited_floor_percent"},
		"fraction":           {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"min_decisions":20`, `"min_decisions":20.5`, 1)), "policy.eligibility.min_decisions"},
		"integer-valued":     {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"min_decisions":20`, `"min_decisions":20.0`, 1)), "policy.eligibility.min_decisions"},
		"exponent":           {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"min_decisions":20`, `"min_decisions":2e1`, 1)), "policy.eligibility.min_decisions"},
		"string value":       {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `"min_decisions":20`, `"min_decisions":"20"`, 1)), "policy.eligibility.min_decisions"},
		"unknown field":      {eligibilityBody(nil, strings.Replace(defaultsEligibilityJSON, `}`, `,"zeta":1,"alpha":1}`, 1)), "policy.eligibility.alpha"},
		"missing field":      {eligibilityBody(nil, `{"evidence_window_hours":720}`), "policy.eligibility.min_decisions"},
		"not an object":      {eligibilityBody(nil, `[1]`), "policy.eligibility"},
		"unknown beats low":  {eligibilityBody(nil, `{"evidence_window_hours":1,"bogus":1}`), "policy.eligibility.bogus"},
		"missing beats low":  {eligibilityBody(nil, `{"evidence_window_hours":1}`), "policy.eligibility.min_decisions"},
		"integer beats low":  {eligibilityBody(nil, `{"evidence_window_hours":1,"min_decisions":"x","unedited_floor_percent":90,"review_max_age_hours":168,"daily_limit":10}`), "policy.eligibility.min_decisions"},
		"actions come first": {eligibilityBody(map[string]string{"message": "bogus"}, `{"evidence_window_hours":1}`), "policy.actions.message"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store, c, svc := realLogFixture(t)
			rev := revisionOf(t, store, c.ID)
			_, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(tc.body))
			var se *SettingsError
			if !errors.As(err, &se) || se.Field != tc.field {
				t.Fatalf("err = %v, want field %s", err, tc.field)
			}
			if strings.HasPrefix(tc.field, "policy.eligibility") && se.Code != codeInvalidSetting {
				t.Fatalf("code = %q, want invalid_setting", se.Code)
			}
			if revisionOf(t, store, c.ID) != rev || eligibilityChangeCount(t, store, c) != 0 {
				t.Fatal("a refused save stored something")
			}
		})
	}
}

func TestSaveSettings_EligibilityRefusedWithPhase3Off(t *testing.T) {
	store, c, svc := realLogFixture(t)
	svc.phase3 = false
	rev := revisionOf(t, store, c.ID)
	_, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(nil, defaultsEligibilityJSON)))
	var se *SettingsError
	if !errors.As(err, &se) || se.Field != "policy.eligibility" || se.Code != codeInvalidSetting {
		t.Fatalf("err = %v", err)
	}
	if revisionOf(t, store, c.ID) != rev {
		t.Fatal("revision moved")
	}
	// An action error still ranks first.
	_, err = svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(map[string]string{"message": "bogus"}, defaultsEligibilityJSON)))
	if !errors.As(err, &se) || se.Field != "policy.actions.message" {
		t.Fatalf("err = %v", err)
	}
	// Null and absent are accepted and the response still carries the object.
	res, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(withEligibility(policyBody(nil), `null`)))
	if err != nil || res.Policy.Eligibility != DefaultEligibility() {
		t.Fatalf("res = %+v err = %v", res, err)
	}
}

func TestSaveSettings_EligibilityWriteAuditAndConversation(t *testing.T) {
	store, c, svc := realLogFixture(t)
	rev := revisionOf(t, store, c.ID)
	tuned := `{"evidence_window_hours":48,"min_decisions":5,"unedited_floor_percent":80,"review_max_age_hours":24,"daily_limit":3}`
	res, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(nil, tuned)))
	if err != nil {
		t.Fatal(err)
	}
	want := EligibilitySettings{EvidenceWindowHours: 48, MinDecisions: 5, UneditedFloorPct: 80, ReviewMaxAgeHours: 24, DailyLimit: 3}
	if res.Policy.Eligibility != want || res.PolicyRevision != rev+1 || revisionOf(t, store, c.ID) != rev+1 {
		t.Fatalf("res = %+v", res)
	}
	if eligibilityChangeCount(t, store, c) != 1 || classChangeCount(t, store, c) != 0 {
		t.Fatalf("audit rows = %d, class rows = %d", eligibilityChangeCount(t, store, c), classChangeCount(t, store, c))
	}
	var by, from, to string
	var revision int
	if err := store.db.QueryRowContext(context.Background(), store.db.Rebind(`SELECT changed_by, from_json, to_json, policy_revision FROM coordinator_eligibility_changes WHERE coordinator_id = ?`), c.ID).
		Scan(&by, &from, &to, &revision); err != nil {
		t.Fatal(err)
	}
	if by != "mgr-1" || revision != rev+1 || !strings.Contains(from, `"daily_limit":10`) || !strings.Contains(to, `"daily_limit":3`) {
		t.Fatalf("audit = %s %s %s %d", by, from, to, revision)
	}
}

func TestSaveSettings_EligibilityEqualAndAbsentBumpNothing(t *testing.T) {
	store, c, svc := realLogFixture(t)
	ctx := authedContext("mgr-1")
	rev := revisionOf(t, store, c.ID)
	// Defaults over an absent object: nothing changes.
	if _, err := svc.SaveSettings(ctx, c.WorkspaceID, c.ID, []byte(eligibilityBody(nil, defaultsEligibilityJSON))); err != nil {
		t.Fatal(err)
	}
	if revisionOf(t, store, c.ID) != rev || eligibilityChangeCount(t, store, c) != 0 {
		t.Fatal("explicit defaults over absent changed something")
	}
	tuned := `{"evidence_window_hours":48,"min_decisions":5,"unedited_floor_percent":80,"review_max_age_hours":24,"daily_limit":3}`
	if _, err := svc.SaveSettings(ctx, c.WorkspaceID, c.ID, []byte(eligibilityBody(nil, tuned))); err != nil {
		t.Fatal(err)
	}
	rev = revisionOf(t, store, c.ID)
	// Same object again, an actions-only body, and a null member: no bump, no row, settings kept.
	for name, body := range map[string]string{
		"same object":  eligibilityBody(nil, tuned),
		"no member":    policyBody(nil),
		"null member":  withEligibility(policyBody(nil), `null`),
		"empty object": `{}`,
	} {
		res, err := svc.SaveSettings(ctx, c.WorkspaceID, c.ID, []byte(body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if revisionOf(t, store, c.ID) != rev || eligibilityChangeCount(t, store, c) != 1 || res.Policy.Eligibility.DailyLimit != 3 {
			t.Fatalf("%s: revision %d audit %d eligibility %+v", name, revisionOf(t, store, c.ID), eligibilityChangeCount(t, store, c), res.Policy.Eligibility)
		}
	}
	// An actions change preserves the tuned object.
	if _, err := svc.SaveSettings(ctx, c.WorkspaceID, c.ID, []byte(policyBody(map[string]string{"message": "requires_approval"}))); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetCoordinatorByID(context.Background(), c.ID)
	if err != nil || svc.policyFor(got).EffectiveEligibility().DailyLimit != 3 || eligibilityChangeCount(t, store, c) != 1 {
		t.Fatalf("eligibility lost on an actions save: %+v err = %v", svc.policyFor(got).EffectiveEligibility(), err)
	}
}

func TestSaveSettings_EligibilityOnlyKeepsConversation(t *testing.T) {
	store, c, svc := realLogFixture(t)
	before := conversationMarker(t, store, c)
	tuned := `{"evidence_window_hours":48,"min_decisions":5,"unedited_floor_percent":80,"review_max_age_hours":24,"daily_limit":3}`
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(nil, tuned))); err != nil {
		t.Fatal(err)
	}
	if conversationMarker(t, store, c) != before {
		t.Fatal("an eligibility-only save reset the conversation")
	}
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(map[string]string{"message": "requires_approval"}, tuned))); err != nil {
		t.Fatal(err)
	}
	if conversationMarker(t, store, c) == before {
		t.Fatal("an actions change did not reset the conversation")
	}
}

func TestRaise_UsesSettingsStoredBeforeTheSave(t *testing.T) {
	store, c, svc := realLogFixture(t)
	seedEarnedClass(t, store, c, ActionResume, 5)
	recordClassReviewFor(t, store, c, ActionResume, automaticNow.Add(-time.Hour))
	loosened := `{"evidence_window_hours":24,"min_decisions":3,"unedited_floor_percent":90,"review_max_age_hours":168,"daily_limit":10}`
	_, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(map[string]string{"resume": "automatic"}, loosened)))
	var refused *RaiseRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("one save loosened the gates and raised: %v", err)
	}
	if _, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(eligibilityBody(nil, loosened))); err != nil {
		t.Fatalf("loosen alone: %v", err)
	}
	got, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if svc.policyFor(got).Actions[ActionResume] == SettingAutomatic {
		t.Fatal("loosening raised a class")
	}
}

func TestLowerClass_PreservesEligibility(t *testing.T) {
	store, c, svc := realLogFixture(t)
	tuned := `{"evidence_window_hours":48,"min_decisions":5,"unedited_floor_percent":80,"review_max_age_hours":24,"daily_limit":3}`
	mustExec(t, store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`,
		`{"version":1,"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"automatic","stop":"denied"},"eligibility":`+tuned+`}`, c.ID)
	if _, err := svc.LowerClass(context.Background(), c.ID, ActionResume, "test", LowerGuard{}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if p := svc.policyFor(got); p.Actions[ActionResume] != SettingRequiresApproval || p.EffectiveEligibility().DailyLimit != 3 {
		t.Fatalf("policy = %+v", p)
	}
}

func TestParsePolicy_StoredEligibilityTakesPerFieldDefaults(t *testing.T) {
	stored := `{"version":1,"actions":{"create_task":"requires_approval"},"eligibility":{"evidence_window_hours":"x","min_decisions":5.5,"unedited_floor_percent":10,"review_max_age_hours":24,"daily_limit":null}}`
	p, err := ParsePolicy(&stored)
	if err != nil {
		t.Fatal(err)
	}
	want := EligibilitySettings{EvidenceWindowHours: 720, MinDecisions: 20, UneditedFloorPct: 90, ReviewMaxAgeHours: 24, DailyLimit: 10}
	if p.EffectiveEligibility() != want {
		t.Fatalf("eligibility = %+v", p.EffectiveEligibility())
	}
	notObject := `{"version":1,"actions":{},"eligibility":[1]}`
	p, err = ParsePolicy(&notObject)
	if err != nil || p.EffectiveEligibility() != DefaultEligibility() {
		t.Fatalf("non-object member: %+v err = %v", p, err)
	}
}

func TestValidate_EligibilityBounds(t *testing.T) {
	for _, f := range eligibilityFields {
		for _, n := range []int{f.min - 1, f.max + 1} {
			e := DefaultEligibility()
			*f.ptr(&e) = n
			p := PhaseOnePolicy()
			p.Eligibility = &e
			var fe *PolicyFieldError
			if err := Validate(p, true); !errors.As(err, &fe) || fe.Field != eligibilityPrefix+f.name {
				t.Fatalf("%s=%d: err = %v", f.name, n, err)
			}
		}
		for _, n := range []int{f.min, f.max} {
			e := DefaultEligibility()
			*f.ptr(&e) = n
			p := PhaseOnePolicy()
			p.Eligibility = &e
			if err := Validate(p, true); err != nil {
				t.Fatalf("%s=%d refused: %v", f.name, n, err)
			}
		}
	}
}

func conversationMarker(t *testing.T, store *Store, c *Coordinator) string {
	t.Helper()
	var marker string
	if err := store.db.GetContext(context.Background(), &marker, store.db.Rebind(`SELECT COALESCE(conversation_task_id, '') || '|' || CAST(config_revision AS TEXT) FROM coordinators WHERE id = ?`), c.ID); err != nil {
		t.Fatalf("conversation marker unavailable: %v", err)
	}
	return marker
}
