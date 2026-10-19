package coordinator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func improvementRoutes(t *testing.T, f *improvementFixture) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, f.svc, newTestLogger(t))
	return router, "/api/v1/workspaces/ws-1/coordinators/" + f.c.ID + "/pending-changes"
}

func doGet(router *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestImprovementRoutesListApplyDiscard(t *testing.T) {
	f := newImprovementFixture(t)
	router, base := improvementRoutes(t, f)
	_, change := f.approved(t)

	rec := doGet(router, base)
	var list struct {
		Changes []PendingChange `json:"changes"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Changes) != 1 || list.Changes[0].ID != change.ID {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	rec = postJSON(router, base+"/"+change.ID+"/apply", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "new instructions") {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(router, base+"/"+change.ID+"/apply", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"status":"applied"`) {
		t.Fatalf("second apply = %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(router, base+"/"+change.ID+"/discard", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("discard of applied = %d %s", rec.Code, rec.Body.String())
	}
}

func TestImprovementRouteContextChangedConflictBody(t *testing.T) {
	f := newImprovementFixture(t)
	router, base := improvementRoutes(t, f)
	_, change := f.approved(t)
	f.setContext(t, "edited elsewhere")

	rec := postJSON(router, base+"/"+change.ID+"/apply", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error  string        `json:"error"`
		Reason string        `json:"reason"`
		Change PendingChange `json:"change"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "conflict" || body.Reason != ChangeReasonContextChanged || body.Change.ID != change.ID {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestImprovementRouteNotFoundAndInvalid(t *testing.T) {
	f := newImprovementFixture(t)
	router, base := improvementRoutes(t, f)
	_, change := f.approved(t)

	if rec := postJSON(router, base+"/missing/apply", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown apply = %d", rec.Code)
	}
	if rec := postJSON(router, base+"/missing/discard", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown discard = %d", rec.Code)
	}
	mustExec(t, f.store, `UPDATE coordinator_pending_changes SET new_value = ? WHERE id = ?`, strings.Repeat("x", contextMaxRunes+1), change.ID)
	if rec := postJSON(router, base+"/"+change.ID+"/apply", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid stored value = %d %s", rec.Code, rec.Body.String())
	}
}

func TestImprovementRoutesAbsentWhenPhase3Off(t *testing.T) {
	f := newImprovementFixture(t)
	f.svc.phase3 = false
	router, base := improvementRoutes(t, f)
	if rec := doGet(router, base); rec.Code != http.StatusNotFound {
		t.Fatalf("list = %d", rec.Code)
	}
	if rec := postJSON(router, base+"/x/apply", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("apply = %d", rec.Code)
	}
}

func TestImprovementRoutesForbiddenForReaders(t *testing.T) {
	f := newImprovementFixture(t)
	router, base := improvementRoutes(t, f)
	_, change := f.approved(t)
	f.svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}
	if rec := doGet(router, base); rec.Code != http.StatusForbidden {
		t.Fatalf("list = %d", rec.Code)
	}
	if rec := postJSON(router, base+"/"+change.ID+"/apply", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("apply = %d", rec.Code)
	}
	if rec := postJSON(router, base+"/"+change.ID+"/discard", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("discard = %d", rec.Code)
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("forbidden apply changed the context")
	}
}
