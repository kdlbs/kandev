package coordinator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/authz"
)

func activityHandlers(t *testing.T, phase2 bool) (*Handlers, *Store, *Coordinator, *fakeWorkspaceAuthorizer, *fakeUndoTasks) {
	t.Helper()
	svc, store, c, az, fake := newActivityService(t, phase2)
	return &Handlers{service: svc, logger: newTestLogger(t)}, store, c, az, fake
}

func activityParams(cid, rid string) gin.Params {
	p := workspaceParams(cid)
	if rid != "" {
		p = append(p, gin.Param{Key: "rid", Value: rid})
	}
	return p
}

func TestHTTPListActivity_PagesAndBadQueries(t *testing.T) {
	h, store, c, _, _ := activityHandlers(t, true)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"a", "b", "c"} {
		seedActivity(t, store, c.ID, id, ActionCreateTask, ActivityProposed, base.Add(time.Duration(i)*time.Second))
	}
	rec := runHandler(h.httpListActivity, http.MethodGet, "/x?limit=2", "", activityParams(c.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Rows       []map[string]any `json:"rows"`
		NextCursor *string          `json:"next_cursor"`
	}
	decodeBody(t, rec, &page)
	if len(page.Rows) != 2 || page.NextCursor == nil {
		t.Fatalf("page = %s", rec.Body.String())
	}
	rec = runHandler(h.httpListActivity, http.MethodGet, "/x?limit=2&before="+*page.NextCursor, "", activityParams(c.ID, ""))
	decodeBody(t, rec, &page)
	if len(page.Rows) != 1 || page.NextCursor != nil {
		t.Fatalf("page 2 = %s", rec.Body.String())
	}
	for query, field := range map[string]string{
		"limit=":                  "limit",
		"limit=0":                 "limit",
		"limit=51":                "limit",
		"limit=abc":               "limit",
		"limit=1&limit=2":         "limit",
		"before=":                 "before",
		"before=%21%21":           "before",
		"before=a&before=b":       "before",
		"class=bogus":             "class",
		"class=move&class=resume": "class",
	} {
		rec := runHandler(h.httpListActivity, http.MethodGet, "/x?"+query, "", activityParams(c.ID, ""))
		var body ErrorResponse
		decodeBody(t, rec, &body)
		if rec.Code != http.StatusBadRequest || body.Field != field {
			t.Errorf("%s: status = %d field = %q", query, rec.Code, body.Field)
		}
	}
	rec = runHandler(h.httpListActivity, http.MethodGet, "/x?class=", "", activityParams(c.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("empty class status = %d", rec.Code)
	}
}

func TestHTTPListActivity_ForeignCoordinator404AndReadScope(t *testing.T) {
	h, store, c, az, _ := activityHandlers(t, true)
	other := newTestCoordinator(t, store, "ws-2")
	rec := runHandler(h.httpListActivity, http.MethodGet, "/x", "", activityParams(other.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	runHandler(h.httpListActivity, http.MethodGet, "/x", "", activityParams(c.ID, ""))
	if az.scopes[len(az.scopes)-1] != authz.ScopeWorkspaceRead {
		t.Fatalf("scopes = %v", az.scopes)
	}
}

func TestHTTPActivitySummary_DaysValidation(t *testing.T) {
	h, _, c, _, _ := activityHandlers(t, true)
	for _, q := range []string{"days=0", "days=91", "days=", "days=x", "days=1&days=2"} {
		rec := runHandler(h.httpActivitySummary, http.MethodGet, "/x?"+q, "", activityParams(c.ID, ""))
		var body ErrorResponse
		decodeBody(t, rec, &body)
		if rec.Code != http.StatusBadRequest || body.Field != "days" {
			t.Errorf("%s: status = %d field = %q", q, rec.Code, body.Field)
		}
	}
	rec := runHandler(h.httpActivitySummary, http.MethodGet, "/x", "", activityParams(c.ID, ""))
	var sum ActivitySummary
	decodeBody(t, rec, &sum)
	if rec.Code != http.StatusOK || sum.Days != DefaultSummaryDays {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPUndoActivity_StatusCodes(t *testing.T) {
	h, store, c, az, _ := activityHandlers(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	rec := runHandler(h.httpUndoActivity, http.MethodPost, "/x", "", activityParams(c.ID, "r1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if az.scopes[len(az.scopes)-1] != authz.ScopeWorkspaceManage {
		t.Fatalf("scopes = %v", az.scopes)
	}
	rec = runHandler(h.httpUndoActivity, http.MethodPost, "/x", "", activityParams(c.ID, "r1"))
	var body map[string]any
	decodeBody(t, rec, &body)
	if rec.Code != http.StatusConflict || body["code"] != UndoAlreadyUndone {
		t.Fatalf("status = %d body = %v", rec.Code, body)
	}
	if _, hasRow := body["id"]; hasRow {
		t.Fatal("409 body carries a row")
	}
	rec = runHandler(h.httpUndoActivity, http.MethodPost, "/x", "", activityParams(c.ID, "absent"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("absent status = %d", rec.Code)
	}
}

func TestHTTPUndoActivity_ConflictCarriesReason(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	fake.tasks["t1"] = &UndoTask{Identifier: "KAN-1", WorkflowID: "wf", WorkflowStepID: "s9"}
	seedMoveRow(t, store, c, "m1", "t1", moveOutcomeS1toS2)
	h := &Handlers{service: svc, logger: newTestLogger(t)}
	rec := runHandler(h.httpUndoActivity, http.MethodPost, "/x", "", activityParams(c.ID, "m1"))
	var body map[string]string
	decodeBody(t, rec, &body)
	if rec.Code != http.StatusConflict || body["code"] != UndoConflict || body["reason"] != UndoReasonMoved {
		t.Fatalf("status = %d body = %v", rec.Code, body)
	}
}

func TestRegisterRoutes_ActivityRoutesOnlyWithPhase2(t *testing.T) {
	for _, on := range []bool{true, false} {
		svc, _, c, _, _ := newActivityService(t, on)
		gin.SetMode(gin.TestMode)
		router := gin.New()
		RegisterRoutes(router, svc, newTestLogger(t))
		for _, tc := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/" + c.ID + "/activity"},
			{http.MethodGet, "/api/v1/workspaces/ws-1/coordinators/" + c.ID + "/activity/summary"},
			{http.MethodPost, "/api/v1/workspaces/ws-1/coordinators/" + c.ID + "/activity/r/undo"},
		} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			notFound := rec.Body.String() == "404 page not found"
			if on == notFound {
				t.Errorf("phase2=%v: %s %s registered=%v", on, tc.method, tc.path, !notFound)
			}
		}
	}
}
