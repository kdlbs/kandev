package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// ensureSessionTables creates the task_sessions / task_session_messages
// stubs the dashboard aggregate queries read. Column types mirror the
// production schema in internal/task/repository/sqlite/base_schema.go so
// the driver's TIMESTAMP↔string conversion behaves the same way here.
func ensureSessionTables(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_execution_id TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT,
			state TEXT NOT NULL DEFAULT 'CREATED',
			started_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			updated_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS task_session_messages (
			id TEXT PRIMARY KEY,
			task_session_id TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'message',
			author_type TEXT NOT NULL DEFAULT 'user',
			content TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := repo.ExecRaw(context.Background(), s); err != nil {
			t.Fatalf("create session test schema: %v", err)
		}
	}
}

// dayStamp renders a SQLite-storable timestamp `offsetDays` before `base`,
// at the given hour. Returned in the format mattn/go-sqlite3 parses back
// into a UTC time.Time.
//
// The base time is passed in rather than read here so a test's seed rows and
// its assertions come from one clock read: these queries bucket by calendar
// day, so a UTC midnight crossing between seeding and asserting would
// otherwise name a different day and fail with no code defect.
func dayStamp(base time.Time, offsetDays, hour int) string {
	d := base.AddDate(0, 0, -offsetDays)
	return fmt.Sprintf("%s %02d:00:00", d.Format("2006-01-02"), hour)
}

func dayDate(base time.Time, offsetDays int) string {
	return base.AddDate(0, 0, -offsetDays).Format("2006-01-02")
}

