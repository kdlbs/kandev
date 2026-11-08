package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProposeAnswerClarificationStoresSnapshotAndRecords(t *testing.T) {
	fx := newAnswerFx(t)
	before := fx.undo.tasks["task-0"].WorkflowStepID
	p := fx.mustPropose(t, clarificationArgs)
	if p.Status != ProposalStatusPending || p.Kind != ProposalKindAnswer || p.TargetTaskID == nil || *p.TargetTaskID != "task-0" {
		t.Fatalf("proposal = %+v", p)
	}
	s := specOf(t, p)
	if s.Target != "clarification" || s.PendingID != "p1" || s.RequestID != "" || len(s.Snapshot.Questions) != 2 ||
		s.Snapshot.Questions[0].Text != "Prompt q1" || len(s.Snapshot.Questions[0].Options) != 2 {
		t.Fatalf("spec = %+v", s)
	}
	if got := s.Draft.Answers[1].CustomText; got != "why" {
		t.Fatalf("custom text %q, want trimmed", got)
	}
	if s.Sources == nil {
		t.Fatal("sources must default to an empty list")
	}
	rows := listActivity(t, fx.store, fx.c.ID)
	if len(rows) != 1 || rows[0].ActionClass != ActionAnswer || rows[0].Outcome != ActivityProposed {
		t.Fatalf("activity = %+v", rows)
	}
	if fx.undo.tasks["task-0"].WorkflowStepID != before || fx.delivery.resolveCount() != 0 {
		t.Fatal("propose must change nothing")
	}
}

func TestProposeAnswerPermissionStoresRequestID(t *testing.T) {
	fx := newAnswerFx(t)
	fx.targets.setSet(permissionSet("pp1", "r1"))
	p := fx.mustPropose(t, permissionArgs)
	s := specOf(t, p)
	if s.RequestID != "r1" || s.Snapshot.Permission == nil || s.Snapshot.Permission.Title != "Run ls" ||
		len(s.Snapshot.Permission.Options) != 3 || s.Draft.OptionID != "allow" || len(s.Sources) != 1 || !s.NeedsDecision {
		t.Fatalf("spec = %+v", s)
	}
}

func TestProposeAnswerFlagOffRefuses(t *testing.T) {
	fx := newAnswerFx(t)
	fx.svc.answer = false
	_, _, err := fx.proposeAnswer(clarificationArgs)
	var denied *PolicyDeniedError
	if !errors.As(err, &denied) || denied.Action != ActionAnswer {
		t.Fatalf("err = %v", err)
	}
}

func TestProposeAnswerStage1FieldErrors(t *testing.T) {
	const draft = `"draft":{"answers":[{"question_id":"q1","option_ids":["a"]},{"question_id":"q2","option_ids":["b"]}]}`
	cases := []struct {
		name, args, field, message string
	}{
		{"unknown top level, alphabetical first", `{"zeta":1,"alpha":2,"task_id":"task-0"}`, "alpha", "unknown_field"},
		{"unknown beats earlier invalid member", `{"task_id":5,"extra":1}`, "extra", "unknown_field"},
		{"task_id wrong type", `{"task_id":5}`, "task_id", ""},
		{"task_id missing", `{"target":"clarification"}`, "task_id", ""},
		{"target invalid", `{"task_id":"task-0","target":"both"}`, "target", ""},
		{"pending_id null", `{"task_id":"task-0","target":"clarification","pending_id":null}`, "pending_id", ""},
		{"draft missing", `{"task_id":"task-0","target":"clarification","pending_id":"p1"}`, "draft", ""},
		{"draft option_id for clarification", `{"task_id":"task-0","target":"clarification","pending_id":"p1","draft":{"option_id":"a"}}`, "draft.option_id", "unknown_field"},
		{"draft answers for permission", `{"task_id":"task-0","target":"permission","pending_id":"p1","draft":{"answers":[]}}`, "draft.answers", "unknown_field"},
		{"entry unknown member", `{"task_id":"task-0","target":"clarification","pending_id":"p1","draft":{"answers":[{"question_id":"q1","x":1}]}}`, "draft.answers[0].x", "unknown_field"},
		{"entry empty answer", `{"task_id":"task-0","target":"clarification","pending_id":"p1","draft":{"answers":[{"question_id":"q1","custom_text":"  "}]}}`, "draft", "empty_answer"},
		{"entry option ids wrong type", `{"task_id":"task-0","target":"clarification","pending_id":"p1","draft":{"answers":[{"question_id":"q1","option_ids":"a"}]}}`, "draft", ""},
		{"sources unknown member", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"sources":[{"ref":"x","bad":1}]}`, "sources[0].bad", "unknown_field"},
		{"sources empty ref", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"sources":[{"ref":" "}]}`, "sources", ""},
		{"sources too many", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"sources":[` + strings.Repeat(`{"ref":"x"},`, 10) + `{"ref":"x"}]}`, "sources", ""},
		{"confidence missing", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `}`, "confidence", ""},
		{"confidence invalid", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"confidence":"certain"}`, "confidence", ""},
		{"needs_decision string", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"confidence":"low","needs_decision":"yes"}`, "needs_decision", ""},
		{"rationale too long", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"confidence":"low","needs_decision":false,"rationale":"` + strings.Repeat("a", 10001) + `"}`, "rationale", ""},
		{"order ids not a list", `{"task_id":"task-0","target":"clarification","pending_id":"p1",` + draft + `,"confidence":"low","needs_decision":false,"standing_order_ids":"o1"}`, "standing_order_ids", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newAnswerFx(t)
			_, _, err := fx.proposeAnswer(tc.args)
			var fe *FieldError
			if !errors.As(err, &fe) || fe.Field != tc.field || (tc.message != "" && fe.Message != tc.message) {
				t.Fatalf("err = %v, want field %q message %q", err, tc.field, tc.message)
			}
			if fx.targets.calls != 0 {
				t.Fatal("stage 1 must not read the target")
			}
			if openAnswerCount(t, fx) != 0 {
				t.Fatal("nothing may be stored")
			}
		})
	}
}

