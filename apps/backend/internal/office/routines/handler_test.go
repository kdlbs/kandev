package routines

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newGatedTestRouter mirrors pause/handler_test.go's newPauseTestRouter, but
// for the routines package's own routes.
func newGatedTestRouter(svc *RoutineService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1/office")
	RegisterRoutes(group, NewHandler(svc))
	return r
}

func createGatedWebhookTrigger(t *testing.T, repo *sqlite.Repository, routineID, publicID string) *RoutineTrigger {
	t.Helper()
	trigger := &RoutineTrigger{
		RoutineID:   routineID,
		Kind:        "webhook",
		PublicID:    publicID,
		SigningMode: "none",
		Enabled:     true,
	}
	if err := repo.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create webhook trigger: %v", err)
	}
	return trigger
}

// TestFireWebhookTrigger_BlockedByPause_IncludesReasonAndWorkspaceID proves
// AC-OFFICE-KILL-SWITCH-002.3: a blocked webhook fire's 409 body must carry
// the pause reason (and, per the system design's general "blocked caller"
// HTTP contract, the workspace id) — not just paused:true.
func TestFireWebhookTrigger_BlockedByPause_IncludesReasonAndWorkspaceID(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	trigger := createGatedWebhookTrigger(t, repo, routine.ID, "wh-pause-1")

	gate := &fakePauseGate{active: []*models.WorkspacePause{
		{ID: "pause-1", WorkspaceID: "ws-1", Reason: "scheduled maintenance"},
	}}
	svc.SetPauseGate(gate)

	r := newGatedTestRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/routine-triggers/"+trigger.PublicID+"/fire",
		bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["paused"] != true {
		t.Errorf("paused = %v, want true", body["paused"])
	}
	if body["reason"] != "scheduled maintenance" {
		t.Errorf("reason = %v, want %q", body["reason"], "scheduled maintenance")
	}
	if body["workspace_id"] != "ws-1" {
		t.Errorf("workspace_id = %v, want ws-1", body["workspace_id"])
	}
}

// TestFireWebhookTrigger_PauseGateError_Returns503WithoutReason proves the
// gate-read-error branch is unchanged by the reason/workspace_id fix: it
// stays a 503 with no pause fields, since the pause state itself is unknown.
func TestFireWebhookTrigger_PauseGateError_Returns503WithoutReason(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	trigger := createGatedWebhookTrigger(t, repo, routine.ID, "wh-gate-error-1")

	gate := &fakePauseGate{errs: []error{context.DeadlineExceeded}}
	svc.SetPauseGate(gate)

	r := newGatedTestRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/routine-triggers/"+trigger.PublicID+"/fire",
		bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["reason"]; ok {
		t.Errorf("body = %v, want no reason field on a gate-error response", body)
	}
	if _, ok := body["paused"]; ok {
		t.Errorf("body = %v, want no paused field on a gate-error response", body)
	}
}

// TestRunRoutine_BlockedByPause_IncludesReasonAndWorkspaceID proves the
// manual-fire endpoint (AC-002.4, which imposes no reason requirement of its
// own) shares writeDispatchError with the webhook endpoint, so the fix
// applies uniformly rather than forking behavior per caller.
func TestRunRoutine_BlockedByPause_IncludesReasonAndWorkspaceID(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{active: []*models.WorkspacePause{
		{ID: "pause-1", WorkspaceID: "ws-1", Reason: "scheduled maintenance"},
	}}
	svc.SetPauseGate(gate)

	r := newGatedTestRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/routines/"+routine.ID+"/run",
		bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["reason"] != "scheduled maintenance" {
		t.Errorf("reason = %v, want %q", body["reason"], "scheduled maintenance")
	}
	if body["workspace_id"] != "ws-1" {
		t.Errorf("workspace_id = %v, want ws-1", body["workspace_id"])
	}
}
