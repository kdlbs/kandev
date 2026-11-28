package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestControlDashboardUsesTheAuthorizedWorkspaceRoute(t *testing.T) {
	deps := newTestDeps(t)
	ensureControlDashboardTables(t, deps.db)
	insertTestTask(t, deps.db, "control-task", "workspace-a", "Control task", "IN_PROGRESS", 2)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/office/workspaces/workspace-a/dashboard?view=control&window_hours=24",
		nil,
	)
	deps.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("control dashboard status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		WorkspaceID    string         `json:"workspace_id"`
		Tasks          map[string]int `json:"tasks"`
		InferenceState string         `json:"inference_state"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode control dashboard: %v", err)
	}
	if response.WorkspaceID != "workspace-a" || response.Tasks["IN_PROGRESS"] != 1 || response.InferenceState != "unknown" {
		t.Fatalf("control response = %+v", response)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/office/workspaces/workspace-a/dashboard?view=control&window_hours=12",
		nil,
	)
	deps.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unsupported window status = %d, want 400", recorder.Code)
	}
}

func ensureControlDashboardTables(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS queued_messages (task_id TEXT)`,
		`CREATE TABLE IF NOT EXISTS task_step_transitions (task_id TEXT, to_workflow_step_id TEXT, occurred_at TIMESTAMP)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create control dashboard table: %v", err)
		}
	}
	if !dashboardTableHasColumn(t, db, "task_sessions", "is_primary") {
		if _, err := db.Exec(`ALTER TABLE task_sessions ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0`); err != nil {
			t.Fatalf("add primary session flag: %v", err)
		}
	}
}

func dashboardTableHasColumn(t *testing.T, db *sqlx.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Queryx("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("read %s columns: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan %s column: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s columns: %v", table, err)
	}
	return false
}
