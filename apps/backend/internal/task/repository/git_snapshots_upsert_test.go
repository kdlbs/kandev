package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedTaskAndSession creates the parent task, environment, and session rows
// required by the environment-owned git snapshot table.
func seedTaskAndSession(t *testing.T, ctx context.Context, repo *sqlite.Repository, taskID, sessionID string) {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Test Task",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	environmentID := "env-" + taskID
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                sessionID,
		TaskID:            taskID,
		TaskEnvironmentID: environmentID,
		AgentProfileID:    "profile-1",
		State:             models.TaskSessionStateStarting,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
}

// TestUpsertLatestLiveGitSnapshot covers the at-most-one-row invariant the
// sidebar diff badge cache depends on. See PR #556.
func TestUpsertLatestLiveGitSnapshot(t *testing.T) {
	ctx := context.Background()

	t.Run("two consecutive upserts keep exactly one live row", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		const taskID = "task-A"
		const sessionID = "session-A"
		seedTaskAndSession(t, ctx, repo, taskID, sessionID)

		first := &models.GitSnapshot{
			SessionID:  sessionID,
			Branch:     "main",
			HeadCommit: "abc123",
			BaseCommit: "base000",
			Ahead:      1,
			Behind:     0,
			Metadata:   map[string]interface{}{"branch_additions": 5, "branch_deletions": 1},
		}
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, first); err != nil {
			t.Fatalf("first upsert: %v", err)
		}

		second := &models.GitSnapshot{
			SessionID:  sessionID,
			Branch:     "main",
			HeadCommit: "def456",
			BaseCommit: "base000",
			Ahead:      2,
			Behind:     0,
			Metadata:   map[string]interface{}{"branch_additions": 9, "branch_deletions": 3},
		}
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, second); err != nil {
			t.Fatalf("second upsert: %v", err)
		}

		all, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 0)
		if err != nil {
			t.Fatalf("list snapshots: %v", err)
		}
		if len(all) != 1 {
			t.Fatalf("expected exactly 1 snapshot row, got %d", len(all))
		}
		got := all[0]
		if got.HeadCommit != "def456" {
			t.Errorf("expected head_commit=def456, got %q", got.HeadCommit)
		}
		if got.TriggeredBy != sqlite.TriggeredByLiveMonitor {
			t.Errorf("expected triggered_by=%q, got %q", sqlite.TriggeredByLiveMonitor, got.TriggeredBy)
		}
		if got.SnapshotType != models.SnapshotTypeStatusUpdate {
			t.Errorf("expected snapshot_type=%q, got %q", models.SnapshotTypeStatusUpdate, got.SnapshotType)
		}
		if got.Metadata["branch_additions"] != float64(9) {
			t.Errorf("expected branch_additions=9 in metadata, got %v", got.Metadata["branch_additions"])
		}
	})

	t.Run("non-live snapshots are untouched by upsert", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		const taskID = "task-B"
		const sessionID = "session-B"
		seedTaskAndSession(t, ctx, repo, taskID, sessionID)

		// Pre-existing archive snapshot — must survive subsequent live upserts.
		archive := &models.GitSnapshot{
			SessionID:    sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			Branch:       "feature",
			HeadCommit:   "archive-head",
			TriggeredBy:  "archive",
		}
		if err := repo.CreateGitSnapshot(ctx, archive); err != nil {
			t.Fatalf("create archive snapshot: %v", err)
		}

		live := &models.GitSnapshot{
			SessionID:  sessionID,
			Branch:     "feature",
			HeadCommit: "live-head-1",
		}
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, live); err != nil {
			t.Fatalf("first live upsert: %v", err)
		}

		live2 := &models.GitSnapshot{
			SessionID:  sessionID,
			Branch:     "feature",
			HeadCommit: "live-head-2",
		}
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, live2); err != nil {
			t.Fatalf("second live upsert: %v", err)
		}

		all, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 0)
		if err != nil {
			t.Fatalf("list snapshots: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("expected 2 rows (1 archive + 1 live), got %d", len(all))
		}

		var sawArchive, sawLive bool
		for _, snap := range all {
			switch snap.SnapshotType {
			case models.SnapshotTypeArchive:
				sawArchive = true
				if snap.HeadCommit != "archive-head" {
					t.Errorf("archive head_commit mutated: got %q", snap.HeadCommit)
				}
			case models.SnapshotTypeStatusUpdate:
				sawLive = true
				if snap.HeadCommit != "live-head-2" {
					t.Errorf("live head_commit not updated: got %q", snap.HeadCommit)
				}
				if snap.TriggeredBy != sqlite.TriggeredByLiveMonitor {
					t.Errorf("live triggered_by wrong: got %q", snap.TriggeredBy)
				}
			}
		}
		if !sawArchive {
			t.Error("archive snapshot disappeared after upsert")
		}
		if !sawLive {
			t.Error("live snapshot missing after upsert")
		}
	})

	t.Run("nil snapshot returns error", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		if err := repo.UpsertLatestLiveGitSnapshot(ctx, nil); err == nil {
			t.Fatal("expected error for nil snapshot, got nil")
		}
	})
}

