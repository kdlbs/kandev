package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	officemodels "github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestReadControlMetricsScopesCountsAndAnchorsTheHistoryWindow(t *testing.T) {
	repo := newTestRepo(t)
	db := repo.ReaderDB()
	createControlMetricsTables(t, db)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)

	insertControlStep(t, db, "step-work", "Build", "work", false)
	insertControlStep(t, db, "step-hold", "Hold", "hold", false)
	insertControlStep(t, db, "step-done", "Done", "completed", true)
	insertControlTask(t, db, "active", "workspace-a", "IN_PROGRESS", "step-work", false, "manual", "")
	insertControlTask(t, db, "blocked", "workspace-a", "BLOCKED", "step-hold", false, "manual", "")
	insertControlTask(t, db, "completed", "workspace-a", "COMPLETED", "step-done", false, "manual", "")
	insertControlTask(t, db, "archived", "workspace-a", "IN_PROGRESS", "step-work", false, "manual", "2026-11-08T00:00:00Z")
	insertControlTask(t, db, "ephemeral", "workspace-a", "IN_PROGRESS", "step-work", true, "manual", "")
	insertControlTask(t, db, "automation", "workspace-a", "IN_PROGRESS", "step-work", false, taskmodels.TaskOriginAutomationRun, "")
	insertControlTask(t, db, "foreign", "workspace-b", "IN_PROGRESS", "step-work", false, "manual", "")

	insertControlSession(t, db, "primary-old", "active", "FAILED", true, now.Add(-time.Hour))
	insertControlSession(t, db, "primary-a", "active", "RUNNING", true, now)
	insertControlSession(t, db, "primary-z", "active", "WAITING_FOR_INPUT", true, now)
	insertControlSession(t, db, "secondary", "active", "WAITING_FOR_INPUT", false, now)
	insertControlSession(t, db, "blocked-running", "blocked", "RUNNING", true, now)
	insertControlSession(t, db, "completed-primary", "completed", "WAITING_FOR_INPUT", true, now)
	insertControlSession(t, db, "archived-primary", "archived", "RUNNING", true, now)
	insertControlSession(t, db, "ephemeral-primary", "ephemeral", "RUNNING", true, now)
	insertControlSession(t, db, "automation-primary", "automation", "RUNNING", true, now)
	insertControlSession(t, db, "foreign-primary", "foreign", "RUNNING", true, now)

	for _, taskID := range []string{"active", "blocked", "archived", "ephemeral", "automation", "foreign"} {
		if _, err := db.Exec(`INSERT INTO queued_messages (task_id) VALUES (?)`, taskID); err != nil {
			t.Fatalf("insert queued message for %s: %v", taskID, err)
		}
	}
	insertControlTransition(t, db, "active", "step-done", since)
	insertControlTransition(t, db, "active", "step-done", now)
	insertControlTransition(t, db, "active", "step-done", since.Add(-time.Nanosecond))
	insertControlTransition(t, db, "active", "step-work", now.Add(time.Nanosecond))
	insertControlTransition(t, db, "archived", "step-done", now)
	insertControlTransition(t, db, "foreign", "step-done", now)
	insertControlTransition(t, db, "ephemeral", "step-done", now)
	insertControlTransition(t, db, "automation", "step-done", now)

	got, err := repo.ReadControlMetrics(ctx, "workspace-a", now)
	if err != nil {
		t.Fatalf("ReadControlMetrics: %v", err)
	}
	if got.WorkspaceID != "workspace-a" || !got.GeneratedAt.Equal(now) || !got.Since.Equal(since) {
		t.Fatalf("window = workspace %q, generated %s, since %s", got.WorkspaceID, got.GeneratedAt, got.Since)
	}
	if got.Basis != "recorded_task_session_queue_transition_state" || got.InferenceState != "unknown" {
		t.Fatalf("basis and inference = %q, %q", got.Basis, got.InferenceState)
	}
	if want := map[string]int{"IN_PROGRESS": 1, "BLOCKED": 1, "COMPLETED": 1}; !equalControlCounts(got.Tasks, want) {
		t.Errorf("task counts = %#v, want %#v", got.Tasks, want)
	}
	if got.WaitingInputTasks != 1 {
		t.Errorf("waiting input tasks = %d, want 1 from the latest primary session only", got.WaitingInputTasks)
	}
	if got.QueuedMessages != 2 {
		t.Errorf("queued messages = %d, want 2 for visible, unarchived tasks", got.QueuedMessages)
	}
	if got.Transitions24h != 3 {
		t.Errorf("transitions in the inclusive window = %d, want 3 including archived history", got.Transitions24h)
	}
	if got.CompletedTasks24h != 2 {
		t.Errorf("completed tasks in the window = %d, want 2 distinct tasks", got.CompletedTasks24h)
	}
	if got.Sessions["WAITING_FOR_INPUT"] != 3 || got.Sessions["RUNNING"] != 2 || got.Sessions["FAILED"] != 1 {
		t.Errorf("session counts = %#v", got.Sessions)
	}
	if len(got.TaskStages) != 3 {
		t.Errorf("task stage groups = %#v, want one group for each current task", got.TaskStages)
	}
}

