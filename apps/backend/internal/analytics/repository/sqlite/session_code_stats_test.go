package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

func TestListSessionCodeStats_CommittedSumsOnly(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	// Two commits on the same session: 10+20 insertions, 2+3 deletions.
	execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES ('c-1', 'sess-1', 'sha1', ?, 2, 10, 2, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES ('c-2', 'sess-1', 'sha2', ?, 1, 20, 3, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	if stat.SessionID != "sess-1" {
		t.Errorf("SessionID: want sess-1, got %s", stat.SessionID)
	}
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 30 {
		t.Errorf("LinesAddedCommitted: want *30, got %v", stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 5 {
		t.Errorf("LinesDeletedCommitted: want *5, got %v", stat.LinesDeletedCommitted)
	}
	if stat.LinesAddedPeakPending != 0 {
		t.Errorf("LinesAddedPeakPending: want 0 (no snapshots), got %d", stat.LinesAddedPeakPending)
	}
	if stat.LinesDeletedPeakPending != 0 {
		t.Errorf("LinesDeletedPeakPending: want 0 (no snapshots), got %d", stat.LinesDeletedPeakPending)
	}
}

func TestListSessionCodeStats_PeakPendingWinsOverLatestSnapshot(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	// Earlier snapshot: large pending diff (the peak).
	earlier := now.Add(-time.Hour).Format(time.RFC3339)
	execOrFatal(t, dbConn, `INSERT INTO task_session_git_snapshots (id, session_id, snapshot_type, files, created_at) VALUES (?, 'sess-1', 'working_tree', ?, ?)`,
		"snap-early",
		`{"a.go":{"additions":100,"deletions":40},"b.go":{"additions":50,"deletions":10}}`,
		earlier,
	)
	// Latest snapshot: clean tree (as it usually is after commit/archive).
	execOrFatal(t, dbConn, `INSERT INTO task_session_git_snapshots (id, session_id, snapshot_type, files, created_at) VALUES (?, 'sess-1', 'working_tree', ?, ?)`,
		"snap-latest",
		`{}`,
		nowStr,
	)
	// A migrated environment-owned snapshot can lose capture provenance after
	// its session is deleted. It must not contribute to a per-session stat.
	execOrFatal(t, dbConn, `INSERT INTO task_session_git_snapshots (id, session_id, snapshot_type, files, created_at) VALUES (?, NULL, 'working_tree', ?, ?)`,
		"snap-no-provenance",
		`{"deleted-session.go":{"additions":999,"deletions":999}}`,
		nowStr,
	)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	// Peak snapshot (snap-early) has additions 100+50=150, deletions 40+10=50 —
	// the latest (clean) snapshot must NOT win just because it's newest.
	if stat.LinesAddedPeakPending != 150 {
		t.Errorf("LinesAddedPeakPending: want 150 (peak, not latest), got %d", stat.LinesAddedPeakPending)
	}
	if stat.LinesDeletedPeakPending != 50 {
		t.Errorf("LinesDeletedPeakPending: want 50 (peak, not latest), got %d", stat.LinesDeletedPeakPending)
	}
	// No commits, but the session started after commit-capture activation
	// (createTestDB seeds the marker at 2000-01-01) — committed sums are a
	// real *0, not nil, and are NOT conflated with pending.
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 0 {
		t.Errorf("LinesAddedCommitted: want *0, got %v", stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 0 {
		t.Errorf("LinesDeletedCommitted: want *0, got %v", stat.LinesDeletedCommitted)
	}
}

func TestListSessionCodeStats_CommittedAndPendingReportedSeparately(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES ('c-1', 'sess-1', 'sha1', ?, 1, 10, 4, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_git_snapshots (id, session_id, snapshot_type, files, created_at) VALUES (?, 'sess-1', 'working_tree', ?, ?)`,
		"snap-1",
		`{"a.go":{"additions":7,"deletions":1}}`,
		nowStr,
	)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	// Committed and pending must be reported as distinct fields, not summed
	// (10 committed + 7 pending must NOT collapse into 17 anywhere).
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 10 {
		t.Errorf("LinesAddedCommitted: want *10, got %v", stat.LinesAddedCommitted)
	}
	if stat.LinesAddedPeakPending != 7 {
		t.Errorf("LinesAddedPeakPending: want 7, got %d", stat.LinesAddedPeakPending)
	}
	if stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 4 {
		t.Errorf("LinesDeletedCommitted: want *4, got %v", stat.LinesDeletedCommitted)
	}
	if stat.LinesDeletedPeakPending != 1 {
		t.Errorf("LinesDeletedPeakPending: want 1, got %d", stat.LinesDeletedPeakPending)
	}
}

func TestListSessionCodeStats_NoCommitsNoSnapshotsReturnsZeros(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'CREATED', ?, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	// The session started after commit-capture activation (createTestDB seeds
	// the marker at 2000-01-01), so committed lines are a real *0, not nil.
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 0 ||
		stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 0 ||
		stat.LinesAddedPeakPending != 0 || stat.LinesDeletedPeakPending != 0 {
		t.Errorf("expected all-zero stats for session with no commits/snapshots, got %+v", stat)
	}
}

func TestListSessionCodeStats_EmptyFilterMatchesNoRows(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	results, err := repo.ListSessionCodeStats(context.Background(), models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 sessions on an empty database, got %d", len(results))
	}
}

func TestListSessionCodeStats_FiltersBySessionIDs(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-2', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{SessionIDs: []string{"sess-2"}})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	if results[0].SessionID != "sess-2" {
		t.Errorf("SessionID: want sess-2, got %s", results[0].SessionID)
	}
}

// TestListSessionCodeStats_ExcludesConfigModeTaskSessions proves
// ListSessionCodeStats applies the same office config-mode exclusion that
// Sessions().List applies (via fetchTasksForWorkspaces' excludeConfig=true):
// a session belonging to a config-mode task (internal bookkeeping, not a
// plugin-visible work item) must not appear in CodeStats, so the two Host
// data API session reads cover the same session set.
func TestListSessionCodeStats_ExcludesConfigModeTaskSessions(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, metadata, created_at, updated_at) VALUES ('task-normal', 'ws-1', 'board-1', '', 'Normal task', 0, '{}', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, metadata, created_at, updated_at) VALUES ('task-config', 'ws-1', 'board-1', '', 'Config task', 0, '{"config_mode":true}', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-normal', 'task-normal', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-config', 'task-config', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session (config-mode task's session excluded), got %d: %+v", len(results), results)
	}
	if results[0].SessionID != "sess-normal" {
		t.Errorf("SessionID: want sess-normal, got %s", results[0].SessionID)
	}
}

func TestListSessionCodeStats_FiltersByWorkspaceIDs(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-2', 'Test2', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-2', 'ws-2', 'Board2', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-2', 'ws-2', 'board-2', '', 'T2', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-2', 'task-2', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{WorkspaceIDs: []string{"ws-2"}})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	if results[0].SessionID != "sess-2" {
		t.Errorf("SessionID: want sess-2, got %s", results[0].SessionID)
	}
}

// legacyStartedAt predates createTestDB's seeded commit_capture_activated_at
// (2000-01-01T00:00:00Z).
const legacyStartedAt = "1999-01-01T00:00:00Z"

// Regression: a session that started before commit-capture activation, and
// has no observed commits, must report nil for committed lines — not 0.
// Capture wasn't running for this session, so "zero commits" and "unknown
// because nothing could have been captured yet" are indistinguishable; only
// nil is honest. Before this fix, COALESCE(..., 0) always claimed a real
// zero regardless of the activation marker (see migrateSessionCommitsDedupeAndActivation).
func TestListSessionCodeStats_LegacySessionReturnsNilCommittedLines(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-legacy', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, legacyStartedAt, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	if stat.LinesAddedCommitted != nil {
		t.Errorf("LinesAddedCommitted: want nil (legacy, predates commit-capture activation), got %d", *stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted != nil {
		t.Errorf("LinesDeletedCommitted: want nil (legacy, predates commit-capture activation), got %d", *stat.LinesDeletedCommitted)
	}
	// Peak-pending is unrelated to commit-capture activation and stays a real 0.
	if stat.LinesAddedPeakPending != 0 || stat.LinesDeletedPeakPending != 0 {
		t.Errorf("peak pending: want 0/0, got %d/%d", stat.LinesAddedPeakPending, stat.LinesDeletedPeakPending)
	}
}

// Regression companion: a session that started AFTER activation, with no
// commits, must still report a real *0 — capture was live for its entire
// lifetime, so zero commits is a trustworthy measurement, not an unknown.
func TestListSessionCodeStats_PostActivationSessionWithNoCommitsReturnsZeroNotNil(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-post', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 0 {
		t.Errorf("LinesAddedCommitted: want *0 (capture was active, genuinely zero commits), got %v", stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 0 {
		t.Errorf("LinesDeletedCommitted: want *0, got %v", stat.LinesDeletedCommitted)
	}
}

// Regression companion: a session that predates activation but DOES have an
// observed commit (captured by a later-activated writer, or backfilled) must
// report the real sum — nil applies only when there is truly nothing to
// distinguish from zero, not to every session older than the activation
// marker.
func TestListSessionCodeStats_LegacySessionWithCommitsReturnsRealSum(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-legacy-with-commit', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, legacyStartedAt, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES ('c-1', 'sess-legacy-with-commit', 'sha1', ?, 1, 10, 4, ?)`, nowStr, nowStr)

	results, err := repo.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	if err != nil {
		t.Fatalf("ListSessionCodeStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	stat := results[0]
	if stat.LinesAddedCommitted == nil || *stat.LinesAddedCommitted != 10 {
		t.Errorf("LinesAddedCommitted: want *10 (real observed commit, not nil), got %v", stat.LinesAddedCommitted)
	}
	if stat.LinesDeletedCommitted == nil || *stat.LinesDeletedCommitted != 4 {
		t.Errorf("LinesDeletedCommitted: want *4, got %v", stat.LinesDeletedCommitted)
	}
}