func TestGetLatestGitSnapshot_PrefersAgentCompleted(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	const taskID = "task-pref"
	const sessionID = "session-pref"
	seedTaskAndSession(t, ctx, repo, taskID, sessionID)

	// Insert a live_monitor snapshot first (earlier timestamp).
	live := &models.GitSnapshot{
		SessionID:  sessionID,
		Branch:     "feature",
		HeadCommit: "live-head",
		Metadata:   map[string]interface{}{"branch_additions": float64(0), "branch_deletions": float64(0)},
	}
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, live); err != nil {
		t.Fatalf("upsert live: %v", err)
	}

	// Insert an agent_completed snapshot (later timestamp, better data).
	completed := &models.GitSnapshot{
		SessionID:   sessionID,
		Branch:      "feature",
		HeadCommit:  "completed-head",
		TriggeredBy: "agent_completed",
		Metadata:    map[string]interface{}{"branch_additions": float64(5), "branch_deletions": float64(2)},
	}
	if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
		t.Fatalf("create agent_completed: %v", err)
	}

	// GetLatestGitSnapshot should return the agent_completed snapshot.
	got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.HeadCommit != "completed-head" {
		t.Errorf("expected completed-head, got %q", got.HeadCommit)
	}
	if got.TriggeredBy != "agent_completed" {
		t.Errorf("expected triggered_by=agent_completed, got %q", got.TriggeredBy)
	}
}

func TestGetLatestGitSnapshot_PrefersAgentCompletedOverNewerLiveMonitor(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	const taskID = "task-race"
	const sessionID = "session-race"
	seedTaskAndSession(t, ctx, repo, taskID, sessionID)

	// Insert agent_completed snapshot first (older timestamp).
	completed := &models.GitSnapshot{
		SessionID:   sessionID,
		Branch:      "feature",
		HeadCommit:  "completed-head",
		TriggeredBy: "agent_completed",
		Metadata:    map[string]interface{}{"branch_additions": float64(10), "branch_deletions": float64(3)},
	}
	if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
		t.Fatalf("create agent_completed: %v", err)
	}

	// Insert a live_monitor snapshot AFTER (newer timestamp, stale data).
	// This simulates a poll racing with agent completion.
	live := &models.GitSnapshot{
		SessionID:  sessionID,
		Branch:     "feature",
		HeadCommit: "live-head-stale",
		Metadata:   map[string]interface{}{"branch_additions": float64(0), "branch_deletions": float64(0)},
	}
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, live); err != nil {
		t.Fatalf("upsert live: %v", err)
	}

	// agent_completed should still win despite having an older timestamp.
	got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if got.HeadCommit != "completed-head" {
		t.Errorf("expected completed-head, got %q — live_monitor snapshot incorrectly won", got.HeadCommit)
	}
}

