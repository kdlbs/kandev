package dashboard_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/dashboard"
)

// seedStatusChangeActivity inserts an office_activity_log row shaped like
// the one DashboardService.UpdateTaskStatus's real event-subscriber
// (handleTaskStatusChanged) would write in production. This test suite's
// DashboardService is built without an event bus (see newTestDeps), so
// calling UpdateTaskStatus directly here would not itself produce any
// activity rows; seed them explicitly instead, matching the pattern used
// by seedSummaryActivity in agent_summary_test.go.
func seedStatusChangeActivity(t *testing.T, db *sqlx.DB, taskID, wsID, newStatus string, at time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO office_activity_log
			(id, workspace_id, actor_type, actor_id, action,
			 target_type, target_id, details, run_id, session_id, created_at)
		VALUES (?, ?, 'system', 'office-scheduler', 'task_status_changed', 'task', ?, ?, '', '', ?)
	`, "act-"+taskID+"-"+at.Format("20060102150405"), wsID, taskID,
		fmt.Sprintf(`{"new_status":%q}`, newStatus), at.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed status-change activity for %s: %v", taskID, err)
	}
}

// TestGetTask_CompletedAtClearsWhenTaskReopened seeds a
// todo -> in_progress -> done -> in_progress activity history (the shape
// the office direct-update path writes) for a task currently in_progress,
// and asserts the sidebar no longer reports a completedAt once the task
// is reopened, even though it once completed. startedAt must survive the
// reopen unaffected.
func TestGetTask_CompletedAtClearsWhenTaskReopened(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "reopen-task", "ws-reopen", "Reopen Task", "in_progress", 0)

	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	seedStatusChangeActivity(t, deps.db, "reopen-task", "ws-reopen", "in_progress", base)
	seedStatusChangeActivity(t, deps.db, "reopen-task", "ws-reopen", "done", base.Add(time.Hour))
	seedStatusChangeActivity(t, deps.db, "reopen-task", "ws-reopen", "in_progress", base.Add(2*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/reopen-task", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Task == nil {
		t.Fatal("expected task in response")
	}
	if resp.Task.StartedAt == "" {
		t.Error("expected startedAt to survive the reopen")
	}
	if resp.Task.CompletedAt != "" {
		t.Errorf("expected completedAt cleared on a currently-open task, got %q", resp.Task.CompletedAt)
	}
}
