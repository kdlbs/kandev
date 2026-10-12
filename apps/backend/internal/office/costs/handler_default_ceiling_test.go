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

func newDefaultCeilingTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newBudgetTestService(t)
	router := gin.New()
	costs.NewHandler(svc).RegisterRoutes(router.Group("/api/v1"))
	return router
}

func doDefaultCeilingRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetDefaultCeiling_NoWriteReturnsShippedConstant(t *testing.T) {
	router := newDefaultCeilingTestRouter(t)

	rec := doDefaultCeilingRequest(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/budgets/default", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got costs.DefaultCeilingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.LimitSubcents != costs.DefaultCeilingSubcents {
		t.Errorf("LimitSubcents = %d, want the shipped constant %d", got.LimitSubcents, costs.DefaultCeilingSubcents)
	}
}

func TestSetDefaultCeiling_ThenGetReturnsWrittenValue(t *testing.T) {
	router := newDefaultCeilingTestRouter(t)

	putRec := doDefaultCeilingRequest(
		t, router, http.MethodPut, "/api/v1/workspaces/ws-1/budgets/default", `{"limit_subcents":750000}`,
	)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}

	getRec := doDefaultCeilingRequest(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/budgets/default", "")
	var got costs.DefaultCeilingResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.LimitSubcents != 750_000 {
		t.Errorf("LimitSubcents = %d, want 750000", got.LimitSubcents)
	}
}

// TestSetDefaultCeiling_RejectsNonPositiveLimit pins AC-OFFICE-BUDGET-003.8:
// a non-positive write is rejected with a validation error over HTTP.
func TestSetDefaultCeiling_RejectsNonPositiveLimit(t *testing.T) {
	router := newDefaultCeilingTestRouter(t)

	rec := doDefaultCeilingRequest(
		t, router, http.MethodPut, "/api/v1/workspaces/ws-1/budgets/default", `{"limit_subcents":0}`,
	)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// TestSetDefaultCeiling_DoesNotAppearInBudgetList pins AC-OFFICE-BUDGET-
// 003.7: writing the default never creates a office_budget_policies row,
// so it never shows up in the policy list API's results.
func TestSetDefaultCeiling_DoesNotAppearInBudgetList(t *testing.T) {
	router := newDefaultCeilingTestRouter(t)

	putRec := doDefaultCeilingRequest(
		t, router, http.MethodPut, "/api/v1/workspaces/ws-1/budgets/default", `{"limit_subcents":750000}`,
	)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}

	listRec := doDefaultCeilingRequest(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/budgets", "")
	var got costs.BudgetListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Budgets) != 0 {
		t.Errorf("Budgets = %+v, want empty: the default must never appear in the policy list", got.Budgets)
	}
}
