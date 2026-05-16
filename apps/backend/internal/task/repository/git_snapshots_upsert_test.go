package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedTaskAndSession creates the parent rows the git_snapshots foreign key
// requires (task → task_session). Returns the session ID.
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
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateStarting,
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

	// Create an agent_completed snapshot.
	completed := &models.GitSnapshot{
		SessionID:   sessionID,
		Branch:      "feature",
		HeadCommit:  "completed-head",
		TriggeredBy: "agent_completed",
		Metadata:    map[string]interface{}{"branch_additions": float64(5)},
	}
	if err := repo.CreateGitSnapshot(ctx, completed); err != nil {
		t.Fatalf("create agent_completed: %v", err)
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

	// Only agent_completed should remain.
	all, err = repo.GetGitSnapshotsBySession(ctx, sessionID, 0)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 snapshot after delete, got %d", len(all))
	}
	if all[0].TriggeredBy != "agent_completed" {
		t.Errorf("expected remaining snapshot to be agent_completed, got %q", all[0].TriggeredBy)
	}
}