func TestQueryRunActivity_BucketsOutcomesPerDay(t *testing.T) {
	repo := newSearchTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()
	base := time.Now().UTC()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, is_ephemeral, origin, created_at, updated_at) VALUES
			('t-main',  'ws-1', 'main',  0, 'manual',         datetime('now'), datetime('now')),
			('t-eph',   'ws-1', 'eph',   1, 'manual',         datetime('now'), datetime('now')),
			('t-auto',  'ws-1', 'auto',  0, 'automation_run', datetime('now'), datetime('now')),
			('t-other', 'ws-2', 'other', 0, 'manual',         datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at) VALUES
			('s-today-ok',    't-main',  'COMPLETED', ?, ?),
			('s-today-fail',  't-main',  'FAILED',    ?, ?),
			('s-today-other', 't-main',  'RUNNING',   ?, ?),
			('s-yday-ok',     't-main',  'COMPLETED', ?, ?),
			('s-old',         't-main',  'COMPLETED', ?, ?),
			('s-eph',         't-eph',   'COMPLETED', ?, ?),
			('s-auto',        't-auto',  'COMPLETED', ?, ?),
			('s-otherws',     't-other', 'COMPLETED', ?, ?)
	`,
		dayStamp(base, 0, 10), dayStamp(base, 0, 10),
		dayStamp(base, 0, 11), dayStamp(base, 0, 11),
		dayStamp(base, 0, 12), dayStamp(base, 0, 12),
		dayStamp(base, 1, 9), dayStamp(base, 1, 9),
		dayStamp(base, 60, 9), dayStamp(base, 60, 9),
		dayStamp(base, 0, 13), dayStamp(base, 0, 13),
		dayStamp(base, 0, 14), dayStamp(base, 0, 14),
		dayStamp(base, 0, 15), dayStamp(base, 0, 15),
	); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	rows, err := repo.QueryRunActivity(ctx, "ws-1", 7)
	if err != nil {
		t.Fatalf("QueryRunActivity: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (today + yesterday): %+v", len(rows), rows)
	}
	// ORDER BY date — oldest first.
	if rows[0].Date != dayDate(base, 1) {
		t.Errorf("rows[0].Date = %q, want yesterday %q (ascending order)", rows[0].Date, dayDate(base, 1))
	}
	if rows[0] != (sqlite.RunActivityRow{Date: dayDate(base, 1), Succeeded: 1}) {
		t.Errorf("yesterday = %+v, want 1 succeeded only", rows[0])
	}
	want := sqlite.RunActivityRow{Date: dayDate(base, 0), Succeeded: 1, Failed: 1, Other: 1}
	if rows[1] != want {
		t.Errorf("today = %+v, want %+v", rows[1], want)
	}
}

func TestQueryRunActivity_EmptyWorkspace(t *testing.T) {
	repo := newSearchTestRepo(t)
	ensureSessionTables(t, repo)

	rows, err := repo.QueryRunActivity(context.Background(), "ws-nothing", 7)
	if err != nil {
		t.Fatalf("QueryRunActivity: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none", rows)
	}
}

func TestQueryTaskBreakdown_GroupsByStateAndFilters(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, state, title, is_ephemeral, origin, archived_at, created_at, updated_at) VALUES
			('b-todo1', 'ws-1', 'TODO',        'a', 0, 'manual',         NULL, datetime('now'), datetime('now')),
			('b-todo2', 'ws-1', 'TODO',        'b', 0, 'manual',         NULL, datetime('now'), datetime('now')),
			('b-done',  'ws-1', 'COMPLETED',   'c', 0, 'manual',         NULL, datetime('now'), datetime('now')),
			('b-eph',   'ws-1', 'TODO',        'd', 1, 'manual',         NULL, datetime('now'), datetime('now')),
			('b-auto',  'ws-1', 'TODO',        'e', 0, 'automation_run', NULL, datetime('now'), datetime('now')),
			('b-arch',  'ws-1', 'TODO',        'f', 0, 'manual',         '2026-01-01 00:00:00', datetime('now'), datetime('now')),
			('b-other', 'ws-2', 'TODO',        'g', 0, 'manual',         NULL, datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	rows, err := repo.QueryTaskBreakdown(ctx, "ws-1")
	if err != nil {
		t.Fatalf("QueryTaskBreakdown: %v", err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.State] = row.Count
	}
	if len(counts) != 2 {
		t.Fatalf("state buckets = %+v, want TODO + COMPLETED only", counts)
	}
	if counts["TODO"] != 2 {
		t.Errorf("TODO = %d, want 2 (ephemeral/automation/archived/other-workspace excluded)", counts["TODO"])
	}
	if counts["COMPLETED"] != 1 {
		t.Errorf("COMPLETED = %d, want 1", counts["COMPLETED"])
	}
}

func TestQueryRecentTasks_OrdersByUpdatedAtDescAndLimits(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks
			(id, workspace_id, workflow_step_id, identifier, title, state, is_ephemeral, origin, created_at, updated_at)
		VALUES
			('r-old', 'ws-1', 'step-r', 'KAN-1', 'Oldest',   'TODO',      0, 'manual', '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
			('r-mid', 'ws-1', 'step-r', 'KAN-2', 'Middle',   'BLOCKED',   0, 'manual', '2026-01-02 00:00:00', '2026-01-02 00:00:00'),
			('r-new', 'ws-1', 'step-r', 'KAN-3', 'Newest',   'COMPLETED', 0, 'manual', '2026-01-03 00:00:00', '2026-01-03 00:00:00'),
			('r-arc', 'ws-1', 'step-r', 'KAN-4', 'Archived', 'TODO',      0, 'manual', '2026-01-04 00:00:00', '2026-01-04 00:00:00')
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET archived_at = '2026-01-05 00:00:00' WHERE id = 'r-arc'`); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('p-new', 'step-r', 'r-new', 'runner', 'agent-runner', 0, 0)
	`); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	rows, err := repo.QueryRecentTasks(ctx, "ws-1", 2)
	if err != nil {
		t.Fatalf("QueryRecentTasks: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (limit): %+v", len(rows), rows)
	}
	if rows[0].ID != "r-new" || rows[1].ID != "r-mid" {
		t.Fatalf("order = %q,%q, want r-new,r-mid (updated_at DESC, archived excluded)", rows[0].ID, rows[1].ID)
	}
	if rows[0].Identifier != "KAN-3" {
		t.Errorf("Identifier = %q, want KAN-3", rows[0].Identifier)
	}
	if rows[0].Title != "Newest" {
		t.Errorf("Title = %q, want Newest", rows[0].Title)
	}
	if rows[0].State != "COMPLETED" {
		t.Errorf("State = %q, want COMPLETED", rows[0].State)
	}
	if rows[0].AssigneeAgentProfileID != "agent-runner" {
		t.Errorf("assignee = %q, want agent-runner from the runner projection", rows[0].AssigneeAgentProfileID)
	}
	if rows[0].UpdatedAt == "" {
		t.Error("UpdatedAt is empty, want the persisted timestamp")
	}
	// The task with no runner row projects to the empty string, not NULL.
	if rows[1].AssigneeAgentProfileID != "" {
		t.Errorf("unassigned task assignee = %q, want empty", rows[1].AssigneeAgentProfileID)
	}
}

func TestQueryRecentSessions_ScopesByWorkspaceAndOrders(t *testing.T) {
	repo := newSearchTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, is_ephemeral, origin, created_at, updated_at) VALUES
			('t-live',  'ws-1', 'live',  0, 'manual',         datetime('now'), datetime('now')),
			('t-eph',   'ws-1', 'eph',   1, 'manual',         datetime('now'), datetime('now')),
			('t-auto',  'ws-1', 'auto',  0, 'automation_run', datetime('now'), datetime('now')),
			('t-other', 'ws-2', 'other', 0, 'manual',         datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_sessions
			(id, task_id, agent_execution_id, state, started_at, completed_at, updated_at)
		VALUES
			('s-1', 't-live',  'exec-1', 'COMPLETED', '2026-01-01 00:00:00', '2026-01-01 01:00:00', '2026-01-01 01:00:00'),
			('s-2', 't-live',  '',       'RUNNING',   '2026-01-02 00:00:00', NULL,                  '2026-01-02 00:00:00'),
			('s-3', 't-eph',   'exec-3', 'COMPLETED', '2026-01-03 00:00:00', NULL,                  '2026-01-03 00:00:00'),
			('s-4', 't-auto',  'exec-4', 'COMPLETED', '2026-01-04 00:00:00', NULL,                  '2026-01-04 00:00:00'),
			('s-5', 't-other', 'exec-5', 'COMPLETED', '2026-01-05 00:00:00', NULL,                  '2026-01-05 00:00:00')
	`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	rows, err := repo.QueryRecentSessions(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("QueryRecentSessions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (ephemeral / automation / other workspace excluded): %+v", len(rows), rows)
	}
	if rows[0].ID != "s-2" || rows[1].ID != "s-1" {
		t.Fatalf("order = %q,%q, want s-2,s-1 (started_at DESC)", rows[0].ID, rows[1].ID)
	}
	if rows[0].TaskID != "t-live" {
		t.Errorf("TaskID = %q, want t-live", rows[0].TaskID)
	}
	if rows[0].AgentExecID != "" {
		t.Errorf("AgentExecID = %q, want empty for the NULL-ish column", rows[0].AgentExecID)
	}
	if rows[0].State != "RUNNING" {
		t.Errorf("State = %q, want RUNNING", rows[0].State)
	}
	if rows[0].StartedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("StartedAt = %q, want 2026-01-02T00:00:00Z", rows[0].StartedAt)
	}
	if rows[0].CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil for an in-flight session", *rows[0].CompletedAt)
	}
	if rows[1].AgentExecID != "exec-1" {
		t.Errorf("AgentExecID = %q, want exec-1", rows[1].AgentExecID)
	}
	if rows[1].CompletedAt == nil || *rows[1].CompletedAt != "2026-01-01T01:00:00Z" {
		t.Errorf("CompletedAt = %v, want 2026-01-01T01:00:00Z", rows[1].CompletedAt)
	}
}

