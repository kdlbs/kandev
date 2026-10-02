package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	svcpkg "github.com/kandev/kandev/internal/task/service"
)

const testWorkspaceID = "ws-1"

func newHandlersForTest(
	t *testing.T,
	agents map[string]*settingsmodels.AgentProfile,
	executors map[string]*taskmodels.ExecutorProfile,
	authzErr error,
) *Handlers {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newServiceForTest(t, agents, executors, authzErr)
	return NewHandlers(svc, newTestLogger(t))
}

// runHandler invokes handler directly with path params and an optional JSON
// body, mirroring internal/clarification/inbox_handlers_test.go's pattern.
func runHandler(handler gin.HandlerFunc, method, target, body string, params gin.Params) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = params
	handler(c)
	c.Writer.WriteHeaderNow()
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
}

func workspaceParams(cid string) gin.Params {
	params := gin.Params{{Key: "id", Value: testWorkspaceID}}
	if cid != "" {
		params = append(params, gin.Param{Key: "cid", Value: cid})
	}
	return params
}

func TestHTTPCreateCoordinator(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("201 with the created coordinator", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		rec := runHandler(h.httpCreateCoordinator, http.MethodPost, "/api/v1/workspaces/ws-1/coordinators",
			`{"name":"Coordinator","agent_profile_id":"ap-1","executor_profile_id":"ep-1"}`, workspaceParams(""))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var dto CoordinatorDTO
		decodeBody(t, rec, &dto)
		if dto.Name != "Coordinator" || dto.ID == "" {
			t.Errorf("body = %+v, want a named coordinator with an id", dto)
		}
	})

	t.Run("400 on malformed JSON", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		rec := runHandler(h.httpCreateCoordinator, http.MethodPost, "/api/v1/workspaces/ws-1/coordinators",
			`{"name":`, workspaceParams(""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("400 naming the field on invalid input", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		rec := runHandler(h.httpCreateCoordinator, http.MethodPost, "/api/v1/workspaces/ws-1/coordinators",
			`{"name":"","agent_profile_id":"ap-1","executor_profile_id":"ep-1"}`, workspaceParams(""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		var resp ErrorResponse
		decodeBody(t, rec, &resp)
		if resp.Field != "name" {
			t.Errorf("Field = %q, want %q", resp.Field, "name")
		}
	})

	t.Run("404 when the workspace cannot be read", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, repoerrors.ErrWorkspaceNotFound)
		rec := runHandler(h.httpCreateCoordinator, http.MethodPost, "/api/v1/workspaces/ws-1/coordinators",
			`{"name":"Coordinator","agent_profile_id":"ap-1","executor_profile_id":"ep-1"}`, workspaceParams(""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("403 when forbidden", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, svcpkg.ErrForbidden)
		rec := runHandler(h.httpCreateCoordinator, http.MethodPost, "/api/v1/workspaces/ws-1/coordinators",
			`{"name":"Coordinator","agent_profile_id":"ap-1","executor_profile_id":"ep-1"}`, workspaceParams(""))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("500 on an unexpected error", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, errors.New("boom"))
		rec := runHandler(h.httpCreateCoordinator, http.MethodPost, "/api/v1/workspaces/ws-1/coordinators",
			`{"name":"Coordinator","agent_profile_id":"ap-1","executor_profile_id":"ep-1"}`, workspaceParams(""))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestHTTPListCoordinators(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("200 with an empty array, never null", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		rec := runHandler(h.httpListCoordinators, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators", "", workspaceParams(""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `"coordinators":[]`) {
			t.Errorf("body = %s, want an empty coordinators array", rec.Body.String())
		}
	})

	t.Run("200 with open_proposals per coordinator", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		if err := h.service.store.InsertProposal(context.Background(), &Proposal{
			CoordinatorID: created.ID, WorkspaceID: testWorkspaceID,
			Spec: ProposalSpec{Title: "t", WorkflowID: "wf", StepID: "step", RepositoryID: "repo"},
		}, false); err != nil {
			t.Fatalf("InsertProposal() unexpected error: %v", err)
		}

		rec := runHandler(h.httpListCoordinators, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators", "", workspaceParams(""))
		var resp CoordinatorListResponse
		decodeBody(t, rec, &resp)
		if len(resp.Coordinators) != 1 || resp.Coordinators[0].OpenProposals == nil || *resp.Coordinators[0].OpenProposals != 1 {
			t.Fatalf("body = %s, want one coordinator with open_proposals=1", rec.Body.String())
		}
	})
}

func TestHTTPGetCoordinator(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("200 with profile statuses", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		rec := runHandler(h.httpGetCoordinator, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/"+created.ID, "", workspaceParams(created.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var dto CoordinatorDTO
		decodeBody(t, rec, &dto)
		if dto.AgentProfileStatus == nil || *dto.AgentProfileStatus != ProfileStatusOK {
			t.Errorf("AgentProfileStatus = %v, want ok", dto.AgentProfileStatus)
		}
		if dto.ExecutorProfileStatus == nil || *dto.ExecutorProfileStatus != ProfileStatusOK {
			t.Errorf("ExecutorProfileStatus = %v, want ok", dto.ExecutorProfileStatus)
		}
	})

	t.Run("404 when absent", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		rec := runHandler(h.httpGetCoordinator, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/missing", "", workspaceParams("missing"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestHTTPPatchCoordinator(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("200 with the updated coordinator", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		rec := runHandler(h.httpPatchCoordinator, http.MethodPatch, "/api/v1/workspaces/ws-1/coordinators/"+created.ID,
			`{"name":"New name"}`, workspaceParams(created.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var dto CoordinatorDTO
		decodeBody(t, rec, &dto)
		if dto.Name != "New name" {
			t.Errorf("Name = %q, want %q", dto.Name, "New name")
		}
	})

	t.Run("400 naming the field on a null value", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		rec := runHandler(h.httpPatchCoordinator, http.MethodPatch, "/api/v1/workspaces/ws-1/coordinators/"+created.ID,
			`{"name":null}`, workspaceParams(created.ID))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		var resp ErrorResponse
		decodeBody(t, rec, &resp)
		if resp.Field != "name" {
			t.Errorf("Field = %q, want %q", resp.Field, "name")
		}
	})
}

func TestHTTPDeleteCoordinator(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("204 on success", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		rec := runHandler(h.httpDeleteCoordinator, http.MethodDelete, "/api/v1/workspaces/ws-1/coordinators/"+created.ID, "", workspaceParams(created.ID))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	})

	t.Run("404 when absent", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		rec := runHandler(h.httpDeleteCoordinator, http.MethodDelete, "/api/v1/workspaces/ws-1/coordinators/missing", "", workspaceParams("missing"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestHTTPListProposals(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	setup := func(t *testing.T) (*Handlers, string) {
		t.Helper()
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		if err := h.service.store.InsertProposal(context.Background(), &Proposal{
			CoordinatorID: created.ID, WorkspaceID: testWorkspaceID,
			Spec: ProposalSpec{Title: "t", WorkflowID: "wf", StepID: "step", RepositoryID: "repo"},
		}, false); err != nil {
			t.Fatalf("InsertProposal() unexpected error: %v", err)
		}
		return h, created.ID
	}

	t.Run("no parameter returns the pending list", func(t *testing.T) {
		h, cid := setup(t)
		rec := runHandler(h.httpListProposals, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/"+cid+"/proposals", "", workspaceParams(cid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var resp ProposalListResponse
		decodeBody(t, rec, &resp)
		if len(resp.Proposals) != 1 {
			t.Fatalf("Proposals len = %d, want 1", len(resp.Proposals))
		}
	})

	t.Run("status=all returns the all list", func(t *testing.T) {
		h, cid := setup(t)
		rec := runHandler(h.httpListProposals, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/"+cid+"/proposals?status=all", "", workspaceParams(cid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	t.Run("404 for an unknown coordinator id", func(t *testing.T) {
		h, _ := setup(t)
		rec := runHandler(h.httpListProposals, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/missing/proposals", "", workspaceParams("missing"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("404 for a coordinator of another workspace", func(t *testing.T) {
		h, cid := setup(t)
		params := gin.Params{{Key: "id", Value: "other-workspace"}, {Key: "cid", Value: cid}}
		rec := runHandler(h.httpListProposals, http.MethodGet, "/api/v1/workspaces/other-workspace/coordinators/"+cid+"/proposals?status=all", "", params)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	for _, raw := range []string{"", "PENDING", "approved"} {
		t.Run("status="+raw+" is 400 naming status", func(t *testing.T) {
			h, cid := setup(t)
			rec := runHandler(h.httpListProposals, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/"+cid+"/proposals?status="+raw, "", workspaceParams(cid))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			var resp ErrorResponse
			decodeBody(t, rec, &resp)
			if resp.Field != "status" {
				t.Errorf("Field = %q, want %q", resp.Field, "status")
			}
		})
	}
}

func TestHTTPGetProposal(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("200 with the proposal", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		created, err := h.service.CreateCoordinator(context.Background(), testWorkspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		proposal := &Proposal{
			CoordinatorID: created.ID, WorkspaceID: testWorkspaceID,
			Spec: ProposalSpec{Title: "t", WorkflowID: "wf", StepID: "step", RepositoryID: "repo"},
		}
		if err := h.service.store.InsertProposal(context.Background(), proposal, false); err != nil {
			t.Fatalf("InsertProposal() unexpected error: %v", err)
		}
		params := append(workspaceParams(created.ID), gin.Param{Key: "pid", Value: proposal.ID})
		rec := runHandler(h.httpGetProposal, http.MethodGet,
			"/api/v1/workspaces/ws-1/coordinators/"+created.ID+"/proposals/"+proposal.ID, "", params)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var dto ProposalDTO
		decodeBody(t, rec, &dto)
		if dto.ID != proposal.ID {
			t.Errorf("ID = %q, want %q", dto.ID, proposal.ID)
		}
	})

	t.Run("404 when absent", func(t *testing.T) {
		h := newHandlersForTest(t, agents, executors, nil)
		params := append(workspaceParams("cid"), gin.Param{Key: "pid", Value: "missing"})
		rec := runHandler(h.httpGetProposal, http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/cid/proposals/missing", "", params)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestHTTPListStalls(t *testing.T) {
	t.Run("200 with an empty array, never null", func(t *testing.T) {
		h := newHandlersForTest(t, nil, nil, nil)
		rec := runHandler(h.httpListStalls, http.MethodGet, "/api/v1/workspaces/ws-1/coordinator-stalls", "", workspaceParams(""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `"stalls":[]`) {
			t.Errorf("body = %s, want an empty stalls array", rec.Body.String())
		}
	})
}
