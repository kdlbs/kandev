package coordinator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHTTPMergeRoutesRegisteredOnlyWithMergeFlag(t *testing.T) {
	for _, on := range []bool{false, true} {
		_, _, svc := phase2Fixture(t, true)
		svc.merge = on
		router := gin.New()
		RegisterRoutes(router, svc, newTestLogger(t))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+testWorkspaceID+"/coordinators/x/tasks/t/dependencies", nil))
		if got := rec.Body.String() == "404 page not found"; got == on {
			t.Fatalf("merge=%v: unregistered=%v", on, got)
		}
	}
}

func TestHTTPDependencyErrorsMapTo400And409(t *testing.T) {
	h, _, c := settingsHandlers(t)
	h.service.merge = true
	params := append(workspaceParams(c.ID), gin.Param{Key: "tid", Value: "t1"})
	rec := runHandler(h.httpPutDependencies, http.MethodPut, "/x", `{"refs":[]}`, params)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing revision: status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = runHandler(h.httpPutDependencies, http.MethodPut, "/x", `{"dependency_revision":4,"refs":[]}`, params)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale revision: status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	decodeBody(t, rec, &got)
	if got["code"] != codeRevisionConflict {
		t.Fatalf("body = %v", got)
	}
}

func TestHTTPSettingsMergeConflictIs409(t *testing.T) {
	h, store, c := settingsHandlers(t)
	h.service.merge = true
	mustExec(t, store, `CREATE TABLE repositories (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL, deleted_at `+tsType(store)+`);
		INSERT INTO repositories (id, workspace_id, name) VALUES ('r1', '`+c.WorkspaceID+`', 'one')`)
	rec := runHandler(h.httpPutSettings, http.MethodPut, "/x", `{"policy_revision":99,"merge":{"repositories":[{"repository_id":"r1","policy":"requires_approval"}]}}`, workspaceParams(c.ID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPGetChangesReturnsEmptyListWithoutLinks(t *testing.T) {
	h, _, c := settingsHandlers(t)
	h.service.merge = true
	params := append(workspaceParams(c.ID), gin.Param{Key: "tid", Value: "t1"})
	rec := runHandler(h.httpGetChanges, http.MethodGet, "/x?retry=1", "", params)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Changes []ChangeView `json:"changes"`
	}
	decodeBody(t, rec, &got)
	if got.Changes == nil || len(got.Changes) != 0 {
		t.Fatalf("changes = %#v", got.Changes)
	}
	params = append(workspaceParams("missing"), gin.Param{Key: "tid", Value: "t1"})
	if rec = runHandler(h.httpGetChanges, http.MethodGet, "/x", "", params); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown coordinator: status = %d", rec.Code)
	}
}