func TestQueryRecentSessions_RespectsLimit(t *testing.T) {
	repo := newSearchTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('t-lim', 'ws-1', 'lim', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 't-lim', 'COMPLETED', ?, ?)
		`, fmt.Sprintf("s-%d", i),
			fmt.Sprintf("2026-01-0%d 00:00:00", i+1),
			fmt.Sprintf("2026-01-0%d 00:00:00", i+1)); err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}

	rows, err := repo.QueryRecentSessions(ctx, "ws-1", 3)
	if err != nil {
		t.Fatalf("QueryRecentSessions: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].ID != "s-4" {
		t.Errorf("newest = %q, want s-4", rows[0].ID)
	}
}

func TestGetTasksByIDs(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, identifier, created_at, updated_at) VALUES
			('g-1', 'ws-1', 'First',  'KAN-1', datetime('now'), datetime('now')),
			('g-2', 'ws-1', 'Second', 'KAN-2', datetime('now'), datetime('now')),
			('g-3', 'ws-2', 'Third',  '',      datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	rows, err := repo.GetTasksByIDs(ctx, []string{"g-1", "g-3", "missing"})
	if err != nil {
		t.Fatalf("GetTasksByIDs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (missing id silently omitted): %+v", len(rows), rows)
	}
	byID := map[string]sqlite.TaskTitleRow{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if byID["g-1"].Title != "First" || byID["g-1"].Identifier != "KAN-1" {
		t.Errorf("g-1 = %+v, want title First / identifier KAN-1", byID["g-1"])
	}
	// Workspace is not part of the filter — the enrichment lookup is by id only.
	if byID["g-3"].Title != "Third" || byID["g-3"].Identifier != "" {
		t.Errorf("g-3 = %+v, want title Third / empty identifier", byID["g-3"])
	}
}

func TestGetTasksByIDs_EmptyInput(t *testing.T) {
	repo := newSearchTestRepo(t)

	rows, err := repo.GetTasksByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetTasksByIDs(nil): %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none", rows)
	}
}

func TestListRecentSessionsByAgentBatch_PartitionsPerAgent(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	// agent-a has 3 sessions, agent-b has 1, agent-c has none.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_sessions
			(id, task_id, agent_profile_id, state, started_at, completed_at, updated_at)
		VALUES
			('a-1', 't-1', 'agent-a', 'COMPLETED', '2026-01-01 00:00:00', '2026-01-01 02:00:00', '2026-01-01 02:00:00'),
			('a-2', 't-1', 'agent-a', 'FAILED',    '2026-01-02 00:00:00', NULL,                  '2026-01-02 03:00:00'),
			('a-3', 't-2', 'agent-a', 'IDLE',      '2026-01-03 00:00:00', NULL,                  '2026-01-03 04:00:00'),
			('b-1', 't-3', 'agent-b', 'COMPLETED', '2026-01-04 00:00:00', NULL,                  '2026-01-04 05:00:00'),
			('z-1', 't-4', 'agent-z', 'COMPLETED', '2026-01-05 00:00:00', NULL,                  '2026-01-05 06:00:00')
	`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	out, err := repo.ListRecentSessionsByAgentBatch(ctx, []string{"agent-a", "agent-b", "agent-c"}, 2)
	if err != nil {
		t.Fatalf("ListRecentSessionsByAgentBatch: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("map keys = %d, want 2 (agent-c has no rows, agent-z not requested): %+v", len(out), out)
	}
	if _, ok := out["agent-z"]; ok {
		t.Error("agent-z leaked into the result despite not being requested")
	}
	a := out["agent-a"]
	if len(a) != 2 {
		t.Fatalf("agent-a rows = %d, want 2 (perAgentLimit)", len(a))
	}
	if a[0].ID != "a-3" || a[1].ID != "a-2" {
		t.Fatalf("agent-a order = %q,%q, want a-3,a-2 (started_at DESC)", a[0].ID, a[1].ID)
	}
	if a[0].TaskID != "t-2" {
		t.Errorf("TaskID = %q, want t-2", a[0].TaskID)
	}
	if a[0].AgentProfileID != "agent-a" {
		t.Errorf("AgentProfileID = %q, want agent-a", a[0].AgentProfileID)
	}
	if a[0].State != "IDLE" {
		t.Errorf("State = %q, want IDLE", a[0].State)
	}
	if a[0].StartedAt != "2026-01-03T00:00:00Z" {
		t.Errorf("StartedAt = %q, want 2026-01-03T00:00:00Z", a[0].StartedAt)
	}
	if a[0].UpdatedAt != "2026-01-03T04:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-01-03T04:00:00Z — office IDLE sessions rely on it", a[0].UpdatedAt)
	}
	if a[0].CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", *a[0].CompletedAt)
	}
	if a[1].CompletedAt != nil {
		t.Errorf("a-2 CompletedAt = %v, want nil", *a[1].CompletedAt)
	}
	if b := out["agent-b"]; len(b) != 1 || b[0].ID != "b-1" {
		t.Errorf("agent-b = %+v, want just b-1", b)
	}
}

