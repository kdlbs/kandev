package routines

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
func patchRoutineTrigger(t *testing.T, router *gin.Engine, triggerID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/office/routine-triggers/"+triggerID,
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createGatedCronTrigger(
	t *testing.T, repo *sqlite.Repository, routineID, expression string, next, lastFired *time.Time,
) *RoutineTrigger {
	t.Helper()
	trigger := &RoutineTrigger{
		RoutineID:      routineID,
		Kind:           "cron",
		CronExpression: expression,
		Timezone:       "UTC",
		Enabled:        true,
		NextRunAt:      next,
		LastFiredAt:    lastFired,
	}
	if err := repo.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create cron trigger: %v", err)
	}
	return trigger
}

// A repeated enable must not re-arm a fire that is due but not yet claimed.
func TestUpdateRoutineTrigger_NoOpEnableKeepsPendingFire(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	due := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	trigger := createGatedCronTrigger(t, repo, routine.ID, "0 9 * * *", &due, nil)

	before, err := repo.GetRoutineTrigger(context.Background(), trigger.ID)
	if err != nil {
		t.Fatalf("get trigger: %v", err)
	}

	rec := patchRoutineTrigger(t, newGatedTestRouter(svc), trigger.ID, `{"enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp TriggerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Trigger.NextRunAt == nil || !resp.Trigger.NextRunAt.Equal(due) {
		t.Errorf("response next_run_at = %v, want the unchanged %v", resp.Trigger.NextRunAt, due)
	}

	got, err := repo.GetRoutineTrigger(context.Background(), trigger.ID)
	if err != nil {
		t.Fatalf("reload trigger: %v", err)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(due) {
		t.Fatalf("stored next_run_at = %v, want the unchanged %v", got.NextRunAt, due)
	}
	if !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("stored updated_at = %v, want the unchanged %v", got.UpdatedAt, before.UpdatedAt)
	}
}

// A real schedule edit rewrites next_run_at but leaves identity and history alone.
func TestUpdateRoutineTrigger_ScheduleEditKeepsIdentityAndHistory(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	due := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	lastFired := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	trigger := createGatedCronTrigger(t, repo, routine.ID, "0 9 * * *", &due, &lastFired)
	nextAfter := time.Now().UTC().Add(-time.Second)

	rec := patchRoutineTrigger(t, newGatedTestRouter(svc), trigger.ID, `{"cron_expression":"0 10 * * *"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp TriggerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Trigger.ID != trigger.ID {
		t.Errorf("trigger id = %q, want %q", resp.Trigger.ID, trigger.ID)
	}
	if !resp.Trigger.CreatedAt.Equal(trigger.CreatedAt) {
		t.Errorf("created_at = %v, want %v", resp.Trigger.CreatedAt, trigger.CreatedAt)
	}
	if resp.Trigger.LastFiredAt == nil || !resp.Trigger.LastFiredAt.Equal(lastFired) {
		t.Errorf("last_fired_at = %v, want %v", resp.Trigger.LastFiredAt, lastFired)
	}
	if resp.Trigger.CronExpression != "0 10 * * *" {
		t.Errorf("cron_expression = %q, want the edited expression", resp.Trigger.CronExpression)
	}
	if resp.Trigger.NextRunAt == nil || !resp.Trigger.NextRunAt.After(nextAfter) {
		t.Errorf("next_run_at = %v, want a recomputed future fire", resp.Trigger.NextRunAt)
	}
}

// An enabled cron trigger with no next fire is owned by a scheduler claim.
func TestUpdateRoutineTrigger_ClaimedTriggerConflicts(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	trigger := createGatedCronTrigger(t, repo, routine.ID, "0 9 * * *", nil, nil)

	rec := patchRoutineTrigger(t, newGatedTestRouter(svc), trigger.ID, `{"cron_expression":"0 10 * * *"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRoutineTrigger_InvalidCronReturns400(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	due := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	trigger := createGatedCronTrigger(t, repo, routine.ID, "0 9 * * *", &due, nil)

	rec := patchRoutineTrigger(t, newGatedTestRouter(svc), trigger.ID, `{"cron_expression":"0 0 30 2 *"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	got, err := repo.GetRoutineTrigger(context.Background(), trigger.ID)
	if err != nil {
		t.Fatalf("reload trigger: %v", err)
	}
	if got.CronExpression != "0 9 * * *" {
		t.Errorf("cron_expression = %q, want the stored expression", got.CronExpression)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(due) {
		t.Errorf("next_run_at = %v, want the stored %v", got.NextRunAt, due)
	}
}

func TestUpdateRoutineTrigger_WebhookToggle(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")
	trigger := createGatedWebhookTrigger(t, repo, routine.ID, "wh-update-toggle")

	rec := patchRoutineTrigger(t, newGatedTestRouter(svc), trigger.ID, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	got, err := repo.GetRoutineTrigger(context.Background(), trigger.ID)
	if err != nil {
		t.Fatalf("reload trigger: %v", err)
	}
	if got.Enabled {
		t.Error("trigger must be disabled")
	}
	if got.NextRunAt != nil {
		t.Errorf("next_run_at = %v, want nil after disabling", got.NextRunAt)
	}
}

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
