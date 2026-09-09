package routines_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routines"
)

// newTestRouter registers routine routes on a fresh gin engine backed by
// svc, so the tests below drive the real HTTP handlers rather than calling
// service methods directly — the 409 body shape is part of the contract
// (AC-OFFICE-ROUTINE-STATUS-004.4) and only the handler builds it.
func newTestRouter(svc *routines.RoutineService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1/office")
	routines.RegisterRoutes(group, routines.NewHandler(svc))
	return engine
}

type statusRefusalBody struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code"`
	Status    string `json:"status"`
}

func decodeStatusRefusal(t *testing.T, rec *httptest.ResponseRecorder) statusRefusalBody {
	t.Helper()
	var body statusRefusalBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return body
}

func TestRunRoutine_PausedRoutine_Returns409WithStatusCode(t *testing.T) {
	svc := newTestRoutineService(t)
	router := newTestRouter(svc)
	routine := createTestRoutine(t, svc, "HTTP Paused", "always_create")
	routine.Status = "paused"
	if err := svc.UpdateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("update routine: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/routines/"+routine.ID+"/run", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	body := decodeStatusRefusal(t, rec)
	if body.ErrorCode != "routine_not_firing" {
		t.Errorf("error_code = %q, want routine_not_firing", body.ErrorCode)
	}
	if body.Status != "paused" {
		t.Errorf("status field = %q, want paused", body.Status)
	}
	if body.Error == "" {
		t.Error("expected a non-empty human-readable error string")
	}

	runs, err := svc.ListRoutineRuns(context.Background(), routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected no run row, got %d", len(runs))
	}
}

func TestRunRoutine_MissingRoutine_StaysAt500(t *testing.T) {
	svc := newTestRoutineService(t)
	router := newTestRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/routines/does-not-exist/run", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d — a missing routine keeps its pre-existing mapping "+
			"(named out of scope), it must not become 409/404", rec.Code, http.StatusInternalServerError)
	}
}

func createWebhookTrigger(t *testing.T, svc *routines.RoutineService, routineID string) *models.RoutineTrigger {
	t.Helper()
	trigger := &models.RoutineTrigger{
		RoutineID:   routineID,
		Kind:        "webhook",
		PublicID:    "wh-" + routineID,
		SigningMode: "none",
		Enabled:     true,
	}
	if err := svc.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create webhook trigger: %v", err)
	}
	return trigger
}

func TestFireWebhookTrigger_PausedRoutine_Returns409WithStatusCode(t *testing.T) {
	svc := newTestRoutineService(t)
	router := newTestRouter(svc)
	routine := createTestRoutine(t, svc, "Webhook Paused", "always_create")
	routine.Status = "paused"
	if err := svc.UpdateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("update routine: %v", err)
	}
	trigger := createWebhookTrigger(t, svc, routine.ID)

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/office/routine-triggers/"+trigger.PublicID+"/fire", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	body := decodeStatusRefusal(t, rec)
	if body.ErrorCode != "routine_not_firing" {
		t.Errorf("error_code = %q, want routine_not_firing", body.ErrorCode)
	}
	if body.Status != "paused" {
		t.Errorf("status field = %q, want paused", body.Status)
	}
}

func TestFireWebhookTrigger_FiringRoutine_Returns200(t *testing.T) {
	svc := newTestRoutineService(t)
	router := newTestRouter(svc)
	routine := createTestRoutine(t, svc, "Webhook Firing", "always_create")
	trigger := createWebhookTrigger(t, svc, routine.ID)

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/office/routine-triggers/"+trigger.PublicID+"/fire", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestFireWebhookTrigger_BadSignature_RefusedRegardlessOfStatus covers
// AC-OFFICE-ROUTINE-STATUS-004.3: the signature check runs before the
// status check, so an unauthenticated caller cannot use the response code
// to probe a paused routine's status.
func TestFireWebhookTrigger_BadSignature_RefusedRegardlessOfStatus(t *testing.T) {
	svc := newTestRoutineService(t)
	router := newTestRouter(svc)
	routine := createTestRoutine(t, svc, "Webhook Bad Sig", "always_create")
	routine.Status = "paused"
	if err := svc.UpdateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("update routine: %v", err)
	}
	trigger := &models.RoutineTrigger{
		RoutineID:   routine.ID,
		Kind:        "webhook",
		PublicID:    "wh-badsig-" + routine.ID,
		SigningMode: "bearer",
		Secret:      "correct-secret",
		Enabled:     true,
	}
	if err := svc.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/office/routine-triggers/"+trigger.PublicID+"/fire", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (signature refusal must win over the status refusal); body=%s",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}
