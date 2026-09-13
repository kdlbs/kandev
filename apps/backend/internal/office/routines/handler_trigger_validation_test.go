package routines_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/routines"
)

// newTestRouter mounts routine routes over a fresh in-memory-SQLite-backed
// RoutineService.
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newTestRoutineService(t)
	router := gin.New()
	routines.RegisterRoutes(router.Group("/api/v1"), routines.NewHandler(svc))
	return router
}

func doRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createTestRoutineViaHTTP(t *testing.T, router *gin.Engine) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/routines", `{
		"name": "r1",
		"task_template": "{}"
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create routine status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp routines.RoutineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode routine response: %v", err)
	}
	return resp.Routine.ID
}

// TestCreateTrigger_InvalidCronExpression_Returns400 verifies a validation
// failure (unsatisfiable cron expression) is a client error, not a server
// error.
func TestCreateTrigger_InvalidCronExpression_Returns400(t *testing.T) {
	router := newTestRouter(t)
	routineID := createTestRoutineViaHTTP(t, router)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/routines/"+routineID+"/triggers", `{
		"kind": "cron",
		"cron_expression": "0 0 30 2 *"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestCreateTrigger_ValidCronExpression_Returns201 is the control case: a
// well-formed cron trigger still creates successfully.
func TestCreateTrigger_ValidCronExpression_Returns201(t *testing.T) {
	router := newTestRouter(t)
	routineID := createTestRoutineViaHTTP(t, router)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/routines/"+routineID+"/triggers", `{
		"kind": "cron",
		"cron_expression": "0 9 * * *"
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}
