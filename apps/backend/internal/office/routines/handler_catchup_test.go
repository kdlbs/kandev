package routines_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routines"
)

// newTestRoutineRouter mounts the routine HTTP routes over a fresh
// RoutineService backed by in-memory SQLite.
func newTestRoutineRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newTestRoutineService(t)
	router := gin.New()
	routines.RegisterRoutes(router.Group("/api/v1"), routines.NewHandler(svc))
	return router
}

func doRoutineRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestCreateRoutineHandler_CatchUpMaxNormalizedInResponse covers
// AC-OFFICE-ROUTINE-CATCHUP-001.13: a routine read back through the API
// must report the same normalized catch_up_max the tick honours, not the
// raw value the client submitted.
func TestCreateRoutineHandler_CatchUpMaxNormalizedInResponse(t *testing.T) {
	router := newTestRoutineRouter(t)

	rec := doRoutineRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/routines",
		`{"name":"Handler routine","catch_up_max":5000,"variables":"{}"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	var resp routines.RoutineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Routine.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("response catch_up_max = %d, want %d (ceiling)", resp.Routine.CatchUpMax, models.CatchUpMaxCeiling)
	}
}

// TestUpdateRoutineHandler_CatchUpMaxNormalizedInResponse is the same
// contract on the PATCH path: submitting a below-floor value must come
// back normalized in the same response, not only on a subsequent GET.
func TestUpdateRoutineHandler_CatchUpMaxNormalizedInResponse(t *testing.T) {
	router := newTestRoutineRouter(t)

	createRec := doRoutineRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/routines",
		`{"name":"Handler routine","catch_up_max":25,"variables":"{}"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %s", createRec.Code, createRec.Body.String())
	}
	var created routines.RoutineResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	updateRec := doRoutineRequest(t, router, http.MethodPatch, "/api/v1/routines/"+created.Routine.ID,
		`{"catch_up_max":0}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %s", updateRec.Code, updateRec.Body.String())
	}
	var updated routines.RoutineResponse
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Routine.CatchUpMax != models.CatchUpMaxDefault {
		t.Errorf("response catch_up_max = %d, want %d (default floor)", updated.Routine.CatchUpMax, models.CatchUpMaxDefault)
	}

	getRec := doRoutineRequest(t, router, http.MethodGet, "/api/v1/routines/"+created.Routine.ID, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200, body = %s", getRec.Code, getRec.Body.String())
	}
	var fetched routines.RoutineResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched.Routine.CatchUpMax != models.CatchUpMaxDefault {
		t.Errorf("read-back catch_up_max = %d, want %d", fetched.Routine.CatchUpMax, models.CatchUpMaxDefault)
	}
}