// TestGetLatestGitSnapshot_PrefersArchiveOverAgentCompleted guards the
// terminal-replay invariant: while a task is still archived, its archive
// snapshot (final cumulative diff captured at archive time) must be selected
// over any agent_completed row. Before the ordering fix, GetLatestGitSnapshot
// preferred triggered_by='agent_completed' regardless of snapshot_type, so
// archived tasks that had completed a turn were replayed from a stale
// live-status row and the archive snapshot (with the cumulative-diff files
// carrying `path`) was never surfaced. See round-2 review blocker on
// d18c1a312.
func TestGetLatestGitSnapshot_PrefersArchiveOverAgentCompleted(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	const taskID = "task-arch"
	const sessionID = "session-arch"
	seedTaskAndSession(t, ctx, repo, taskID, sessionID)

	// The task must be archived for the archive snapshot to be authoritative;
	// an unarchived task's archive row is stale (see
	// TestGetLatestGitSnapshot_UnarchivedResumePrefersCompleted).
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	// An agent_completed row exists from the last completed turn (older).
	completed := &models.GitSnapshot{
		SessionID:   sessionID,
		Branch:      "feature",
		HeadCommit:  "completed-head",
		TriggeredBy: "agent_completed",
		Metadata:    map[string]interface{}{"branch_additions": float64(5), "branch_deletions": float64(2)},
	}
	if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
		t.Fatalf("create agent_completed: %v", err)
	}

	// The archive snapshot carries the final cumulative diff (with path fields).
	archive := &models.GitSnapshot{
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeArchive,
		Branch:       "feature",
		HeadCommit:   "archive-head",
		BaseCommit:   "base000",
		Files: map[string]interface{}{
			"file.txt": map[string]interface{}{
				"path":      "file.txt",
				"status":    "modified",
				"additions": float64(3),
				"deletions": float64(1),
			},
		},
	}
	if err := repo.CreateGitSnapshot(ctx, archive); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	t.Run("single query", func(t *testing.T) {
		got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetLatestGitSnapshot: %v", err)
		}
		if got.HeadCommit != "archive-head" {
			t.Errorf("expected archive-head, got %q — archive snapshot shadowed by agent_completed row", got.HeadCommit)
		}
		if got.SnapshotType != models.SnapshotTypeArchive {
			t.Errorf("expected snapshot_type=archive, got %q", got.SnapshotType)
		}
	})

	t.Run("batch query", func(t *testing.T) {
		got, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
		}
		if snap := got[sessionID]; snap == nil {
			t.Fatal("expected a snapshot for session, got none")
		} else if snap.HeadCommit != "archive-head" {
			t.Errorf("expected archive-head, got %q — batch query shadowed by agent_completed row", snap.HeadCommit)
		}
	})
}

// TestGetLatestGitSnapshot_UnarchivedBeforeResumeKeepsArchive guards the
// unarchive-before-resume window: httpUnarchiveTask / UnarchiveTaskTree only
// clear archived_at and restore branches/workspaces — no snapshot is written
// until the session is resumed and the agent completes another turn. During
// that window the terminal archive row must remain authoritative; demoting it
// below an older agent_completed row would make a reload or task-list fallback
// show pre-archive files and diff counts. The archive row loses rank-0 only
// once a non-archive snapshot newer than it exists. Codex review P1 on PR
// #2851.
func TestGetLatestGitSnapshot_UnarchivedBeforeResumeKeepsArchive(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	const taskID = "task-unarchive-pending"
	const sessionID = "session-unarchive-pending"
	seedTaskAndSession(t, ctx, repo, taskID, sessionID)

	// Older agent_completed row from a turn before the archive.
	completed := &models.GitSnapshot{
		SessionID:   sessionID,
		Branch:      "feature",
		HeadCommit:  "completed-before-archive",
		TriggeredBy: "agent_completed",
	}
	if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
		t.Fatalf("create agent_completed: %v", err)
	}

	// Task is archived; the terminal archive snapshot is captured.
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	archive := &models.GitSnapshot{
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeArchive,
		Branch:       "feature",
		HeadCommit:   "archive-head",
		BaseCommit:   "base000",
		Files: map[string]interface{}{
			"file.txt": map[string]interface{}{
				"path":      "file.txt",
				"status":    "modified",
				"additions": float64(3),
				"deletions": float64(1),
			},
		},
	}
	if err := repo.CreateGitSnapshot(ctx, archive); err != nil {
		t.Fatalf("create archive: %v", err)
	}

	// Task is unarchived but NOT yet resumed: no newer snapshot exists.
	if _, err := repo.UnarchiveTask(ctx, taskID); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}

	// The archive row must still win — it is the newest snapshot and the
	// only terminal state; the older agent_completed row is pre-archive.
	t.Run("single query", func(t *testing.T) {
		got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetLatestGitSnapshot: %v", err)
		}
		if got.HeadCommit != "archive-head" {
			t.Errorf("expected archive-head, got %q — pre-archive agent_completed row outranks terminal archive before resume", got.HeadCommit)
		}
	})
	t.Run("batch query", func(t *testing.T) {
		got, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
		}
		if snap := got[sessionID]; snap == nil {
			t.Fatal("expected a snapshot for session, got none")
		} else if snap.HeadCommit != "archive-head" {
			t.Errorf("expected archive-head in batch, got %q", snap.HeadCommit)
		}
	})
}

