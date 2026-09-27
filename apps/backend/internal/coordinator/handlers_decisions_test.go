package coordinator

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

// newDecisionHandlersForTest wraps approveFixture's Service in Handlers, for
// the approve/reject HTTP handler tests below.
func newDecisionHandlersForTest(t *testing.T) (*Handlers, *Store, *Coordinator, *fakeDecisionTaskService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, c, tasks, svc := approveFixture(t)
	return NewHandlers(svc, newTestLogger(t)), store, c, tasks
}

func proposalParams(cid, pid string) gin.Params {
	return append(workspaceParams(cid), gin.Param{Key: "pid", Value: pid})
}

func TestHTTPApproveProposal(t *testing.T) {
	t.Run("200 with an empty body approves the base spec unchanged", func(t *testing.T) {
		h, store, c, tasks := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		tasks.createResult = createdResult("task-new")
		tasks.settled = true

		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", "", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var dto ProposalDTO
		decodeBody(t, rec, &dto)
		if dto.Status != ProposalStatusApproved || dto.TaskID == nil || *dto.TaskID != "task-new" {
			t.Fatalf("dto = %+v, want approved/task-new", dto)
		}
	})

	t.Run("200 with an empty JSON object approves the base spec unchanged", func(t *testing.T) {
		h, store, c, tasks := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		tasks.createResult = createdResult("task-new")
		tasks.settled = true

		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", "{}", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	t.Run("400 on malformed JSON", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())

		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", `{"title":`, proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("400 naming the field on an invalid edit", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())

		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", `{"title":null}`, proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		var resp ErrorResponse
		decodeBody(t, rec, &resp)
		if resp.Field != "title" {
			t.Errorf("Field = %q, want %q", resp.Field, "title")
		}
	})

	t.Run("409 with the current row on a settled proposal", func(t *testing.T) {
		h, store, c, tasks := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		tasks.createResult = createdResult("task-new")
		tasks.settled = true
		first := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", "", proposalParams(c.ID, p.ID))
		if first.Code != http.StatusOK {
			t.Fatalf("setup: status = %d, body=%s", first.Code, first.Body.String())
		}

		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", "", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
		}
		var resp ProposalConflictResponse
		decodeBody(t, rec, &resp)
		if resp.ErrorCode != ErrorCodeProposalConflict || resp.Proposal.Status != ProposalStatusApproved {
			t.Fatalf("resp = %+v, want proposal_conflict/approved", resp)
		}
	})

	t.Run("404 when absent", func(t *testing.T) {
		h, _, c, _ := newDecisionHandlersForTest(t)
		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/missing/approve", "", proposalParams(c.ID, "missing"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("403 when the caller lacks workspace.manage", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		h.service.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}

		rec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", "", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})
}

func TestHTTPRejectProposal(t *testing.T) {
	t.Run("200 with an empty body rejects with no reason", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())

		rec := runHandler(h.httpRejectProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/reject", "", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var dto ProposalDTO
		decodeBody(t, rec, &dto)
		if dto.Status != ProposalStatusRejected || dto.RejectReason != nil {
			t.Fatalf("dto = %+v, want rejected with no reason", dto)
		}
	})

	t.Run("200 with a reason", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())

		rec := runHandler(h.httpRejectProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/reject", `{"reason":"no thanks"}`, proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var dto ProposalDTO
		decodeBody(t, rec, &dto)
		if dto.RejectReason == nil || *dto.RejectReason != "no thanks" {
			t.Fatalf("RejectReason = %v, want \"no thanks\"", dto.RejectReason)
		}
	})

	t.Run("400 naming reason when too long", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		reason := strings.Repeat("a", proposalRejectReasonMaxRunes+1)

		rec := runHandler(h.httpRejectProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/reject", `{"reason":"`+reason+`"}`, proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		var resp ErrorResponse
		decodeBody(t, rec, &resp)
		if resp.Field != "reason" {
			t.Errorf("Field = %q, want %q", resp.Field, "reason")
		}
	})

	t.Run("409 with the current row on an already-decided proposal", func(t *testing.T) {
		h, store, c, tasks := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		tasks.createResult = createdResult("task-new")
		tasks.settled = true
		approveRec := runHandler(h.httpApproveProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/approve", "", proposalParams(c.ID, p.ID))
		if approveRec.Code != http.StatusOK {
			t.Fatalf("setup: status = %d, body=%s", approveRec.Code, approveRec.Body.String())
		}

		rec := runHandler(h.httpRejectProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/reject", "", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
		}
		var resp ProposalConflictResponse
		decodeBody(t, rec, &resp)
		if resp.ErrorCode != ErrorCodeProposalConflict || resp.Proposal.Status != ProposalStatusApproved {
			t.Fatalf("resp = %+v, want proposal_conflict/approved", resp)
		}
	})

	t.Run("404 when absent", func(t *testing.T) {
		h, _, c, _ := newDecisionHandlersForTest(t)
		rec := runHandler(h.httpRejectProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/missing/reject", "", proposalParams(c.ID, "missing"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("403 when the caller lacks workspace.manage", func(t *testing.T) {
		h, store, c, _ := newDecisionHandlersForTest(t)
		p := insertProposal(t, store, c, sampleSpec())
		h.service.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}

		rec := runHandler(h.httpRejectProposal, http.MethodPost,
			"/api/v1/workspaces/ws-1/coordinators/"+c.ID+"/proposals/"+p.ID+"/reject", "", proposalParams(c.ID, p.ID))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})
}
