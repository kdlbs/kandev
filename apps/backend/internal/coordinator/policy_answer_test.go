package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestActionAnswerIsAPolicyActionButNotRaisable(t *testing.T) {
	if !isPolicyAction(ActionAnswer) {
		t.Fatal("answer must be a policy action")
	}
	if raisableClasses[ActionAnswer] {
		t.Fatal("answer must never be raisable")
	}
	if len(AllActions) != 7 {
		t.Fatalf("AllActions = %d, want 7", len(AllActions))
	}
}

func TestDeniedActionsIncludeAnswer(t *testing.T) {
	m := deniedActions()
	if len(m) != 7 || m[ActionAnswer] != SettingDenied {
		t.Fatalf("deniedActions = %v", m)
	}
}

func TestPhaseOnePolicyLeavesAnswerAbsent(t *testing.T) {
	if _, ok := PhaseOnePolicy().Actions[ActionAnswer]; ok {
		t.Fatal("phase-one policy must not contain answer")
	}
	p, err := ParsePolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Actions[ActionAnswer]; ok {
		t.Fatal("NULL policy must leave answer absent")
	}
}

func TestParsePolicyAnswerEntry(t *testing.T) {
	base := `"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"`
	cases := []struct {
		name   string
		answer string
		want   Setting
		absent bool
	}{
		{name: "absent", answer: "", absent: true},
		{name: "denied", answer: `,"answer":"denied"`, want: SettingDenied},
		{name: "requires_approval", answer: `,"answer":"requires_approval"`, want: SettingRequiresApproval},
		{name: "automatic parses to denied", answer: `,"answer":"automatic"`, want: SettingDenied},
		{name: "unusable parses to denied", answer: `,"answer":"bogus"`, want: SettingDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"version":1,"actions":{` + base + tc.answer + `}}`
			p, err := ParsePolicy(strPtr(raw))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := p.Actions[ActionAnswer]
			if tc.absent {
				if ok {
					t.Fatalf("answer = %q, want absent", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("answer = %q (present %v), want %q", got, ok, tc.want)
			}
		})
	}
}

func TestUnreadablePolicyDeniesAllSevenClasses(t *testing.T) {
	p, err := ParsePolicy(strPtr("{"))
	if !errors.Is(err, ErrPolicyUnreadable) {
		t.Fatalf("err = %v", err)
	}
	if len(p.Actions) != 7 || p.Actions[ActionAnswer] != SettingDenied {
		t.Fatalf("actions = %v", p.Actions)
	}
}

func TestValidateRefusesAutomaticAnswer(t *testing.T) {
	p := PhaseOnePolicy()
	p.Actions[ActionAnswer] = SettingAutomatic
	var fe *PolicyFieldError
	if err := Validate(p, true); !errors.As(err, &fe) || fe.Field != "answer" || fe.Code != "automatic_not_available" {
		t.Fatalf("err = %v", err)
	}
	p.Actions[ActionAnswer] = SettingRequiresApproval
	if err := Validate(p, true); err != nil {
		t.Fatalf("requires_approval refused: %v", err)
	}
	delete(p.Actions, ActionAnswer)
	if err := Validate(p, true); err != nil {
		t.Fatalf("absent answer refused: %v", err)
	}
}

func TestAllowsAbsentAnswerIsFalse(t *testing.T) {
	if PhaseOnePolicy().Allows(ActionAnswer) {
		t.Fatal("an absent answer entry must not allow")
	}
}

func answerService(t *testing.T, store *Store, on bool) *Service {
	t.Helper()
	svc := newPhase2Service(t, store, true)
	svc.answer = on
	return svc
}

func TestPolicyViewAnswerDefaultsAndFlag(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	off := answerService(t, store, false)
	v, err := off.Policy(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.Actions[ActionAnswer]; ok {
		t.Fatalf("flag off: view names answer: %v", v.Actions)
	}
	if got := off.policyFor(c).Actions[ActionAnswer]; got != SettingDenied {
		t.Fatalf("flag off effective answer = %q, want denied", got)
	}

	on := answerService(t, store, true)
	v, err = on.Policy(ctx, c.ID)
	if err != nil || v.Actions[ActionAnswer] != SettingRequiresApproval {
		t.Fatalf("flag on: absent answer = %q, %v; want requires_approval", v.Actions[ActionAnswer], err)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET policy_json = ? WHERE id = ?`,
		`{"version":1,"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied","answer":"denied"}}`, c.ID); err != nil {
		t.Fatal(err)
	}
	c, _ = store.GetCoordinatorByID(ctx, c.ID)
	if got := on.policyFor(c).Actions[ActionAnswer]; got != SettingDenied {
		t.Fatalf("stored denied = %q, want denied", got)
	}
	if on.policyFor(c).Allows(ActionAnswer) {
		t.Fatal("a stored denied answer must not allow")
	}
}

