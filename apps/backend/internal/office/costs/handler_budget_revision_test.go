package costs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/costs"
)

// newBudgetTestRouter mounts the budget routes over a fresh CostService, for
// tests that need to observe the HTTP-level wire contract rather than the
// service API directly.
func newBudgetTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	svc, _, _ := newBudgetTestService(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	costs.NewHandler(svc).RegisterRoutes(router.Group("/api/v1"))
	return router
}

func doBudgetRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestCreateBudget_ResponseCarriesServerAssignedRevision covers
// AC-OFFICE-COSTS-003.2 and .4 on the wire: the 201 response's budget
// carries "revision": 1, never the column default leaking through as 0,
// and a client-presented revision in the create body has no effect (the
// request DTO has no such field, so this is enforced structurally, but the
// response must still report what the row actually holds).
func TestCreateBudget_ResponseCarriesServerAssignedRevision(t *testing.T) {
	router := newBudgetTestRouter(t)

	rec := doBudgetRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/budgets", `{
		"scope_type": "workspace",
		"limit_subcents": 1000,
		"period": "monthly",
		"alert_threshold_pct": 80,
		"action_on_exceed": "notify_only",
		"revision": 999
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Budget struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if body.Budget.Revision != 1 {
		t.Fatalf("create response revision = %d, want 1 (a presented 999 must not survive)", body.Budget.Revision)
	}
	if !strings.Contains(rec.Body.String(), `"revision"`) {
		t.Fatalf("response body must serialize revision as a visible field, got %s", rec.Body.String())
	}
}

// TestUpdateBudget_ClientRevisionFieldHasNoEffect covers
// AC-OFFICE-COSTS-003.7: the update request body has no revision field, so
// a client-supplied "revision" in the PATCH JSON is inert, and the response
// reports the server's own bump (2, after the create's 1) regardless of
// what the caller sent.
func TestUpdateBudget_ClientRevisionFieldHasNoEffect(t *testing.T) {
	router := newBudgetTestRouter(t)

	createRec := doBudgetRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/budgets", `{
		"scope_type": "workspace",
		"limit_subcents": 1000,
		"period": "monthly",
		"alert_threshold_pct": 80,
		"action_on_exceed": "notify_only"
	}`)
	var created struct {
		Budget struct {
			ID string `json:"id"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create body %q: %v", createRec.Body.String(), err)
	}

	updateRec := doBudgetRequest(t, router, http.MethodPatch, "/api/v1/budgets/"+created.Budget.ID, `{
		"limit_subcents": 2000,
		"revision": 999999
	}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", updateRec.Code, updateRec.Body.String())
	}

	var updated struct {
		Budget struct {
			Revision      int64 `json:"revision"`
			LimitSubcents int64 `json:"limit_subcents"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update body %q: %v", updateRec.Body.String(), err)
	}
	if updated.Budget.Revision != 2 {
		t.Fatalf("updated revision = %d, want 2 (a presented 999999 must not survive)", updated.Budget.Revision)
	}
	if updated.Budget.LimitSubcents != 2000 {
		t.Fatalf("limit_subcents = %d, want 2000", updated.Budget.LimitSubcents)
	}
}
