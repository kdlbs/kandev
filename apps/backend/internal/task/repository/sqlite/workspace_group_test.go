package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedWorkspaceGroupOwnerFixture creates the task_workspace_groups /
// task_workspace_group_members rows GetWorkspaceGroupOwnerTaskID reads. Those
// tables are owned by internal/office/repository/sqlite's schema init, which
// never runs against this package's own test DB (see
// TestCreateTaskSessionWithSharedGroupWorkspaceBindingElectsOneMaterializer for
// the same convention), so the fixture creates the columns the query actually
// touches directly.
func seedWorkspaceGroupOwnerFixture(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`
		CREATE TABLE IF NOT EXISTS task_workspace_groups (
			id TEXT PRIMARY KEY,
			owner_task_id TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS task_workspace_group_members (
			workspace_group_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			released_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			PRIMARY KEY (workspace_group_id, task_id)
		);
	`); err != nil {
		t.Fatalf("create workspace group tables: %v", err)
	}
}

func insertWorkspaceGroupMember(
	t *testing.T, repo *Repository, groupID, ownerTaskID, memberTaskID string, createdAt time.Time,
) {
	t.Helper()
	if _, err := repo.db.Exec(
		`INSERT OR IGNORE INTO task_workspace_groups (id, owner_task_id) VALUES (?, ?)`,
		groupID, ownerTaskID,
	); err != nil {
		t.Fatalf("seed workspace group %s: %v", groupID, err)
	}
	if _, err := repo.db.Exec(
		`INSERT INTO task_workspace_group_members (workspace_group_id, task_id, created_at) VALUES (?, ?, ?)`,
		groupID, memberTaskID, createdAt,
	); err != nil {
		t.Fatalf("seed workspace group member %s/%s: %v", groupID, memberTaskID, err)
	}
}

// TestGetWorkspaceGroupOwnerTaskIDPicksEarliestMembershipDeterministically
// covers the F3 finding from Review Round 1: the query's LIMIT 1 had no
// ORDER BY, so when a task is an (unreleased) member of more than one active
// workspace group, the redirect target picked by SQLite's query plan could
// change across a re-sync, breaking AC-2's requirement that the redirect
// target stay stable. Earliest membership (by created_at) must win, and the
// result must not depend on the order the groups happen to be inserted in.
func TestGetWorkspaceGroupOwnerTaskIDPicksEarliestMembershipDeterministically(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-group-owner")
	for _, taskID := range []string{"member1", "owner-early", "owner-late"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-group-owner", Title: taskID}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}
	seedWorkspaceGroupOwnerFixture(t, repo)

	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	// Insert the LATER membership first so a naive LIMIT-1-no-ORDER-BY query
	// (or one that happens to favor insertion/rowid order) would be tempted
	// to return the wrong owner.
	insertWorkspaceGroupMember(t, repo, "group-late", "owner-late", "member1", later)
	insertWorkspaceGroupMember(t, repo, "group-early", "owner-early", "member1", earlier)

	ownerTaskID, err := repo.GetWorkspaceGroupOwnerTaskID(ctx, "member1")
	if err != nil {
		t.Fatalf("GetWorkspaceGroupOwnerTaskID: %v", err)
	}
	if ownerTaskID != "owner-early" {
		t.Fatalf("owner task ID = %q, want owner-early (earliest membership)", ownerTaskID)
	}

	// Re-running must not flip the answer even though the physical row order
	// on disk hasn't changed — pins that the result is genuinely determined by
	// the ORDER BY clause, not incidental query-plan behavior.
	ownerTaskID2, err := repo.GetWorkspaceGroupOwnerTaskID(ctx, "member1")
	if err != nil {
		t.Fatalf("GetWorkspaceGroupOwnerTaskID (second run): %v", err)
	}
	if ownerTaskID2 != "owner-early" {
		t.Fatalf("second run: owner task ID = %q, want owner-early", ownerTaskID2)
	}
}

// TestGetWorkspaceGroupOwnerTaskIDIgnoresReleasedMembership pins the existing
// released_at filter alongside the new ordering, so the F3 fix didn't
// accidentally widen the query to include stale memberships.
func TestGetWorkspaceGroupOwnerTaskIDIgnoresReleasedMembership(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-group-owner-released")
	for _, taskID := range []string{"member1", "owner-active"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-group-owner-released", Title: taskID}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}
	seedWorkspaceGroupOwnerFixture(t, repo)

	now := time.Now().UTC()
	insertWorkspaceGroupMember(t, repo, "group-active", "owner-active", "member1", now)
	if _, err := repo.db.Exec(
		`UPDATE task_workspace_group_members SET released_at = ? WHERE workspace_group_id = 'group-active' AND task_id = 'member1'`,
		now,
	); err != nil {
		t.Fatalf("release membership: %v", err)
	}

	ownerTaskID, err := repo.GetWorkspaceGroupOwnerTaskID(ctx, "member1")
	if err != nil {
		t.Fatalf("GetWorkspaceGroupOwnerTaskID: %v", err)
	}
	if ownerTaskID != "" {
		t.Fatalf("owner task ID = %q, want empty (only membership is released)", ownerTaskID)
	}
}

func TestGetWorkspaceGroupOwnerTaskIDForSessionUsesBoundEnvironment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-group-session-owner")
	for _, taskID := range []string{"member1", "owner-early", "owner-late"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-group-session-owner", Title: taskID}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}
	seedWorkspaceGroupOwnerFixture(t, repo)
	if _, err := repo.db.Exec(`ALTER TABLE task_workspace_groups ADD COLUMN materialized_environment_id TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatalf("add materialized environment column: %v", err)
	}

	insertWorkspaceGroupMember(t, repo, "group-early", "owner-early", "member1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	insertWorkspaceGroupMember(t, repo, "group-late", "owner-late", "member1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if _, err := repo.db.Exec(`UPDATE task_workspace_groups SET materialized_environment_id = CASE id WHEN 'group-early' THEN 'env-early' ELSE 'env-late' END`); err != nil {
		t.Fatalf("seed materialized environments: %v", err)
	}
	if _, err := repo.db.Exec(`INSERT INTO task_sessions (id, task_id, task_environment_id, started_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"session-late", "member1", "env-late", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed bound session: %v", err)
	}

	resolver, ok := any(repo).(interface {
		GetWorkspaceGroupOwnerTaskIDForSession(context.Context, string, string) (string, error)
	})
	if !ok {
		t.Fatal("Repository does not implement session-bound workspace owner resolution")
	}
	ownerTaskID, err := resolver.GetWorkspaceGroupOwnerTaskIDForSession(ctx, "member1", "session-late")
	if err != nil {
		t.Fatalf("GetWorkspaceGroupOwnerTaskIDForSession: %v", err)
	}
	if ownerTaskID != "owner-late" {
		t.Fatalf("owner task ID = %q, want owner-late for the session-bound group", ownerTaskID)
	}
}
