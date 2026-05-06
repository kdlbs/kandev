package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// insertTestSession inserts a minimal task_sessions row for testing.
func insertTestSession(t *testing.T, deps *testDeps, sessionID, taskID, state, startedAt string, completedAt *string) {
	t.Helper()
	if completedAt != nil {
		_, err := deps.db.Exec(`
			INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, completed_at, updated_at)
			VALUES (?, ?, '', ?, ?, ?, ?)
		`, sessionID, taskID, state, startedAt, *completedAt, startedAt)
		if err != nil {
			t.Fatalf("insert session %s: %v", sessionID, err)
		}
	} else {
		_, err := deps.db.Exec(`
			INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
			VALUES (?, ?, '', ?, ?, ?)
		`, sessionID, taskID, state, startedAt, startedAt)
		if err != nil {
			t.Fatalf("insert session %s: %v", sessionID, err)
		}
	}
}

// createTaskSessionsTable ensures task_sessions exists in the test DB.
func createTaskSessionsTable(t *testing.T, deps *testDeps) {
	t.Helper()
	_, err := deps.db.Exec(`
		CREATE TABLE IF NOT EXISTS task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_execution_id TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT 'CREATED',
			started_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			updated_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create task_sessions: %v", err)
	}
}

// TestDashboard_RunActivity_14DayPadding verifies that run_activity always returns
// exactly 14 entries and gaps are padded with zeros.
func TestDashboard_RunActivity_14DayPadding(t *testing.T) {
	deps := newTestDeps(t)
	createTaskSessionsTable(t, deps)

	// Insert one task for the workspace.
	insertTestTask(t, deps.db, "task-ra", "ws-ra", "RA Task", "IN_PROGRESS", 0)

	// Insert two sessions: one completed 3 days ago, one failed yesterday.
	day3 := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02") + " 10:00:00"
	day1 := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + " 10:00:00"
	completedAt := day3
	insertTestSession(t, deps, "s1", "task-ra", "COMPLETED", day3, &completedAt)
	failedAt := day1
	insertTestSession(t, deps, "s2", "task-ra", "FAILED", day1, &failedAt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-ra/dashboard", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var activity []map[string]interface{}
	if err := json.Unmarshal(resp["run_activity"], &activity); err != nil {
		t.Fatalf("decode run_activity: %v", err)
	}

	if len(activity) != 14 {
		t.Errorf("run_activity length = %d, want 14", len(activity))
	}

	// Find day3 entry and verify succeeded=1.
	day3Date := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
	day1Date := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	var foundDay3, foundDay1 bool
	for _, entry := range activity {
		date, _ := entry["date"].(string)
		if date == day3Date {
			foundDay3 = true
			if v, _ := entry["succeeded"].(float64); v != 1 {
				t.Errorf("day3 succeeded = %v, want 1", entry["succeeded"])
			}
		}
		if date == day1Date {
			foundDay1 = true
			if v, _ := entry["failed"].(float64); v != 1 {
				t.Errorf("day1 failed = %v, want 1", entry["failed"])
			}
		}
	}
	if !foundDay3 {
		t.Errorf("day3 entry (%s) not found in run_activity", day3Date)
	}
	if !foundDay1 {
		t.Errorf("day1 entry (%s) not found in run_activity", day1Date)
	}
}

// TestDashboard_RunActivity_EmptyWhenNoSessions verifies that run_activity returns
// 14 zero-filled entries when there are no sessions.
func TestDashboard_RunActivity_EmptyWhenNoSessions(t *testing.T) {
	deps := newTestDeps(t)
	createTaskSessionsTable(t, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-empty-ra/dashboard", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var activity []map[string]interface{}
	if err := json.Unmarshal(resp["run_activity"], &activity); err != nil {
		t.Fatalf("decode run_activity: %v", err)
	}

	if len(activity) != 14 {
		t.Errorf("run_activity length = %d, want 14 (zero-padded)", len(activity))
	}
	for _, entry := range activity {
		if v, _ := entry["succeeded"].(float64); v != 0 {
			t.Errorf("expected 0 succeeded in empty workspace, got %v", v)
		}
	}
}

// TestDashboard_TaskBreakdown_StatusBucketing verifies that task_breakdown
// correctly buckets tasks by their state.
func TestDashboard_TaskBreakdown_StatusBucketing(t *testing.T) {
	deps := newTestDeps(t)

	ws := "ws-breakdown"
	insertTestTask(t, deps.db, "bd-todo1", ws, "Open 1", "TODO", 0)
	insertTestTask(t, deps.db, "bd-todo2", ws, "Open 2", "TODO", 0)
	insertTestTask(t, deps.db, "bd-ip", ws, "In Progress", "IN_PROGRESS", 0)
	insertTestTask(t, deps.db, "bd-blocked", ws, "Blocked", "BLOCKED", 0)
	insertTestTask(t, deps.db, "bd-done1", ws, "Done 1", "COMPLETED", 0)
	insertTestTask(t, deps.db, "bd-done2", ws, "Done 2", "COMPLETED", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+ws+"/dashboard", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var breakdown map[string]int
	if err := json.Unmarshal(resp["task_breakdown"], &breakdown); err != nil {
		t.Fatalf("decode task_breakdown: %v", err)
	}

	if breakdown["open"] != 2 {
		t.Errorf("open = %d, want 2", breakdown["open"])
	}
	if breakdown["in_progress"] != 1 {
		t.Errorf("in_progress = %d, want 1", breakdown["in_progress"])
	}
	if breakdown["blocked"] != 1 {
		t.Errorf("blocked = %d, want 1", breakdown["blocked"])
	}
	if breakdown["done"] != 2 {
		t.Errorf("done = %d, want 2", breakdown["done"])
	}
}

// TestDashboard_RecentTasks_ReturnsMostRecent verifies that recent_tasks returns
// the 10 most recently updated tasks.
func TestDashboard_RecentTasks_ReturnsMostRecent(t *testing.T) {
	deps := newTestDeps(t)

	ws := "ws-recent"
	// Insert 12 tasks; only 10 should appear.
	for i := 0; i < 12; i++ {
		id := "rt-task-" + string(rune('a'+i))
		_, err := deps.db.Exec(`
			INSERT INTO tasks (id, workspace_id, title, state, priority, identifier, created_at, updated_at)
			VALUES (?, ?, ?, 'TODO', 'medium', ?, datetime('now', '-' || ? || ' minutes'), datetime('now', '-' || ? || ' minutes'))
		`, id, ws, "Task "+string(rune('A'+i)), id, i, i)
		if err != nil {
			t.Fatalf("insert task %s: %v", id, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+ws+"/dashboard", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var recent []map[string]interface{}
	if err := json.Unmarshal(resp["recent_tasks"], &recent); err != nil {
		t.Fatalf("decode recent_tasks: %v", err)
	}

	if len(recent) != 10 {
		t.Errorf("recent_tasks length = %d, want 10", len(recent))
	}
	// Each entry should have required fields.
	for _, task := range recent {
		if task["id"] == nil || task["title"] == nil {
			t.Errorf("recent task missing required fields: %v", task)
		}
	}
}

// TestLiveRuns_ReturnsRunsForWorkspace verifies the live-runs endpoint returns
// sessions enriched with task context.
func TestLiveRuns_ReturnsRunsForWorkspace(t *testing.T) {
	deps := newTestDeps(t)
	createTaskSessionsTable(t, deps)

	ws := "ws-liveruns"
	insertTestTask(t, deps.db, "lr-task1", ws, "Live Task", "IN_PROGRESS", 0)
	startedAt := time.Now().UTC().Add(-5 * time.Minute).Format("2006-01-02T15:04:05Z")
	insertTestSession(t, deps, "lr-sess1", "lr-task1", "RUNNING", startedAt, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+ws+"/live-runs", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var runs []map[string]interface{}
	if err := json.Unmarshal(resp["runs"], &runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}

	if len(runs) != 1 {
		t.Fatalf("runs length = %d, want 1", len(runs))
	}
	run := runs[0]
	if run["taskId"] != "lr-task1" {
		t.Errorf("taskId = %v, want lr-task1", run["taskId"])
	}
	if run["status"] != "running" {
		t.Errorf("status = %v, want running", run["status"])
	}
	if run["taskTitle"] != "Live Task" {
		t.Errorf("taskTitle = %v, want 'Live Task'", run["taskTitle"])
	}
}

// TestLiveRuns_EmptyForWorkspaceWithNoSessions verifies that the endpoint
// returns an empty runs array when there are no sessions.
func TestLiveRuns_EmptyForWorkspaceWithNoSessions(t *testing.T) {
	deps := newTestDeps(t)
	createTaskSessionsTable(t, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-no-sessions/live-runs", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var runs []interface{}
	if err := json.Unmarshal(resp["runs"], &runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

// TestLiveRuns_StatusMapping verifies the session state → status mapping.
func TestLiveRuns_StatusMapping(t *testing.T) {
	deps := newTestDeps(t)
	createTaskSessionsTable(t, deps)

	ws := "ws-status-map"
	insertTestTask(t, deps.db, "sm-task1", ws, "Task 1", "COMPLETED", 0)
	insertTestTask(t, deps.db, "sm-task2", ws, "Task 2", "COMPLETED", 0)
	insertTestTask(t, deps.db, "sm-task3", ws, "Task 3", "FAILED", 0)

	now := time.Now().UTC()
	started := now.Add(-10 * time.Minute).Format("2006-01-02T15:04:05Z")
	finished := now.Add(-1 * time.Minute).Format("2006-01-02T15:04:05Z")

	insertTestSession(t, deps, "sm-s1", "sm-task1", "COMPLETED", started, &finished)
	insertTestSession(t, deps, "sm-s2", "sm-task2", "FAILED", started, &finished)
	insertTestSession(t, deps, "sm-s3", "sm-task3", "CANCELLED", started, &finished)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+ws+"/live-runs?limit=10", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var runs []map[string]interface{}
	if err := json.Unmarshal(resp["runs"], &runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}

	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	statusBySession := map[string]string{}
	for _, r := range runs {
		// Note: agentId in this test setup will be the agent_execution_id from session (empty string default)
		// We key by taskId instead.
		taskID, _ := r["taskId"].(string)
		status, _ := r["status"].(string)
		statusBySession[taskID] = status
	}
	if statusBySession["sm-task1"] != "completed" {
		t.Errorf("sm-task1 status = %q, want completed", statusBySession["sm-task1"])
	}
	if statusBySession["sm-task2"] != "failed" {
		t.Errorf("sm-task2 status = %q, want failed", statusBySession["sm-task2"])
	}
	if statusBySession["sm-task3"] != "cancelled" {
		t.Errorf("sm-task3 status = %q, want cancelled", statusBySession["sm-task3"])
	}
}
