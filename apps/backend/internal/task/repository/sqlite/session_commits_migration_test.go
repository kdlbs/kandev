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

// Regression: initSchema() (the real boot path — persistence.Provide calls
// this, not runMigrations() directly) must not crash on an existing database
// that already has duplicate (session_id, commit_sha) rows. Before this
// test, initGitSchema() (base_schema.go) created
// uniq_session_commits_session_sha unconditionally as part of its
// unconditional, error-propagating schema-init Exec — which runs BEFORE
// runMigrations() in initSchema()'s step list, i.e. before
// migrateSessionCommitsDedupeAndActivation ever gets a chance to dedupe.
// CREATE UNIQUE INDEX over pre-existing duplicate data fails outright, and
// initGitSchema returns that error directly (unlike migrate.Apply, which
// swallows it), so initSchema() — and therefore process boot — would abort.
// TestSessionCommitsDedupeAndActivationMigration alone cannot catch this: it
// calls runMigrations() directly, skipping initGitSchema entirely.
func TestInitSchemaSurvivesLegacyDuplicateCommitRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedSessionForGit(t, repo, "task-boot-dup", "session-boot-dup")

	// seedSessionForGit's bare CreateTask leaves workspace_id unset. On a
	// real legacy database every existing task already went through
	// ensureDefaultWorkspace's backfill on a prior boot; simulate that here
	// so this test isolates the task_session_commits ordering bug under
	// test rather than tripping the unrelated (pre-existing, out of scope)
	// ensureDefaultWorkspace behavior for a same-boot, workflow-less task.
	var defaultWorkspaceID string
	if err := repo.db.Get(&defaultWorkspaceID, `SELECT id FROM workspaces ORDER BY created_at LIMIT 1`); err != nil {
		t.Fatalf("look up default workspace: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET workspace_id = ? WHERE id = ?`),
		defaultWorkspaceID, "task-boot-dup"); err != nil {
		t.Fatalf("backfill task workspace_id: %v", err)
	}

	// Simulate the pre-deploy production state this migration exists to
	// clean up: unique index absent, duplicate (session_id, commit_sha) rows
	// already present (e.g. from a pre-idempotency-fix repeated archive
	// capture).
	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS uniq_session_commits_session_sha`); err != nil {
		t.Fatalf("drop index to simulate legacy schema: %v", err)
	}
	now := time.Now().UTC()
	insertRawCommit(t, repo, "dup-a", "session-boot-dup", "dup-sha", now)
	insertRawCommit(t, repo, "dup-b", "session-boot-dup", "dup-sha", now.Add(time.Minute))

	// Simulate the next process boot on this same (legacy, duplicate-laden)
	// database.
	if err := repo.initSchema(); err != nil {
		t.Fatalf("initSchema() on a legacy DB with pre-existing duplicate commit rows must not error: %v", err)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ? AND commit_sha = ?`,
		"session-boot-dup", "dup-sha"); got != 1 {
		t.Errorf("rows for dup-sha after initSchema() = %d, want 1 (deduped)", got)
	}

	var indexName string
	if err := repo.db.Get(&indexName, `
		SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'uniq_session_commits_session_sha'
	`); err != nil {
		t.Fatalf("uniq_session_commits_session_sha index is missing after initSchema(): %v", err)
	}
}

// Regression: migrateSessionCommitsDedupeAndActivation must not swallow a
// failure in the dedupe DELETE or the CREATE UNIQUE INDEX step. Both used to
// run through r.migrate.Apply, which logs a WARN and returns void on any
// error that isn't "already exists" - so a failure there left the function
// returning nil, publishing the activation marker, and leaving
// CreateSessionCommit's `ON CONFLICT (session_id, commit_sha)` writer
// permanently broken (no matching unique constraint) with nothing visibly
// wrong at boot. Dropping the table before calling the migration function
// directly reproduces a failure in that first step without needing to
// fabricate a real duplicate-data edge case DELETE/CREATE INDEX IF NOT
// EXISTS can't otherwise hit.
func TestMigrateSessionCommitsDedupeAndActivationPropagatesDedupeOrIndexFailure(t *testing.T) {
	repo := newRepoForSessionTests(t)

	// newRepoForSessionTests already ran this migration once successfully
	// (via NewWithDB -> initSchema -> runMigrations), so the activation
	// marker already exists with a real timestamp. Capture it, then prove a
	// second, failing call does not touch it - the failure must be surfaced
	// as an error, not silently absorbed and treated as another successful
	// (re-)activation.
	activatedAtBefore := readCommitCaptureActivatedAt(t, repo)

	if _, err := repo.db.Exec(`DROP TABLE task_session_commits`); err != nil {
		t.Fatalf("drop table to simulate a failed dedupe/index step: %v", err)
	}

	if err := repo.migrateSessionCommitsDedupeAndActivation(); err == nil {
		t.Fatal("migrateSessionCommitsDedupeAndActivation returned nil after the dedupe/index step failed - " +
			"the unique index the ON CONFLICT write path depends on may now be silently missing")
	}

	if got := readCommitCaptureActivatedAt(t, repo); got != activatedAtBefore {
		t.Errorf("commit_capture_activated_at changed to %q despite the dedupe/index step failing, want unchanged %q",
			got, activatedAtBefore)
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