func TestListRecentSessionsByAgentBatch_DefaultsAndEmptyInput(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	out, err := repo.ListRecentSessionsByAgentBatch(ctx, nil, 5)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if out == nil {
		t.Fatal("returned nil map, want empty non-nil")
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}

	// A non-positive per-agent limit falls back to 5.
	for i := 0; i < 7; i++ {
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
			VALUES (?, 't-1', 'agent-d', 'COMPLETED', ?, ?)
		`, fmt.Sprintf("d-%d", i),
			fmt.Sprintf("2026-01-%02d 00:00:00", i+1),
			fmt.Sprintf("2026-01-%02d 00:00:00", i+1)); err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}
	out, err = repo.ListRecentSessionsByAgentBatch(ctx, []string{"agent-d"}, 0)
	if err != nil {
		t.Fatalf("zero limit: %v", err)
	}
	if len(out["agent-d"]) != 5 {
		t.Errorf("agent-d rows = %d, want the default 5", len(out["agent-d"]))
	}
}

func TestCountToolCallMessagesBySession(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, type, created_at) VALUES
			('m-1', 's-1', 'tool_call', datetime('now')),
			('m-2', 's-1', 'tool_call', datetime('now')),
			('m-3', 's-1', 'message',   datetime('now')),
			('m-4', 's-2', 'message',   datetime('now')),
			('m-5', 's-3', 'tool_call', datetime('now'))
	`); err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"s-1", "s-2", "s-missing"})
	if err != nil {
		t.Fatalf("CountToolCallMessagesBySession: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("map = %+v, want only s-1 (zero-tool-call sessions omitted)", got)
	}
	if got["s-1"] != 2 {
		t.Errorf("s-1 = %d, want 2", got["s-1"])
	}
	if _, ok := got["s-3"]; ok {
		t.Error("s-3 leaked in despite not being requested")
	}
}

