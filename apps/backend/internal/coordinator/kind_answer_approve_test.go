package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
)

func editBody(draft string) ApproveProposalRequest {
	return ApproveProposalRequest{"draft": json.RawMessage(draft)}
}

func (fx *answerFx) approveAnswer(p *Proposal, edits ApproveProposalRequest) (*Proposal, error) {
	return fx.approve(p, edits)
}

func TestApproveAnswerDeliversClarification(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	got, err := fx.approveAnswer(p, nil)
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(fx.delivery.outcomes) != 1 || fx.delivery.pendingIDs[0] != "p1" {
		t.Fatalf("delivery = %+v", fx.delivery.outcomes)
	}
	a := fx.delivery.outcomes[0].Answers
	if len(a) != 2 || a[0].QuestionID != "q1" || a[1].CustomText != "why" || a[1].SelectedOptions[0] != "b" {
		t.Fatalf("answers = %+v", a)
	}
	var out struct {
		SessionID string `json:"session_id"`
		PendingID string `json:"pending_id"`
		Answers   []struct {
			QuestionID string   `json:"question_id"`
			OptionIDs  []string `json:"option_ids"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(fx.outcomeOf(t, p.ID)), &out); err != nil || out.PendingID != "p1" || out.SessionID != "s1" || len(out.Answers) != 2 {
		t.Fatalf("outcome = %q err=%v", fx.outcomeOf(t, p.ID), err)
	}
	var approved *ActivityRow
	for _, r := range listActivity(t, fx.store, fx.c.ID) {
		if r.Outcome == ActivityApproved {
			r := r
			approved = &r
		}
	}
	if approved == nil || approved.ActionClass != ActionAnswer || approved.Edited || approved.Detail == "" || len([]rune(approved.Detail)) > 200 {
		t.Fatalf("approved row = %+v", approved)
	}
}

func TestApproveAnswerDeliversPermission(t *testing.T) {
	fx := newAnswerFx(t)
	fx.targets.setSet(permissionSet("pp1", "r1"))
	p := fx.mustPropose(t, permissionArgs)
	got, err := fx.approveAnswer(p, nil)
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	want := []string{"task-0", "s1", "r1", "pp1", "allow"}
	if len(fx.delivery.permCalls) != 1 || len(fx.delivery.permCalls[0]) != 5 {
		t.Fatalf("calls = %+v", fx.delivery.permCalls)
	}
	for i, v := range want {
		if fx.delivery.permCalls[0][i] != v {
			t.Fatalf("call = %v, want %v", fx.delivery.permCalls[0], want)
		}
	}
}

func TestApproveAnswerEdits(t *testing.T) {
	const edited = `{"answers":[{"question_id":"q1","option_ids":["b","b"]},{"question_id":"q2","custom_text":" mine "}]}`
	t.Run("edited draft is delivered and recorded as edited", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		got, err := fx.approveAnswer(p, editBody(edited))
		if err != nil || got.Status != ProposalStatusApproved {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		a := fx.delivery.outcomes[0].Answers
		if len(a[0].SelectedOptions) != 1 || a[0].SelectedOptions[0] != "b" || a[1].CustomText != "mine" || len(a[1].SelectedOptions) != 0 {
			t.Fatalf("answers = %+v", a)
		}
		edits := 0
		for _, r := range listActivity(t, fx.store, fx.c.ID) {
			if r.Outcome == ActivityApproved && r.Edited {
				edits++
			}
		}
		if edits != 1 {
			t.Fatal("the approved row must be edited")
		}
	})
	t.Run("equal draft is no edit", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		if _, err := fx.approveAnswer(p, editBody(`{"answers":[{"question_id":"q1","option_ids":["a"]},{"question_id":"q2","option_ids":["b","b"],"custom_text":"why"}]}`)); err != nil {
			t.Fatal(err)
		}
		for _, r := range listActivity(t, fx.store, fx.c.ID) {
			if r.Outcome == ActivityApproved && r.Edited {
				t.Fatal("an equal draft is not an edit")
			}
		}
	})
	for name, body := range map[string]ApproveProposalRequest{
		"target not editable":  {"target": json.RawMessage(`"permission"`)},
		"sources not editable": {"sources": json.RawMessage(`[]`)},
		"snapshot":             {"snapshot": json.RawMessage(`{}`)},
		"unknown member":       {"edits": json.RawMessage(`{}`)},
		"draft null":           {"draft": json.RawMessage(`null`)},
		"partial draft":        editBody(`{"answers":[{"question_id":"q1","option_ids":["a"]}]}`),
		"unknown option":       editBody(`{"answers":[{"question_id":"q1","option_ids":["zz"]},{"question_id":"q2","option_ids":["a"]}]}`),
		"empty answer":         editBody(`{"answers":[{"question_id":"q1"},{"question_id":"q2","option_ids":["a"]}]}`),
		"wrong member":         editBody(`{"option_id":"x"}`),
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			fx := newAnswerFx(t)
			p := fx.mustPropose(t, clarificationArgs)
			_, err := fx.approveAnswer(p, body)
			var fe *FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("err = %v, want a field error", err)
			}
			if fx.reload(t, p.ID).Status != ProposalStatusPending || fx.delivery.resolveCount() != 0 {
				t.Fatal("a refused edit must leave the row pending and deliver nothing")
			}
		})
	}
	t.Run("permission edit may not name a lasting option", func(t *testing.T) {
		fx := newAnswerFx(t)
		fx.targets.setSet(permissionSet("pp1", "r1"))
		p := fx.mustPropose(t, permissionArgs)
		_, err := fx.approveAnswer(p, editBody(`{"option_id":"always"}`))
		wantField(t, err, "draft")
	})
	t.Run("live read failure leaves the row unchanged", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		fx.targets.err = ErrAnswerTargetRead
		_, err := fx.approveAnswer(p, editBody(edited))
		if !errors.Is(err, ErrAnswerTargetRead) || fx.reload(t, p.ID).Status != ProposalStatusPending {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("edit against a changed target is left to Execute", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		fx.targets.setSet(clarificationSet("p9", yesNo("q1")))
		got, err := settledRefusal(fx.approveAnswer(p, editBody(`{"answers":[{"question_id":"qq","option_ids":["zz"]}]}`)))
		if err != nil || !isRefusedAnswer(got) || *got.Error != "stale" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

func TestApproveAnswerRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(fx *answerFx)
		code  string
	}{
		{"pending id changed", func(fx *answerFx) { fx.targets.setSet(clarificationSet("p9", yesNo("q1"), yesNo("q2"))) }, "stale"},
		{"target gone", func(fx *answerFx) { fx.targets.setSet(TargetSet{}) }, "stale"},
		{"task deleted between steps", func(fx *answerFx) { fx.targets.err = ErrNotFound }, "task_archived"},
		{"task archived", func(fx *answerFx) { now := time.Now(); fx.tasks.target.ArchivedAt = &now }, "task_archived"},
		{"task left workspace", func(fx *answerFx) { fx.tasks.target.WorkspaceID = "ws-2" }, "task_archived"},
		{"task unwatched", func(fx *answerFx) { fx.tasks.target.WorkflowID = "wf-x" }, "task_archived"},
		{"task is a conversation", func(fx *answerFx) { fx.tasks.target.Origin = "coordinator" }, "task_archived"},
		{"task not found", func(fx *answerFx) { fx.tasks.err = ErrTaskNotFound }, "task_archived"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newAnswerFx(t)
			mustSave(t, fx.svc, fx.c.WorkspaceID, fx.c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-1"]}}`)
			p := fx.mustPropose(t, clarificationArgs)
			tc.setup(fx)
			got, err := settledRefusal(fx.approveAnswer(p, nil))
			if err != nil || !isRefusedAnswer(got) || *got.Error != tc.code {
				t.Fatalf("got=%+v err=%v, want refused %s", got, err, tc.code)
			}
			if got.DecidedBy != nil || fx.delivery.resolveCount() != 0 {
				t.Fatalf("decided_by=%v deliveries=%d", got.DecidedBy, fx.delivery.resolveCount())
			}
			var rows []ActivityRow
			for _, r := range listActivity(t, fx.store, fx.c.ID) {
				if r.Outcome == ActivityRefused && r.ProposalID != nil {
					rows = append(rows, r)
				}
			}
			if len(rows) != 1 || rows[0].ReasonCode == nil || *rows[0].ReasonCode != tc.code || rows[0].RefusalCount > 1 {
				t.Fatalf("refusal rows = %+v", rows)
			}
			// A refused proposal is final.
			if _, err := fx.approveAnswer(got, nil); !isConflict(err) {
				t.Fatalf("second approve err = %v, want conflict", err)
			}
		})
	}
}