func TestPolicyForUnreadableDeniesAnswerEvenWithFlagOn(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET policy_json = 'not json' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	c, _ = store.GetCoordinatorByID(ctx, c.ID)
	svc := answerService(t, store, true)
	p := svc.policyFor(c)
	if len(p.Actions) != 7 {
		t.Fatalf("actions = %v", p.Actions)
	}
	for a, v := range p.Actions {
		if v != SettingDenied {
			t.Fatalf("%s = %q, want denied", a, v)
		}
	}
}

func TestSettingsSaveAnswerClass(t *testing.T) {
	store := newTestStore(t)
	ctx := authedContext("mgr-1")
	c := newTestCoordinator(t, store, "ws-1")
	svc := answerService(t, store, true)
	svc.phase3 = true

	body := func(answer string) []byte {
		actions := `"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"`
		if answer != "" {
			actions += `,"answer":"` + answer + `"`
		}
		return []byte(`{"policy":{"actions":{` + actions + `}}}`)
	}
	save := func(b []byte) (*CoordinatorPhase2, error) { return svc.SaveSettings(ctx, c.WorkspaceID, c.ID, b) }
	storedRaw := func() string {
		got, _ := store.GetCoordinatorByID(context.Background(), c.ID)
		if got.PolicyJSON == nil {
			return ""
		}
		return *got.PolicyJSON
	}

	// A first save that omits answer on a pre-feature coordinator changes nothing
	// about answer and leaves it absent.
	if _, err := save(body("")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedRaw(), `"answer"`) {
		t.Fatalf("an omitted answer was stored: %s", storedRaw())
	}

	// Naming requires_approval for an absent entry is not a change.
	before, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if _, err := save(body("requires_approval")); err != nil {
		t.Fatal(err)
	}
	after, _ := store.GetCoordinatorByID(context.Background(), c.ID)
	if after.PolicyRevision != before.PolicyRevision {
		t.Fatalf("revision %d -> %d: requires_approval over an absent entry is no change", before.PolicyRevision, after.PolicyRevision)
	}

	// automatic is refused with the same code as every non-raisable class.
	_, err := save(body("automatic"))
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != "automatic_not_available" || se.Field != "policy.actions.answer" {
		t.Fatalf("automatic: err = %v", err)
	}

	// An explicit denied is stored, shown, and survives a save that omits answer.
	if _, err := save(body("denied")); err != nil {
		t.Fatal(err)
	}
	got, err := save(body(""))
	if err != nil {
		t.Fatal(err)
	}
	if got.Policy.Actions[ActionAnswer] != SettingDenied || !strings.Contains(storedRaw(), `"answer":"denied"`) {
		t.Fatalf("stored denied was dropped: response %v stored %s", got.Policy.Actions[ActionAnswer], storedRaw())
	}
}

func TestSettingsSaveAnswerUnavailableWhileFlagOff(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	svc := answerService(t, store, false)
	svc.phase3 = true
	for _, value := range []string{"requires_approval", "denied", "automatic"} {
		body := `{"policy":{"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied","answer":"` + value + `"}}}`
		_, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID, []byte(body))
		var se *SettingsError
		if !errors.As(err, &se) || se.Code != "answer_unavailable" {
			t.Fatalf("%s: err = %v, want answer_unavailable", value, err)
		}
	}
	// The save response of a body that omits answer never names it.
	got, err := svc.SaveSettings(authedContext("mgr-1"), c.WorkspaceID, c.ID,
		[]byte(`{"policy":{"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Policy.Actions[ActionAnswer]; ok {
		t.Fatalf("flag off: save response names answer: %v", got.Policy.Actions)
	}
}

func TestClassChangeRecordsAbsentAnswerAsRequiresApproval(t *testing.T) {
	stored := PhaseOnePolicy()
	final := PhaseOnePolicy()
	final.Actions[ActionAnswer] = SettingDenied
	if actionsEqual(stored, final) {
		t.Fatal("absent -> denied must differ")
	}
	final.Actions[ActionAnswer] = SettingRequiresApproval
	if !actionsEqual(stored, final) {
		t.Fatal("absent equals requires_approval")
	}
	if effectiveSetting(stored, ActionAnswer) != SettingRequiresApproval {
		t.Fatal("an absent answer reads as requires_approval")
	}
}