func TestProposeAnswerStage2Refusals(t *testing.T) {
	t.Run("pending id differs", func(t *testing.T) {
		fx := newAnswerFx(t)
		_, _, err := fx.proposeAnswer(strings.Replace(clarificationArgs, `"p1"`, `"p9"`, 1))
		assertRefusal(t, err, refusalNothingToAnswer, "pending_id")
	})
	t.Run("target absent", func(t *testing.T) {
		fx := newAnswerFx(t)
		fx.targets.setSet(TargetSet{})
		_, _, err := fx.proposeAnswer(clarificationArgs)
		assertRefusal(t, err, refusalNothingToAnswer, "pending_id")
	})
	t.Run("other kind present only", func(t *testing.T) {
		fx := newAnswerFx(t)
		fx.targets.setSet(permissionSet("pp1", "r1"))
		_, _, err := fx.proposeAnswer(clarificationArgs)
		assertRefusal(t, err, refusalNothingToAnswer, "pending_id")
	})
	t.Run("read failure", func(t *testing.T) {
		fx := newAnswerFx(t)
		fx.targets.err = ErrAnswerTargetRead
		_, _, err := fx.proposeAnswer(clarificationArgs)
		if !errors.Is(err, ErrAnswerTargetRead) || openAnswerCount(t, fx) != 0 {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("task vanished", func(t *testing.T) {
		fx := newAnswerFx(t)
		fx.targets.err = ErrNotFound
		_, _, err := fx.proposeAnswer(clarificationArgs)
		wantField(t, err, "task_id")
	})
}

func assertRefusal(t *testing.T, err error, code, field string) {
	t.Helper()
	var r *ProposeRefusal
	if !errors.As(err, &r) || r.Code != code || r.Field != field {
		t.Fatalf("err = %v, want refusal %s on %s", err, code, field)
	}
}

func TestProposeAnswerDraftContent(t *testing.T) {
	cases := []struct{ name, draft string }{
		{"missing question", `{"answers":[{"question_id":"q1","option_ids":["a"]}]}`},
		{"out of order", `{"answers":[{"question_id":"q2","option_ids":["a"]},{"question_id":"q1","option_ids":["a"]}]}`},
		{"repeated question", `{"answers":[{"question_id":"q1","option_ids":["a"]},{"question_id":"q1","option_ids":["a"]}]}`},
		{"unknown question", `{"answers":[{"question_id":"q1","option_ids":["a"]},{"question_id":"zz","option_ids":["a"]}]}`},
		{"unknown option", `{"answers":[{"question_id":"q1","option_ids":["a"]},{"question_id":"q2","option_ids":["nope"]}]}`},
		{"no answers", `{"answers":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newAnswerFx(t)
			args := `{"task_id":"task-0","target":"clarification","pending_id":"p1","confidence":"low","needs_decision":false,"draft":` + tc.draft + `}`
			_, _, err := fx.proposeAnswer(args)
			wantField(t, err, "draft")
		})
	}
	for _, option := range []string{"always", "missing", ""} {
		t.Run("permission "+option, func(t *testing.T) {
			fx := newAnswerFx(t)
			fx.targets.setSet(permissionSet("pp1", "r1"))
			args := `{"task_id":"task-0","target":"permission","pending_id":"pp1","confidence":"low","needs_decision":false,"draft":{"option_id":"` + option + `"}}`
			_, _, err := fx.proposeAnswer(args)
			wantField(t, err, "draft")
		})
	}
	t.Run("permission option of unknown kind", func(t *testing.T) {
		fx := newAnswerFx(t)
		set := permissionSet("pp1", "r1")
		set.Permission.Options = append(set.Permission.Options, PermissionOptionTarget{ID: "weird", Name: "W"})
		fx.targets.setSet(set)
		args := `{"task_id":"task-0","target":"permission","pending_id":"pp1","confidence":"low","needs_decision":false,"draft":{"option_id":"weird"}}`
		_, _, err := fx.proposeAnswer(args)
		wantField(t, err, "draft")
	})
}

func TestProposeAnswerTargetChecksShared(t *testing.T) {
	fx := newAnswerFx(t)
	fx.tasks.target.WorkspaceID = "ws-2"
	_, _, err := fx.proposeAnswer(clarificationArgs)
	wantField(t, err, "task_id")
	if fx.targets.calls != 0 {
		t.Fatal("a foreign task must be refused before the target read")
	}
}

func TestProposeAnswerDedupesEqualCall(t *testing.T) {
	fx := newAnswerFx(t)
	first := fx.mustPropose(t, clarificationArgs)
	reads := fx.targets.calls
	changed := strings.Replace(clarificationArgs, `"option_ids":["a"]`, `"option_ids":["b"]`, 1)
	again, deduped, err := fx.proposeAnswer(changed)
	if err != nil || !deduped || again.ID != first.ID {
		t.Fatalf("again=%+v deduped=%v err=%v", again, deduped, err)
	}
	if fx.targets.calls != reads {
		t.Fatal("an equal call is answered before stage 2")
	}
	if got := specOf(t, fx.reload(t, first.ID)).Draft.Answers[0].OptionIDs[0]; got != "a" {
		t.Fatalf("draft replaced: %q", got)
	}
}

func TestProposeAnswerSupersedesStaleOpenProposal(t *testing.T) {
	fx := newAnswerFx(t)
	first := fx.mustPropose(t, clarificationArgs)
	fx.ageProposal(t, first.ID)
	fx.targets.setSet(clarificationSet("p2", yesNo("q1")))
	args := `{"task_id":"task-0","target":"clarification","pending_id":"p2","confidence":"low","needs_decision":false,"draft":{"answers":[{"question_id":"q1","option_ids":["a"]}]}}`
	second, deduped, err := fx.proposeAnswer(args)
	if err != nil || deduped || second.ID == first.ID {
		t.Fatalf("second=%+v deduped=%v err=%v", second, deduped, err)
	}
	old := fx.reload(t, first.ID)
	if old.Status != ProposalStatusRejected || old.Error == nil || *old.Error != "stale" || old.DecidedBy != nil || !isRefusedAnswer(old) {
		t.Fatalf("old = %+v", old)
	}
	if openAnswerCount(t, fx) != 1 {
		t.Fatal("exactly one answer proposal must be open")
	}
	var refused int
	for _, r := range listActivity(t, fx.store, fx.c.ID) {
		if r.Outcome == ActivityRefused && r.ProposalID != nil && *r.ProposalID == first.ID && r.ReasonCode != nil && *r.ReasonCode == "stale" {
			refused++
		}
	}
	if refused != 1 {
		t.Fatalf("refused rows = %d, want 1", refused)
	}
}

func TestProposeAnswerPendingWhileOpenTargetIsCurrent(t *testing.T) {
	fx := newAnswerFx(t)
	first := fx.mustPropose(t, clarificationArgs)
	fx.ageProposal(t, first.ID)
	// The clarification is still current; a permission call is another target.
	set := clarificationSet("p1", yesNo("q1"), yesNo("q2"))
	set.Permission = permissionSet("pp1", "r1").Permission
	fx.targets.setSet(set)
	_, _, err := fx.proposeAnswer(permissionArgs)
	assertRefusal(t, err, refusalAnswerPending, "task_id")
	if fx.reload(t, first.ID).Status != ProposalStatusPending {
		t.Fatal("the open proposal must stay pending")
	}
}

func TestProposeAnswerRefusalsForApprovingAndOutcomeUnknown(t *testing.T) {
	fx := newAnswerFx(t)
	first := fx.mustPropose(t, clarificationArgs)
	fx.ageProposal(t, first.ID)
	fx.targets.setSet(permissionSet("pp1", "r1"))

	fx.forceApproving(t, first, time.Now())
	_, _, err := fx.proposeAnswer(permissionArgs)
	assertRefusal(t, err, refusalApprovalInProgress, "task_id")

	if _, e := fx.store.db.Exec(`UPDATE coordinator_proposals SET status='failed', error='outcome_unknown', claim_token=NULL WHERE id=?`, first.ID); e != nil {
		t.Fatal(e)
	}
	_, _, err = fx.proposeAnswer(permissionArgs)
	assertRefusal(t, err, refusalAnswerPending, "task_id")
}

// A competing call commits its row between this call's target read and its
// store step.
func racingPropose(t *testing.T, fx *answerFx, competing string) {
	t.Helper()
	fx.targets.onRead = func() {
		fx.targets.onRead = nil
		fx.mustPropose(t, competing)
	}
}

func bothTargets() TargetSet {
	set := clarificationSet("p1", yesNo("q1"), yesNo("q2"))
	set.Permission = permissionSet("pp1", "r1").Permission
	return set
}

func TestProposeAnswerRacingEqualCallDedupes(t *testing.T) {
	fx := newAnswerFx(t)
	fx.targets.setSet(bothTargets())
	racingPropose(t, fx, clarificationArgs)
	got, deduped, err := fx.proposeAnswer(clarificationArgs)
	if err != nil || !deduped || got == nil {
		t.Fatalf("got=%+v deduped=%v err=%v", got, deduped, err)
	}
	if openAnswerCount(t, fx) != 1 {
		t.Fatalf("open answers = %d, want 1", openAnswerCount(t, fx))
	}
}

func TestProposeAnswerRacingDifferentCallIsPendingNotSuperseded(t *testing.T) {
	fx := newAnswerFx(t)
	fx.targets.setSet(bothTargets())
	racingPropose(t, fx, permissionArgs)
	_, _, err := fx.proposeAnswer(clarificationArgs)
	assertRefusal(t, err, refusalAnswerPending, "task_id")
	if openAnswerCount(t, fx) != 1 {
		t.Fatalf("open answers = %d, want 1", openAnswerCount(t, fx))
	}
	for _, p := range mustListAll(t, fx) {
		if isRefusedAnswer(p) {
			t.Fatal("the racing row must not be refused")
		}
	}
}

func mustListAll(t *testing.T, fx *answerFx) []*Proposal {
	t.Helper()
	list, err := fx.store.ListProposals(context.Background(), fx.c.WorkspaceID, fx.c.ID, ListProposalsAll, true)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestProposeAnswerBadOrderRollsBackSupersede(t *testing.T) {
	fx := newAnswerFx(t)
	first := fx.mustPropose(t, clarificationArgs)
	fx.ageProposal(t, first.ID)
	fx.targets.setSet(clarificationSet("p2", yesNo("q1")))
	args := `{"task_id":"task-0","target":"clarification","pending_id":"p2","confidence":"low","needs_decision":false,
"standing_order_ids":["missing"],"draft":{"answers":[{"question_id":"q1","option_ids":["a"]}]}}`
	_, _, err := fx.proposeAnswer(args)
	wantField(t, err, "standing_order_ids")
	if fx.reload(t, first.ID).Status != ProposalStatusPending {
		t.Fatal("a failed propose must not leave the old proposal refused")
	}
}

func TestProposeAnswerRefusedRowFreesTheOpenSlot(t *testing.T) {
	fx := newAnswerFx(t)
	first := fx.mustPropose(t, clarificationArgs)
	fx.ageProposal(t, first.ID)
	fx.targets.setSet(clarificationSet("p2", yesNo("q1")))
	args := `{"task_id":"task-0","target":"clarification","pending_id":"p2","confidence":"low","needs_decision":false,"draft":{"answers":[{"question_id":"q1","option_ids":["a"]}]}}`
	if _, _, err := fx.proposeAnswer(args); err != nil {
		t.Fatal(err)
	}
	open, err := fx.store.OpenTargetProposal(context.Background(), fx.store.ro, fx.c.ID, ProposalKindAnswer, "task-0")
	if err != nil || open == nil || open.ID == first.ID {
		t.Fatalf("open = %+v err=%v", open, err)
	}
}
