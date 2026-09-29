package coordinator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func settingsHandlers(t *testing.T) (*Handlers, *Store, *Coordinator) {
	t.Helper()
	store, c, svc := phase2Fixture(t, true)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", c.WorkspaceID)
	addWorkflow(t, store, "wf-b", c.WorkspaceID)
	return &Handlers{service: svc, logger: newTestLogger(t)}, store, c
}

func TestHTTPSettingsRoutesRegisteredOnlyWithPhase2(t *testing.T) {
	for _, on := range []bool{false, true} {
		_, _, svc := phase2Fixture(t, on)
		router := gin.New()
		RegisterRoutes(router, svc, newTestLogger(t))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+testWorkspaceID+"/coordinators/x/settings", nil))
		if got := rec.Body.String() == "404 page not found"; got == on {
			t.Fatalf("phase2=%v: unregistered=%v", on, got)
		}
	}
}

func TestHTTPPutSettingsReturnsCoordinatorDTO(t *testing.T) {
	h, _, c := settingsHandlers(t)
	body := `{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`
	rec := runHandler(h.httpPutSettings, http.MethodPut, "/x", body, workspaceParams(c.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var dto CoordinatorDTO
	decodeBody(t, rec, &dto)
	if dto.ID != c.ID || dto.CoordinatorPhase2 == nil || dto.Watches.Scope != "selected" || len(dto.Watches.WorkflowIDs) != 1 {
		t.Fatalf("dto = %+v", dto)
	}
}

func TestHTTPPutSettingsValidationIs400WithCode(t *testing.T) {
	h, _, c := settingsHandlers(t)
	rec := runHandler(h.httpPutSettings, http.MethodPut, "/x", `{"policy":{"actions":{"stop":"automatic"}}}`, workspaceParams(c.ID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	decodeBody(t, rec, &got)
	if got["code"] == "" || got["field"] == "" || got["error"] == "" {
		t.Fatalf("body = %v, want error, field and code", got)
	}
}

func TestHTTPSettingsStaleWorkflowIsAbsentFromEveryDTO(t *testing.T) {
	h, store, c := settingsHandlers(t)
	rec := runHandler(h.httpPutSettings, http.MethodPut, "/x", `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-b"]}}`, workspaceParams(c.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", rec.Code, rec.Body.String())
	}
	dropWorkflow(t, store, "wf-b")

	for name, handler := range map[string]gin.HandlerFunc{"settings": h.httpGetSettings, "coordinator": h.httpGetCoordinator} {
		rec := runHandler(handler, http.MethodGet, "/x", "", workspaceParams(c.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", name, rec.Code, rec.Body.String())
		}
		var raw map[string]json.RawMessage
		decodeBody(t, rec, &raw)
		var watches CoordinatorWatchDTO
		if err := json.Unmarshal(raw["watches"], &watches); err != nil {
			t.Fatalf("%s watches: %v", name, err)
		}
		if len(watches.WorkflowIDs) != 1 || watches.WorkflowIDs[0] != "wf-a" {
			t.Fatalf("%s workflow_ids = %v, want [wf-a]", name, watches.WorkflowIDs)
		}
	}
	rec = runHandler(h.httpListCoordinators, http.MethodGet, "/x", "", workspaceParams(""))
	if rec.Code != http.StatusOK || json.Valid(rec.Body.Bytes()) == false {
		t.Fatalf("list status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"wf-a"`) || strings.Contains(rec.Body.String(), `"wf-b"`) {
		t.Fatalf("list body = %s, want only the effective ids", rec.Body.String())
	}
}

func TestHTTPApproveDeniedActionIs409PolicyDenied(t *testing.T) {
	h, store, c := settingsHandlers(t)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	rec := runHandler(h.httpPutSettings, http.MethodPut, "/x", policyBody(map[string]string{"create_task": "denied"}), workspaceParams(c.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", rec.Code, rec.Body.String())
	}
	params := append(workspaceParams(c.ID), gin.Param{Key: "pid", Value: p.ID})
	rec = runHandler(h.httpApproveProposal, http.MethodPost, "/x", "", params)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	decodeBody(t, rec, &got)
	if got["error"] != "policy_denied" || got["action"] != "create_task" {
		t.Fatalf("body = %v", got)
	}
}