func TestCountAgentInstancesByWorkspaceIDsBatchesAndFiltersRows(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	working := fullAgentInstance("agent-working", "workspace-a", "Working")
	working.Status = officemodels.AgentStatusWorking
	idle := fullAgentInstance("agent-idle", "workspace-a", "Idle")
	outside := fullAgentInstance("agent-outside", "workspace-outside", "Outside")
	for _, agent := range []*officemodels.AgentInstance{working, idle, outside} {
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent %s: %v", agent.ID, err)
		}
	}
	if _, err := repo.ReaderDB().Exec(`UPDATE agent_profiles SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?`, idle.ID); err != nil {
		t.Fatalf("soft-delete agent: %v", err)
	}

	workspaceIDs := make([]string, 401)
	for i := range workspaceIDs {
		workspaceIDs[i] = fmt.Sprintf("workspace-%03d", i)
	}
	workspaceIDs[len(workspaceIDs)-1] = "workspace-a"

	got, err := repo.CountAgentInstancesByWorkspaceIDs(ctx, workspaceIDs)
	if err != nil {
		t.Fatalf("CountAgentInstancesByWorkspaceIDs: %v", err)
	}
	if len(got) != 1 || got["workspace-a"] != (officemodels.AgentWorkspaceCounts{Total: 1, Running: 1}) {
		t.Fatalf("agent counts = %#v, want only the live workspace-a agent", got)
	}
}

func createControlMetricsTables(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE tasks (
			id TEXT PRIMARY KEY, workspace_id TEXT, state TEXT, is_ephemeral INTEGER,
			origin TEXT, workflow_step_id TEXT, archived_at TIMESTAMP
		)`,
		`CREATE TABLE workflow_steps (
			id TEXT PRIMARY KEY, name TEXT, stage_type TEXT, complete_task_on_enter INTEGER
		)`,
		`CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY, task_id TEXT, state TEXT, is_primary INTEGER, started_at TIMESTAMP
		)`,
		`CREATE TABLE queued_messages (task_id TEXT)`,
		`CREATE TABLE task_step_transitions (
			task_id TEXT, to_workflow_step_id TEXT, occurred_at TIMESTAMP
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create control metrics table: %v", err)
		}
	}
}

func insertControlStep(t *testing.T, db *sqlx.DB, id, name, stage string, completes bool) {
	t.Helper()
	complete := 0
	if completes {
		complete = 1
	}
	if _, err := db.Exec(`INSERT INTO workflow_steps (id,name,stage_type,complete_task_on_enter) VALUES (?,?,?,?)`, id, name, stage, complete); err != nil {
		t.Fatalf("insert workflow step %s: %v", id, err)
	}
}

func insertControlTask(t *testing.T, db *sqlx.DB, id, workspaceID, state, stepID string, ephemeral bool, origin, archivedAt string) {
	t.Helper()
	ephemeralValue := 0
	if ephemeral {
		ephemeralValue = 1
	}
	var archived any
	if archivedAt != "" {
		parsed, err := time.Parse(time.RFC3339, archivedAt)
		if err != nil {
			t.Fatalf("parse archive timestamp: %v", err)
		}
		archived = parsed
	}
	if _, err := db.Exec(`INSERT INTO tasks (id,workspace_id,state,is_ephemeral,origin,workflow_step_id,archived_at) VALUES (?,?,?,?,?,?,?)`, id, workspaceID, state, ephemeralValue, origin, stepID, archived); err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

func insertControlSession(t *testing.T, db *sqlx.DB, id, taskID, state string, primary bool, startedAt time.Time) {
	t.Helper()
	primaryValue := 0
	if primary {
		primaryValue = 1
	}
	if _, err := db.Exec(`INSERT INTO task_sessions (id,task_id,state,is_primary,started_at) VALUES (?,?,?,?,?)`, id, taskID, state, primaryValue, startedAt); err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
}

func insertControlTransition(t *testing.T, db *sqlx.DB, taskID, stepID string, occurredAt time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO task_step_transitions (task_id,to_workflow_step_id,occurred_at) VALUES (?,?,?)`, taskID, stepID, occurredAt); err != nil {
		t.Fatalf("insert transition for %s: %v", taskID, err)
	}
}

func equalControlCounts(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, count := range want {
		if got[key] != count {
			return false
		}
	}
	return true
}