// TestGetLatestGitSnapshot_UnarchivedResumePrefersCompleted guards the
// lifecycle-conditional archive ranking: an archive snapshot is authoritative
// only while its owning task is still archived. ArchiveTask captures one, but
// UnarchiveTask does not invalidate it, and an archive-cancelled session
// resumes on the SAME session ID (task_operations.go: resume-reason path for
// archive-cancelled sessions). Once the task is unarchived and the agent runs
// again, newer agent_completed / live_monitor rows describe the current
// execution and must win over the stale archive row — otherwise the DB-fallback
// status and the sidebar badge (loadTaskGitObservations) show the pre-resume
// state forever. Round-3 review blocker on 62bd42f3a.
func TestGetLatestGitSnapshot_UnarchivedResumePrefersCompleted(t *testing.T) {
	ctx := context.Background()

	t.Run("completed row after unarchive wins over older archive row", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		const taskID = "task-resume"
		const sessionID = "session-resume"
		seedTaskAndSession(t, ctx, repo, taskID, sessionID)

		// Task is archived; the archive snapshot is captured.
		if err := repo.ArchiveTask(ctx, taskID); err != nil {
			t.Fatalf("archive task: %v", err)
		}
		archive := &models.GitSnapshot{
			SessionID:    sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			Branch:       "feature",
			HeadCommit:   "archive-head",
			BaseCommit:   "base000",
		}
		if err := repo.CreateGitSnapshot(ctx, archive); err != nil {
			t.Fatalf("create archive: %v", err)
		}

		// Task is unarchived and resumes; the agent completes a turn and
		// writes a newer agent_completed row.
		if _, err := repo.UnarchiveTask(ctx, taskID); err != nil {
			t.Fatalf("unarchive task: %v", err)
		}
		completed := &models.GitSnapshot{
			SessionID:   sessionID,
			Branch:      "feature",
			HeadCommit:  "completed-after-resume",
			TriggeredBy: "agent_completed",
			Metadata:    map[string]interface{}{"branch_additions": float64(7), "branch_deletions": float64(3)},
		}
		if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
			t.Fatalf("create agent_completed: %v", err)
		}

		// The resumed agent_completed row must win — the archive row is stale.
		t.Run("single query", func(t *testing.T) {
			got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
			if err != nil {
				t.Fatalf("GetLatestGitSnapshot: %v", err)
			}
			if got.HeadCommit != "completed-after-resume" {
				t.Errorf("expected completed-after-resume, got %q — stale archive row outranks resumed execution", got.HeadCommit)
			}
		})

		t.Run("batch query", func(t *testing.T) {
			got, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{sessionID})
			if err != nil {
				t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
			}
			if snap := got[sessionID]; snap == nil {
				t.Fatal("expected a snapshot for session, got none")
			} else if snap.HeadCommit != "completed-after-resume" {
				t.Errorf("expected completed-after-resume, got %q — batch query served stale archive row", snap.HeadCommit)
			}
		})
	})

	t.Run("live_monitor row after unarchive wins over older archive row", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		const taskID = "task-resume-live"
		const sessionID = "session-resume-live"
		seedTaskAndSession(t, ctx, repo, taskID, sessionID)

		if err := repo.ArchiveTask(ctx, taskID); err != nil {
			t.Fatalf("archive task: %v", err)
		}
		archive := &models.GitSnapshot{
			SessionID:    sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			Branch:       "feature",
			HeadCommit:   "archive-head",
		}
		if err := repo.CreateGitSnapshot(ctx, archive); err != nil {
			t.Fatalf("create archive: %v", err)
		}

		if _, err := repo.UnarchiveTask(ctx, taskID); err != nil {
			t.Fatalf("unarchive task: %v", err)
		}
		live := &models.GitSnapshot{
			SessionID:  sessionID,
			Branch:     "feature",
			HeadCommit: "live-after-resume",
		}
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, live); err != nil {
			t.Fatalf("upsert live: %v", err)
		}

		got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetLatestGitSnapshot: %v", err)
		}
		if got.HeadCommit != "live-after-resume" {
			t.Errorf("expected live-after-resume, got %q — stale archive row outranks resumed live status", got.HeadCommit)
		}

		// The batch selector has a separate ROW_NUMBER() expression; it must
		// agree for the rank-2 live-monitor vs rank-3 stale-archive case too.
		gotBatch, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
		}
		if snap := gotBatch[sessionID]; snap == nil {
			t.Fatal("expected a snapshot for session, got none")
		} else if snap.HeadCommit != "live-after-resume" {
			t.Errorf("expected live-after-resume in batch query, got %q — batch served stale archive row", snap.HeadCommit)
		}
	})

	t.Run("archive still wins while the task remains archived", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		const taskID = "task-still-archived"
		const sessionID = "session-still-archived"
		seedTaskAndSession(t, ctx, repo, taskID, sessionID)

		if err := repo.ArchiveTask(ctx, taskID); err != nil {
			t.Fatalf("archive task: %v", err)
		}
		archive := &models.GitSnapshot{
			SessionID:    sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			Branch:       "feature",
			HeadCommit:   "archive-head",
		}
		if err := repo.CreateGitSnapshot(ctx, archive); err != nil {
			t.Fatalf("create archive: %v", err)
		}
		// A turn completed before the archive; the archive must still win.
		completed := &models.GitSnapshot{
			SessionID:   sessionID,
			Branch:      "feature",
			HeadCommit:  "completed-before-archive",
			TriggeredBy: "agent_completed",
		}
		if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
			t.Fatalf("create agent_completed: %v", err)
		}

		got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetLatestGitSnapshot: %v", err)
		}
		if got.HeadCommit != "archive-head" {
			t.Errorf("expected archive-head, got %q — archive lost while task is still archived", got.HeadCommit)
		}
	})

	t.Run("current live row beats pre-archive completion row", func(t *testing.T) {
		repo, cleanup := createTestSQLiteRepo(t)
		defer cleanup()

		const taskID = "task-resume-generation"
		const sessionID = "session-resume-generation"
		seedTaskAndSession(t, ctx, repo, taskID, sessionID)
		base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

		if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
			SessionID:   sessionID,
			HeadCommit:  "pre-archive-completed",
			TriggeredBy: "agent_completed",
			CreatedAt:   base,
		}); err != nil {
			t.Fatalf("create pre-archive completion: %v", err)
		}
		if err := repo.ArchiveTask(ctx, taskID); err != nil {
			t.Fatalf("archive task: %v", err)
		}
		if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
			SessionID:    sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			HeadCommit:   "archive-head",
			CreatedAt:    base.Add(time.Minute),
		}); err != nil {
			t.Fatalf("create archive: %v", err)
		}
		if _, err := repo.UnarchiveTask(ctx, taskID); err != nil {
			t.Fatalf("unarchive task: %v", err)
		}
		if err := repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
			SessionID:  sessionID,
			HeadCommit: "live-after-resume",
			CreatedAt:  base.Add(2 * time.Minute),
		}); err != nil {
			t.Fatalf("create resumed live snapshot: %v", err)
		}

		got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetLatestGitSnapshot: %v", err)
		}
		if got.HeadCommit != "live-after-resume" {
			t.Errorf("head commit = %q, want current live row instead of pre-archive completion", got.HeadCommit)
		}

		gotBatch, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
		}
		if gotBatch[sessionID] == nil || gotBatch[sessionID].HeadCommit != "live-after-resume" {
			t.Errorf("batch head commit = %v, want current live row", gotBatch[sessionID])
		}
	})
}

