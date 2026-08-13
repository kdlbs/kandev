package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// TestCutover_OrphanedSessionWorktreeUsesFlatOwner proves that a legacy row
// whose session was already deleted cannot override the surviving flat source
// for the same physical worktree.
func TestCutover_OrphanedSessionWorktreeUsesFlatOwner(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-orphan-flat", taskID: "task-orphan-flat", repoID: "repo-orphan-flat", sessionID: "sess-orphan-flat"}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-orphan-flat", "/tasks/orphan-flat/current", "feature/current", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-orphan-flat", seed.repoID, "",
		"/tasks/orphan-flat/stale", "feature/stale", "active", now)
	deleteLegacySession(t, db, seed.sessionID)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-orphan-flat" ||
		env.Repos[0].WorktreePath != "/tasks/orphan-flat/current" ||
		env.Repos[0].WorktreeBranch != "feature/current" {
		t.Fatalf("flat environment owner = %+v", env.Repos)
	}
	if legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("task_session_worktrees must be dropped after recovering the orphan")
	}
}

// TestCutover_OrphanedSessionWorktreeUsesCanonicalOwner proves that a
// non-deleted canonical row remains authoritative over stale orphan metadata.
func TestCutover_OrphanedSessionWorktreeUsesCanonicalOwner(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-orphan-canonical", taskID: "task-orphan-canonical", repoID: "repo-orphan-canonical", sessionID: "sess-orphan-canonical"}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "", "", "", now)
	seedLegacyEnvRepo(t, db, "env-repo-orphan-canonical", seed.envID, seed.repoID,
		"wt-orphan-canonical", "/tasks/orphan-canonical/current", "feature/current", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-orphan-canonical", seed.repoID, "",
		"/tasks/orphan-canonical/stale", "feature/stale", "active", now)
	deleteLegacySession(t, db, seed.sessionID)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-orphan-canonical" ||
		env.Repos[0].WorktreePath != "/tasks/orphan-canonical/current" ||
		env.Repos[0].WorktreeBranch != "feature/current" {
		t.Fatalf("canonical environment owner = %+v", env.Repos)
	}
}

// TestCutover_DeletedOrphanedSessionWorktreeIsHistory proves that a deleted
// legacy row needs no replacement owner when its session is already gone.
func TestCutover_DeletedOrphanedSessionWorktreeIsHistory(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{taskID: "task-orphan-deleted", sessionID: "sess-orphan-deleted"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-orphan-deleted", "repo-orphan-deleted", "",
		"/tasks/orphan-deleted/old", "feature/old", "deleted", now)
	deleteLegacySession(t, db, seed.sessionID)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("cutover: %v", err)
	}
	if legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("task_session_worktrees must be dropped after discarding deleted history")
	}
	var repoRows int
	if err := db.Get(&repoRows, `SELECT COUNT(*) FROM task_environment_repos`); err != nil {
		t.Fatalf("count normalized repository rows: %v", err)
	}
	if repoRows != 0 {
		t.Fatalf("normalized repository rows = %d, want 0", repoRows)
	}
}