func TestCountToolCallMessagesBySession_EmptyInput(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)

	got, err := repo.CountToolCallMessagesBySession(context.Background(), nil)
	if err != nil {
		t.Fatalf("CountToolCallMessagesBySession(nil): %v", err)
	}
	if got == nil {
		t.Fatal("returned nil map, want empty non-nil")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestBucketTaskBreakdown(t *testing.T) {
	got := sqlite.BucketTaskBreakdown([]sqlite.TaskBreakdownRow{
		{State: "COMPLETED", Count: 3},
		{State: "IN_PROGRESS", Count: 2},
		{State: "SCHEDULING", Count: 1},
		{State: "BLOCKED", Count: 4},
		{State: "TODO", Count: 5},
		{State: "", Count: 6},
		{State: "REVIEW", Count: 7},
	})
	want := models.TaskBreakdown{Done: 3, InProgress: 3, Blocked: 4, Open: 18}
	if got != want {
		t.Errorf("BucketTaskBreakdown = %+v, want %+v", got, want)
	}
}

func TestBucketTaskBreakdown_EmptyInput(t *testing.T) {
	if got := sqlite.BucketTaskBreakdown(nil); got != (models.TaskBreakdown{}) {
		t.Errorf("BucketTaskBreakdown(nil) = %+v, want zero value", got)
	}
}