// TestGetLatestGitSnapshot_TieBreakerIsDeterministic guards the equal-rank
// ordering: Postgres stores created_at at microsecond precision, so distinct
// writes (e.g. two archive rows from re-archive flows) can collapse to the
// same timestamp. Both selectors must break the tie identically via the ID
// column so the selected row is never database-plan dependent.
func TestGetLatestGitSnapshot_TieBreakerIsDeterministic(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	const taskID = "task-tie"
	const sessionID = "session-tie"
	seedTaskAndSession(t, ctx, repo, taskID, sessionID)
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	at := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	for _, snap := range []*models.GitSnapshot{
		{ID: "tie-aaa", SessionID: sessionID, SnapshotType: models.SnapshotTypeArchive,
			Branch: "feature", HeadCommit: "older-id", CreatedAt: at},
		{ID: "tie-bbb", SessionID: sessionID, SnapshotType: models.SnapshotTypeArchive,
			Branch: "feature", HeadCommit: "newer-id", CreatedAt: at},
	} {
		if err := repo.CreateGitSnapshot(ctx, snap); err != nil {
			t.Fatalf("create snapshot %s: %v", snap.ID, err)
		}
	}

	// Both rows tie on created_at and rank; the id DESC tie-breaker picks
	// tie-bbb deterministically in both selectors.
	t.Run("single query", func(t *testing.T) {
		got, err := repo.GetLatestGitSnapshot(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetLatestGitSnapshot: %v", err)
		}
		if got.ID != "tie-bbb" {
			t.Errorf("expected tie-bbb (id DESC tie-break), got %q", got.ID)
		}
	})
	t.Run("batch query", func(t *testing.T) {
		got, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
		}
		if snap := got[sessionID]; snap == nil || snap.ID != "tie-bbb" {
			t.Errorf("expected tie-bbb in batch, got %+v", got[sessionID])
		}
	})
}