// TestCutover_ActiveOrphanedSessionWorktreeFailsClosed proves that physical
// identity is required from a live task-owned source before an orphan is safe
// to discard.
func TestCutover_ActiveOrphanedSessionWorktreeFailsClosed(t *testing.T) {
	t.Run("unique physical identity", func(t *testing.T) {
		db := openLegacyDB(t)
		now := time.Now().UTC().Truncate(time.Second)
		seed := legacySeed{taskID: "task-orphan-unique", sessionID: "sess-orphan-unique"}
		seedLegacyTask(t, db, seed, now)
		seedLegacySessionWorktree(t, db, seed.sessionID, "wt-orphan-unique", "repo-orphan-unique", "",
			"/tasks/orphan-unique/repo", "feature/unique", "active", now)
		deleteLegacySession(t, db, seed.sessionID)

		assertOrphanCutoverFailsClosed(t, db, seed.sessionID, "wt-orphan-unique")
	})

	t.Run("collapsed flat source", func(t *testing.T) {
		db := openLegacyDB(t)
		now := time.Now().UTC().Truncate(time.Second)
		seed := legacySeed{envID: "env-orphan-winner", taskID: "task-orphan-loser", repoID: "repo-orphan-winner", sessionID: "sess-orphan-loser"}
		seedLegacyTask(t, db, seed, now)
		loser := seed
		loser.envID = "env-orphan-loser"
		loser.repoID = "repo-orphan-loser"
		seedLegacyFlatEnv(t, db, loser, "wt-orphan-loser", "/tasks/orphan-loser/old", "feature/old", now.Add(-time.Hour))
		seedLegacyFlatEnv(t, db, seed, "wt-orphan-winner", "/tasks/orphan-loser/current", "feature/current", now)
		seedLegacySessionWorktree(t, db, seed.sessionID, "wt-orphan-loser", loser.repoID, "",
			"/tasks/orphan-loser/old", "feature/old", "active", now)
		deleteLegacySession(t, db, seed.sessionID)

		assertOrphanCutoverFailsClosed(t, db, seed.sessionID, "wt-orphan-loser")
	})

	t.Run("deleted canonical source", func(t *testing.T) {
		db := openLegacyDB(t)
		now := time.Now().UTC().Truncate(time.Second)
		seed := legacySeed{envID: "env-orphan-deleted-owner", taskID: "task-orphan-deleted-owner", repoID: "repo-orphan-deleted-owner", sessionID: "sess-orphan-deleted-owner"}
		seedLegacyTask(t, db, seed, now)
		seedLegacyFlatEnv(t, db, seed, "", "", "", now)
		addLegacyRepoLifecycleColumns(t, db)
		seedLegacyEnvRepo(t, db, "env-repo-orphan-deleted-owner", seed.envID, seed.repoID,
			"wt-orphan-deleted-owner", "/tasks/orphan-deleted-owner/old", "feature/old", now)
		if _, err := db.Exec(db.Rebind(`
			UPDATE task_environment_repos
			SET status = 'deleted', deleted_at = ?
			WHERE id = 'env-repo-orphan-deleted-owner'`), now); err != nil {
			t.Fatalf("mark canonical repository row deleted: %v", err)
		}
		seedLegacySessionWorktree(t, db, seed.sessionID, "wt-orphan-deleted-owner", seed.repoID, "",
			"/tasks/orphan-deleted-owner/current", "feature/current", "active", now)
		deleteLegacySession(t, db, seed.sessionID)

		assertOrphanCutoverFailsClosed(t, db, seed.sessionID, "wt-orphan-deleted-owner")
	})
}

func deleteLegacySession(t *testing.T, db *sqlx.DB, sessionID string) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), sessionID); err != nil {
		t.Fatalf("delete legacy session: %v", err)
	}
}

func addLegacyRepoLifecycleColumns(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, statement := range []string{
		`ALTER TABLE task_environment_repos ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE task_environment_repos ADD COLUMN merged_at TIMESTAMP`,
		`ALTER TABLE task_environment_repos ADD COLUMN deleted_at TIMESTAMP`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("add legacy repository lifecycle column: %v", err)
		}
	}
}

func assertOrphanCutoverFailsClosed(t *testing.T, db *sqlx.DB, sessionID, worktreeID string) {
	t.Helper()
	if _, err := NewWithDB(db, db, nil); err == nil ||
		!strings.Contains(err.Error(), "references missing session "+sessionID) {
		t.Fatalf("cutover error = %v, want missing-session conflict", err)
	}
	if !legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("failed cutover must retain task_session_worktrees")
	}
	var rows int
	if err := db.Get(&rows, db.Rebind(
		`SELECT COUNT(*) FROM task_session_worktrees WHERE session_id = ? AND worktree_id = ?`),
		sessionID, worktreeID); err != nil {
		t.Fatalf("count retained orphan row: %v", err)
	}
	if rows != 1 {
		t.Fatalf("retained orphan rows = %d, want 1", rows)
	}
}