// settledRefusal reads the proposal an approve that settled a refusal reports:
// the refusal is the 409 conflict carrying the refused row.
func settledRefusal(p *Proposal, err error) (*Proposal, error) {
	var c *ProposalConflictError
	if errors.As(err, &c) && isRefusedAnswer(c.Proposal) {
		return c.Proposal, nil
	}
	return p, err
}

func isConflict(err error) bool {
	var c *ProposalConflictError
	return errors.As(err, &c)
}

func TestApproveAnswerFailureMapping(t *testing.T) {
	validation := func() error {
		return clarification.ValidateOutcome([]clarification.Question{{ID: "q"}}, clarification.Outcome{})
	}
	type result struct {
		status ProposalStatus
		code   string
	}
	clar := []struct {
		name  string
		won   bool
		err   error
		want  result
		retry bool
	}{
		{"lost with winner", false, nil, result{ProposalStatusRejected, "stale"}, false},
		{"not active", false, clarification.ErrNotActive, result{ProposalStatusRejected, "stale"}, false},
		{"validation", false, validation(), result{ProposalStatusFailed, "invalid_draft"}, true},
		{"pre-claim error", false, errors.New("timeout"), result{ProposalStatusFailed, "not_delivered"}, true},
		{"claim won then error", true, errors.New("attachment"), result{ProposalStatusFailed, "outcome_unknown"}, false},
	}
	for _, tc := range clar {
		t.Run("clarification "+tc.name, func(t *testing.T) {
			fx := newAnswerFx(t)
			p := fx.mustPropose(t, clarificationArgs)
			fx.delivery.won, fx.delivery.resolveErr = tc.won, tc.err
			got, err := settledRefusal(fx.approveAnswer(p, nil))
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want.status || got.Error == nil || *got.Error != tc.want.code {
				t.Fatalf("got status %s error %v, want %+v", got.Status, got.Error, tc.want)
			}
			if tc.want.status == ProposalStatusFailed && answerRetryable(*got.Error) != tc.retry {
				t.Fatalf("retryable(%s) wrong", *got.Error)
			}
		})
	}
	perm := []struct {
		name string
		err  error
		want result
	}{
		{"stale", ErrDeliveryStale, result{ProposalStatusRejected, "stale"}},
		{"option not offered", ErrDeliveryOptionNotOffered, result{ProposalStatusFailed, "option_not_offered"}},
		{"in progress", ErrDeliveryInProgress, result{ProposalStatusFailed, "resolution_in_progress"}},
		{"not delivered", ErrDeliveryNotDelivered, result{ProposalStatusFailed, "not_delivered"}},
		{"unclassified", errors.New("audit"), result{ProposalStatusFailed, "outcome_unknown"}},
	}
	for _, tc := range perm {
		t.Run("permission "+tc.name, func(t *testing.T) {
			fx := newAnswerFx(t)
			fx.targets.setSet(permissionSet("pp1", "r1"))
			p := fx.mustPropose(t, permissionArgs)
			fx.delivery.permErr = tc.err
			got, err := settledRefusal(fx.approveAnswer(p, nil))
			if err != nil || got.Status != tc.want.status || got.Error == nil || *got.Error != tc.want.code {
				t.Fatalf("got=%+v err=%v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestApproveAnswerGoneSessionDecidedByTaskAvailability(t *testing.T) {
	for _, tc := range []struct {
		name     string
		archived bool
		want     string
	}{{"task available", false, "stale"}, {"task archived", true, "task_archived"}} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newAnswerFx(t)
			p := fx.mustPropose(t, clarificationArgs)
			fx.delivery.won, fx.delivery.resolveErr = false, clarification.ErrBundleNotFound
			fx.delivery.resolveHook = func() {
				if tc.archived {
					now := time.Now()
					fx.tasks.target.ArchivedAt = &now
				}
			}
			got, err := settledRefusal(fx.approveAnswer(p, nil))
			if err != nil || !isRefusedAnswer(got) || *got.Error != tc.want {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

func TestApproveAnswerStepErrors(t *testing.T) {
	t.Run("target read error fails not_delivered and retry succeeds", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		fx.targets.err = ErrAnswerTargetRead
		got, err := fx.approveAnswer(p, nil)
		if err != nil || got.Status != ProposalStatusFailed || *got.Error != "not_delivered" || fx.delivery.resolveCount() != 0 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		fx.targets.err = nil
		got, err = fx.approveAnswer(got, nil)
		if err != nil || got.Status != ProposalStatusApproved {
			t.Fatalf("retry got=%+v err=%v", got, err)
		}
	})
	t.Run("draft no longer valid fails invalid_draft until edited", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		q := yesNo("q1")
		q.Options = []clarification.Option{{ID: "z", Label: "Zed"}}
		fx.targets.setSet(clarificationSet("p1", q, yesNo("q2")))
		got, err := fx.approveAnswer(p, nil)
		if err != nil || got.Status != ProposalStatusFailed || *got.Error != "invalid_draft" || fx.delivery.resolveCount() != 0 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		again, err := fx.approveAnswer(got, nil)
		if err != nil || *again.Error != "invalid_draft" {
			t.Fatalf("retry without an edit must fail the same way: %+v %v", again, err)
		}
		fixed, err := fx.approveAnswer(again, editBody(`{"answers":[{"question_id":"q1","option_ids":["z"]},{"question_id":"q2","option_ids":["a"]}]}`))
		if err != nil || fixed.Status != ProposalStatusApproved {
			t.Fatalf("edited retry got=%+v err=%v", fixed, err)
		}
	})
	t.Run("delivery not wired", func(t *testing.T) {
		fx := newAnswerFx(t)
		p := fx.mustPropose(t, clarificationArgs)
		fx.svc.SetKindDeps(KindDeps{Tasks: fx.tasks})
		got, err := fx.approveAnswer(p, nil)
		if err != nil || *got.Error != "not_delivered" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

func TestApproveAnswerOutcomeUnknownIsNeverReclaimed(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	fx.delivery.won, fx.delivery.resolveErr = true, errors.New("boom")
	got, _ := fx.approveAnswer(p, nil)
	if *got.Error != "outcome_unknown" {
		t.Fatalf("got=%+v", got)
	}
	fx.delivery.resolveErr = nil
	if _, err := fx.approveAnswer(got, nil); !isConflict(err) {
		t.Fatalf("err = %v, want conflict", err)
	}
	if fx.delivery.resolveCount() != 1 {
		t.Fatalf("deliveries = %d, want 1", fx.delivery.resolveCount())
	}
	// A claim that read a retryable failure loses to a row since settled outcome_unknown.
	stale := *got
	stale.Error = nil
	if _, err := fx.approveAnswer(&stale, nil); !isConflict(err) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestApproveAnswerTwoApproversDeliverOnce(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = fx.svc.ApproveProposal(context.Background(), "ws-1", fx.c.ID, p.ID, nil)
		}(i)
	}
	wg.Wait()
	if fx.delivery.resolveCount() != 1 {
		t.Fatalf("deliveries = %d, want exactly 1 (errs %v)", fx.delivery.resolveCount(), results)
	}
	var winners, conflicts int
	for _, err := range results {
		switch {
		case err == nil:
			winners++
		case isConflict(err):
			conflicts++
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("want one winner and one conflict, got winners=%d conflicts=%d (errs %v)", winners, conflicts, results)
	}
	if fx.reload(t, p.ID).Status != ProposalStatusApproved {
		t.Fatal("the proposal must end approved")
	}
}

func TestApproveAnswerApprovingRowRules(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	fx.forceApproving(t, p, time.Now())
	if _, err := fx.approveAnswer(p, editBody(`{"answers":[]}`)); !isConflict(err) {
		t.Fatalf("an edit-carrying approve on an approving row: %v", err)
	}
	if _, err := fx.approveAnswer(fx.reload(t, p.ID), nil); !isConflict(err) {
		t.Fatalf("a fresh claim: %v", err)
	}
	fx.forceApproving(t, p, time.Now().Add(-2*staleApproveWindow))
	got, err := fx.approveAnswer(fx.reload(t, p.ID), nil)
	if err != nil || got.Status != ProposalStatusFailed || *got.Error != "outcome_unknown" || fx.delivery.resolveCount() != 0 {
		t.Fatalf("stale claim got=%+v err=%v", got, err)
	}
}

func TestApproveAnswerFlagAndClass(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	mustSave(t, fx.svc, fx.c.WorkspaceID, fx.c.ID, policyBody(map[string]string{"answer": "denied"}))
	if got, err := fx.approveAnswer(p, nil); err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("a denied class must not block a pending approval: %+v %v", got, err)
	}
	q := newAnswerFx(t)
	p2 := q.mustPropose(t, clarificationArgs)
	q.svc.answer = false
	_, err := q.approveAnswer(p2, nil)
	var denied *PolicyDeniedError
	if !errors.As(err, &denied) || denied.Action != ActionAnswer || q.delivery.resolveCount() != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestRejectAnswerProposal(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	got, err := fx.svc.RejectProposal(context.Background(), "ws-1", fx.c.ID, p.ID, RejectProposalRequest{Reason: strPtr("no")})
	if err != nil || got.Status != ProposalStatusRejected || isRefusedAnswer(got) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestReplyToAnswerProposalNeedsFlagOnlyWhilePending(t *testing.T) {
	fx := newAnswerFx(t)
	fx.svc.phase3 = true
	p := fx.mustPropose(t, clarificationArgs)
	fx.svc.answer = false
	_, err := fx.svc.ReplyToProposal(context.Background(), "ws-1", fx.c.ID, p.ID, ReplyRequest{Text: "condition"})
	var denied *PolicyDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("err = %v", err)
	}
	if got := replyTitle(p); got != "Answer task task-0" {
		t.Fatalf("title = %q", got)
	}
}
