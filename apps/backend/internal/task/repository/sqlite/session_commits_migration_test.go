package sqlite

import (
	"context"
	"testing"
	"time"
)

// TestSessionCommitsDedupeAndActivationMigration mirrors
// TestTaskExternalIDMigrationAddsColumnsAndIndex: it reproduces the
// pre-migration state a legacy database can be in (duplicate
// (session_id, commit_sha) rows, no unique index, no activation marker),
// replays the migration twice, and asserts the end state is correct and
// idempotent. See migrateSessionCommitsDedupeAndActivation.
func TestSessionCommitsDedupeAndActivationMigration(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForGit(t, repo, "task-commits-migration", "session-commits-migration")

	// Simulate a legacy database: no unique index yet, and a duplicate
	// (session_id, commit_sha) pair that a plain INSERT (pre-ON CONFLICT)
	// could have produced, e.g. via a re-run archive capture.
	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS uniq_session_commits_session_sha`); err != nil {
		t.Fatalf("drop unique index to simulate legacy schema: %v", err)
	}

	earlier := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	insertRawCommit(t, repo, "commit-earliest", "session-commits-migration", "dup-sha", earlier)
	insertRawCommit(t, repo, "commit-latest", "session-commits-migration", "dup-sha", later)
	insertRawCommit(t, repo, "commit-unique", "session-commits-migration", "solo-sha", later)

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`, "session-commits-migration"); got != 3 {
		t.Fatalf("pre-migration commit rows = %d, want 3", got)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	var indexName string
	if err := repo.db.Get(&indexName, `
		SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'uniq_session_commits_session_sha'
	`); err != nil {
		t.Fatalf("uniq_session_commits_session_sha index is missing: %v", err)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ? AND commit_sha = ?`,
		"session-commits-migration", "dup-sha"); got != 1 {
		t.Fatalf("duplicate (session_id, commit_sha) rows after migration = %d, want 1", got)
	}

	survivor, err := repo.GetLatestSessionCommit(ctx, "session-commits-migration")
	if err != nil {
		t.Fatalf("GetLatestSessionCommit: %v", err)
	}
	if survivor.ID != "commit-unique" {
		// commit-unique has the later committed_at among the two distinct
		// SHAs, so it should sort first; the dedupe survivor for dup-sha is
		// checked directly below.
		t.Errorf("GetLatestSessionCommit = %q, want commit-unique", survivor.ID)
	}

	var survivorID string
	if err := repo.db.Get(&survivorID, repo.db.Rebind(`
		SELECT id FROM task_session_commits WHERE session_id = ? AND commit_sha = ?
	`), "session-commits-migration", "dup-sha"); err != nil {
		t.Fatalf("select dedupe survivor: %v", err)
	}
	if survivorID != "commit-earliest" {
		t.Errorf("dedupe kept %q, want commit-earliest (earliest created_at)", survivorID)
	}

	activatedAt := readCommitCaptureActivatedAt(t, repo)
	if activatedAt == "" {
		t.Fatal("commit_capture_activated_at was not published")
	}
	if _, err := time.Parse(time.RFC3339Nano, activatedAt); err != nil {
		t.Errorf("commit_capture_activated_at = %q, not RFC3339Nano: %v", activatedAt, err)
	}

	// Replay: must not error, must not duplicate the index, and must not
	// move the activation marker (legacy rows must stay pinned to the
	// original activation instant, not silently drift on every boot).
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations twice: %v", err)
	}
	if got := readCommitCaptureActivatedAt(t, repo); got != activatedAt {
		t.Errorf("commit_capture_activated_at changed on replay: %q -> %q", activatedAt, got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`, "session-commits-migration"); got != 2 {
		t.Fatalf("commit rows after second replay = %d, want 2 (no re-duplication)", got)
	}
}

// insertRawCommit writes a task_session_commits row bypassing
// CreateSessionCommit, so a test can construct pre-migration states
// (duplicates) the production write path would now refuse.
func insertRawCommit(t *testing.T, repo *Repository, id, sessionID, commitSHA string, committedAt time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, sessionID, commitSHA, committedAt, committedAt); err != nil {
		t.Fatalf("insertRawCommit(%s): %v", id, err)
	}
}

func readCommitCaptureActivatedAt(t *testing.T, repo *Repository) string {
	t.Helper()
	var value string
	err := repo.db.Get(&value, repo.db.Rebind(`
		SELECT value FROM kandev_meta WHERE key = ?
	`), commitCaptureActivatedAtMetaKey)
	if err != nil {
		t.Fatalf("read commit_capture_activated_at: %v", err)
	}
	return value
}
