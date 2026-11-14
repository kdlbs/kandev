package coordinator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func refusalBody(t *testing.T, fx *answerFx, err error) (int, map[string]json.RawMessage) {
	t.Helper()
	h := NewHandlers(fx.svc, newTestLogger(t))
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	h.respondError(ctx, err)
	var body map[string]json.RawMessage
	if jerr := json.Unmarshal(rec.Body.Bytes(), &body); jerr != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), jerr)
	}
	return rec.Code, body
}

func TestRefusedAnswerConflictBody(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	fx.targets.setSet(clarificationSet("p9", yesNo("q1")))
	_, err := fx.approveAnswer(p, nil)
	if !isConflict(err) {
		t.Fatalf("approve that settles a refusal: err = %v, want conflict", err)
	}
	code, body := refusalBody(t, fx, err)
	if code != http.StatusConflict || string(body["error"]) != `"refused"` || string(body["code"]) != `"stale"` {
		t.Fatalf("status %d body %v", code, body)
	}
	var dto struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body["proposal"], &dto); err != nil || dto.Status != "rejected" || dto.Error != "stale" {
		t.Fatalf("proposal = %s (%v)", body["proposal"], err)
	}

	// Reject and reply on the refused row answer with the same body.
	refused, gerr := fx.store.GetProposal(t.Context(), fx.c.WorkspaceID, fx.c.ID, p.ID, true)
	if gerr != nil {
		t.Fatal(gerr)
	}
	_, rerr := fx.svc.RejectProposal(authedContext("mgr-1"), fx.c.WorkspaceID, fx.c.ID, refused.ID, RejectProposalRequest{})
	code, body = refusalBody(t, fx, rerr)
	if code != http.StatusConflict || string(body["error"]) != `"refused"` {
		t.Fatalf("reject of a refused row: status %d body %v (err %v)", code, body, rerr)
	}
}

func TestPersonRejectKeepsPlainConflictShape(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	if _, err := fx.svc.RejectProposal(authedContext("mgr-1"), fx.c.WorkspaceID, fx.c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	_, err := fx.approveAnswer(p, nil)
	code, body := refusalBody(t, fx, err)
	if code != http.StatusConflict || string(body["error"]) != `"proposal_conflict"` {
		t.Fatalf("status %d body %v", code, body)
	}
}

func TestProposalDTORetryableOnlyForTheFourCodes(t *testing.T) {
	for code, want := range map[string]bool{
		"invalid_draft": true, "option_not_offered": true, "resolution_in_progress": true, "not_delivered": true,
		"outcome_unknown": false, "stale": false,
	} {
		c := code
		p := &Proposal{Kind: ProposalKindAnswer, Status: ProposalStatusFailed, Error: &c}
		dto := NewProposalDTOPhases(p, true, true)
		if got := dto.ProposalPhase2 != nil && dto.Retryable; got != want {
			t.Errorf("%s: retryable = %v, want %v", code, got, want)
		}
	}
	other := "not_delivered"
	if dto := NewProposalDTOPhases(&Proposal{Kind: ProposalKindMove, Status: ProposalStatusFailed, Error: &other}, true, true); dto.Retryable {
		t.Error("a non-answer kind must never be retryable")
	}
}

func TestLiveEditReadErrorIsHTTP500WithCode(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	fx.targets.mu.Lock()
	fx.targets.err = ErrAnswerTargetRead
	fx.targets.mu.Unlock()
	_, err := fx.approveAnswer(p, editBody(`{"answers":[{"question_id":"q1","option_ids":["b"]},{"question_id":"q2","option_ids":["a"]}]}`))
	code, body := refusalBody(t, fx, err)
	if code != http.StatusInternalServerError || string(body["code"]) != `"target_read_failed"` {
		t.Fatalf("status %d body %v err %v", code, body, err)
	}
	got, gerr := fx.store.GetProposal(t.Context(), fx.c.WorkspaceID, fx.c.ID, p.ID, true)
	if gerr != nil || got.Status != ProposalStatusPending {
		t.Fatalf("row after a failed live read = %+v (%v), want pending", got, gerr)
	}
}

func TestSummaryApprovalActionsCountsAnswerOnlyWhenNamed(t *testing.T) {
	base := func(answer *Setting) *CoordinatorDTO {
		actions := map[Action]Setting{ActionCreateTask: SettingRequiresApproval, ActionMove: SettingDenied}
		if answer != nil {
			actions[ActionAnswer] = *answer
		}
		return (&CoordinatorDTO{CoordinatorPhase2: &CoordinatorPhase2{
			Watches: CoordinatorWatchDTO{Scope: watchScopeAll}, Policy: CoordinatorPolicyDTO{Actions: actions},
		}}).WithSummary(0)
	}
	ra, denied := SettingRequiresApproval, SettingDenied
	if got := base(nil).Summary.ApprovalActions; got != 1 {
		t.Errorf("flag off (answer unnamed) = %d, want 1", got)
	}
	if got := base(&ra).Summary.ApprovalActions; got != 2 {
		t.Errorf("answer requires_approval = %d, want 2", got)
	}
	if got := base(&denied).Summary.ApprovalActions; got != 1 {
		t.Errorf("answer denied = %d, want 1", got)
	}
}