func TestDeleteLiveMonitorSnapshots(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	const taskID = "task-del"
	const sessionID = "session-del"
	seedTaskAndSession(t, ctx, repo, taskID, sessionID)

	// Create a live_monitor snapshot.
	live := &models.GitSnapshot{
		SessionID:  sessionID,
		Branch:     "feature",
		HeadCommit: "live-head",
		Metadata:   map[string]interface{}{"branch_additions": float64(1)},
	}
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, live); err != nil {
		t.Fatalf("upsert live: %v", err)
	}

	// Create a non-live snapshot. An agent_completed write intentionally
	// supersedes the live row in the same environment, so it cannot be used to
	// exercise DeleteLiveMonitorSnapshots directly.
	completed := &models.GitSnapshot{
		SessionID:    sessionID,
		SnapshotType: models.SnapshotTypeArchive,
		Branch:       "feature",
		HeadCommit:   "completed-head",
		TriggeredBy:  "archive",
		Metadata:     map[string]interface{}{"branch_additions": float64(5)},
	}
	if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
		t.Fatalf("create non-live snapshot: %v", err)
	}

	// Verify both exist.
	all, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 0)
	if err != nil {
		t.Fatalf("list before delete: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 snapshots before delete, got %d", len(all))
	}

	// Delete live_monitor snapshots.
	if err := repo.DeleteLiveMonitorSnapshots(ctx, sessionID); err != nil {
		t.Fatalf("DeleteLiveMonitorSnapshots: %v", err)
	}

	// Only the non-live snapshot should remain.
	all, err = repo.GetGitSnapshotsBySession(ctx, sessionID, 0)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 snapshot after delete, got %d", len(all))
	}
	if all[0].TriggeredBy != "archive" {
		t.Errorf("expected remaining snapshot to be archive, got %q", all[0].TriggeredBy)
	}
}
