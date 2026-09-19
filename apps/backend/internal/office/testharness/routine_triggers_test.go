package testharness

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newRoutineTriggerTestRouter wires the office repo (with a seeded routine)
// and a router mounting only the routine-triggers route.
func newRoutineTriggerTestRouter(t *testing.T) (*gin.Engine, *officesqlite.Repository, string) {
	t.Helper()
	taskRepo, sqlxDB := newTestRepo(t)
	officeRepo, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, logger.Default())
	if err != nil {
		t.Fatalf("new office repo: %v", err)
	}
	routine := &officemodels.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Seeded routine",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := officeRepo.CreateRoutine(t.Context(), routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, taskRepo, officeRepo, nil, nil, nil, logger.Default(), nil, nil)
	return router, officeRepo, routine.ID
}

func postSeedRoutineTrigger(t *testing.T, router *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_test/routine-triggers", bytes.NewReader(mustJSON(t, body)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestSeedRoutineTriggerDefaultsEnabledAndTimezone(t *testing.T) {
	router, officeRepo, routineID := newRoutineTriggerTestRouter(t)

	res := postSeedRoutineTrigger(t, router, map[string]any{
		"routine_id":      routineID,
		"kind":            "cron",
		"cron_expression": "0 9 * * *",
	})
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	triggers, err := officeRepo.ListTriggersByRoutineID(t.Context(), routineID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers))
	}
	if !triggers[0].Enabled {
		t.Fatalf("enabled defaulted to false, want true")
	}
	if triggers[0].Timezone != "UTC" {
		t.Fatalf("timezone defaulted to %q, want UTC", triggers[0].Timezone)
	}
}

// TestSeedRoutineTriggerBypassesCronValidation proves this route can produce
// the schedule states the public create-trigger endpoint cannot: an enabled
// cron trigger with an unparseable expression (trigger_invalid) and a
// disabled trigger (trigger_disabled).
func TestSeedRoutineTriggerBypassesCronValidation(t *testing.T) {
	router, officeRepo, routineID := newRoutineTriggerTestRouter(t)

	disabled := false
	res := postSeedRoutineTrigger(t, router, map[string]any{
		"routine_id":      routineID,
		"kind":            "cron",
		"cron_expression": "not a cron expression",
		"enabled":         disabled,
	})
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	triggers, err := officeRepo.ListTriggersByRoutineID(t.Context(), routineID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers))
	}
	got := triggers[0]
	if got.Enabled {
		t.Fatalf("enabled = true, want false")
	}
	if got.CronExpression != "not a cron expression" {
		t.Fatalf("cron_expression = %q, want the invalid literal preserved", got.CronExpression)
	}
}

func TestSeedRoutineTriggerPersistsNextRunAtAndLastFiredAt(t *testing.T) {
	router, officeRepo, routineID := newRoutineTriggerTestRouter(t)

	nextRunAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	lastFiredAt := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	res := postSeedRoutineTrigger(t, router, map[string]any{
		"routine_id":      routineID,
		"kind":            "cron",
		"cron_expression": "0 9 * * *",
		"next_run_at":     nextRunAt.Format(time.RFC3339),
		"last_fired_at":   lastFiredAt.Format(time.RFC3339),
	})
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	triggers, err := officeRepo.ListTriggersByRoutineID(t.Context(), routineID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	got := triggers[0]
	if got.NextRunAt == nil || !got.NextRunAt.Equal(nextRunAt) {
		t.Fatalf("next_run_at = %v, want %v", got.NextRunAt, nextRunAt)
	}
	if got.LastFiredAt == nil || !got.LastFiredAt.Equal(lastFiredAt) {
		t.Fatalf("last_fired_at = %v, want %v", got.LastFiredAt, lastFiredAt)
	}
}

func TestSeedRoutineTriggerRejectsMissingFields(t *testing.T) {
	router, _, routineID := newRoutineTriggerTestRouter(t)

	res := postSeedRoutineTrigger(t, router, map[string]any{"routine_id": routineID})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 when kind is missing", res.Code)
	}

	res = postSeedRoutineTrigger(t, router, map[string]any{"kind": "cron"})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 when routine_id is missing", res.Code)
	}
}

func TestSeedRoutineTriggerRejectsBadTimestamp(t *testing.T) {
	router, _, routineID := newRoutineTriggerTestRouter(t)

	res := postSeedRoutineTrigger(t, router, map[string]any{
		"routine_id":  routineID,
		"kind":        "cron",
		"next_run_at": "not-a-timestamp",
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for malformed next_run_at", res.Code)
	}
}
