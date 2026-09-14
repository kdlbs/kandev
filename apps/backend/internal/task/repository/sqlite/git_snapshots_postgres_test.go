package sqlite

// Postgres parity coverage for the git snapshot read/write paths.
// GetLatestGitSnapshotsBySessionIDs uses a ROW_NUMBER() window function with a
// CASE-ordered partition and a rebound IN list; UpsertLatestLiveGitSnapshot
// runs DELETE + INSERT in one transaction; and the files/metadata columns are
// JSON text with an "{}"-sentinel contract. All dialect-sensitive.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresGitSnapshotLifecycle(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-git-pg")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-task-git-pg", TaskID: "task-git-pg",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	for _, sessionID := range []string{"session-git-pg-a", "session-git-pg-b"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-git-pg", TaskEnvironmentID: "env-task-git-pg", State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
		}
	}

	// Archive the task so the archive snapshot's conditional rank-0 branch
	// (`snapshot_type='archive' AND task archived`) is the one being tested —
	// with the task unarchived the row would rank as a stale archive instead.
	// Set archived_at directly rather than calling ArchiveTask: that method
	// also purges the messagequeue in the same transaction, and on Postgres a
	// missing queue table aborts the tx so the later Commit() fails with
	// "commit unexpectedly resulted in rollback". The snapshot ordering only
	// needs the archived_at flag.
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE tasks SET archived_at = ?, updated_at = ? WHERE id = ?`),
		time.Now().UTC(), time.Now().UTC(), "task-git-pg"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	base := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	want := fullGitSnapshot("session-git-pg-a", base)
	if err := repo.CreateGitSnapshot(ctx, want); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}
	got, err := repo.GetLatestGitSnapshot(ctx, "session-git-pg-a")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	assertGitSnapshotEqual(t, got, want)

	// A newer live_monitor row must lose to the older archive row (task still
	// archived) in both the single-row read and the window-function batch read.
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snap-pg-live", SessionID: "session-git-pg-a",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		TriggeredBy: TriggeredByLiveMonitor, HeadCommit: "stale", CreatedAt: base.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(live): %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snap-pg-b", SessionID: "session-git-pg-b",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		TriggeredBy: TriggeredByLiveMonitor, HeadCommit: "b-head", CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(b): %v", err)
	}

	latest, err := repo.GetLatestGitSnapshot(ctx, "session-git-pg-a")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot: %v", err)
	}
	if latest.ID != "snapshot-full" {
		t.Errorf("GetLatestGitSnapshot = %q, want the archive snapshot-full", latest.ID)
	}
	batch, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx,
		[]string{"session-git-pg-a", "session-git-pg-b", "session-git-pg-missing"})
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("batch has %d entries (%v), want 2", len(batch), batch)
	}
	if batch["session-git-pg-a"].ID != "snapshot-full" {
		t.Errorf("batch[a] = %q, want snapshot-full (ROW_NUMBER must mirror the single-row read)",
			batch["session-git-pg-a"].ID)
	}
	if batch["session-git-pg-b"].HeadCommit != "b-head" {
		t.Errorf("batch[b] head = %q, want b-head", batch["session-git-pg-b"].HeadCommit)
	}
	assertJSONMapEqual(t, "batch[a].Files", batch["session-git-pg-a"].Files, want.Files)

	// Unarchive + resume: a newer agent_completed row must now outrank the
	// stale archive row in both selectors (round-3 lifecycle fix parity).
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE tasks SET archived_at = NULL, updated_at = ? WHERE id = ?`),
		time.Now().UTC(), "task-git-pg"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snap-pg-resumed", SessionID: "session-git-pg-a",
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
		TriggeredBy: "agent_completed", HeadCommit: "resumed-head", CreatedAt: base.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(resumed): %v", err)
	}
	latestResumed, err := repo.GetLatestGitSnapshot(ctx, "session-git-pg-a")
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot(resumed): %v", err)
	}
	if latestResumed.ID != "snap-pg-resumed" {
		t.Errorf("after unarchive GetLatestGitSnapshot = %q, want the resumed agent_completed row", latestResumed.ID)
	}
	batchResumed, err := repo.GetLatestGitSnapshotsBySessionIDs(ctx, []string{"session-git-pg-a"})
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsBySessionIDs(resumed): %v", err)
	}
	if batchResumed["session-git-pg-a"].ID != "snap-pg-resumed" {
		t.Errorf("after unarchive batch[a] = %q, want the resumed agent_completed row",
			batchResumed["session-git-pg-a"].ID)
	}

	// The single-live-row upsert transaction.
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snap-pg-live-2", SessionID: "session-git-pg-a", Branch: "main",
		HeadCommit: "fresh", CreatedAt: base.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertLatestLiveGitSnapshot: %v", err)
	}
	liveRows := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ? AND triggered_by = ?`,
		"session-git-pg-a", TriggeredByLiveMonitor)
	if liveRows != 1 {
		t.Errorf("live_monitor rows = %d, want exactly 1", liveRows)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ?`, "session-git-pg-a"); got != 3 {
		t.Errorf("total rows = %d, want 3 (archive + live + resumed agent_completed)", got)
	}

	if err := repo.DeleteLiveMonitorSnapshots(ctx, "session-git-pg-a"); err != nil {
		t.Fatalf("DeleteLiveMonitorSnapshots: %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE session_id = ?`, "session-git-pg-a"); got != 2 {
		t.Errorf("rows after live delete = %d, want 2 (archive + resumed agent_completed)", got)
	}

	if _, err := repo.GetLatestGitSnapshot(ctx, "session-git-pg-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetLatestGitSnapshot(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestPostgresSessionCommitLifecycle(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-commit-pg")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-commit-pg", TaskID: "task-commit-pg", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	want := &models.SessionCommit{
		ID: "commit-pg", SessionID: "session-commit-pg",
		CommitSHA: "0123456789abcdef", ParentSHA: "fedcba9876543210",
		AuthorName: "Kandev Agent", AuthorEmail: "agent@kandev.local",
		CommitMessage:       "feat: postgres parity",
		CommittedAt:         time.Date(2026, 4, 2, 3, 4, 5, 678000000, time.UTC),
		PreCommitSnapshotID: "pre", PostCommitSnapshotID: "post",
		FilesChanged: 9, Insertions: 123, Deletions: 45,
		CreatedAt: time.Date(2026, 4, 2, 3, 4, 6, 0, time.UTC),
	}
	inserted, err := repo.CreateSessionCommit(ctx, want)
	if err != nil {
		t.Fatalf("CreateSessionCommit: %v", err)
	}
	if !inserted {
		t.Error("CreateSessionCommit(want) inserted = false, want true (first observation)")
	}
	got, err := repo.GetLatestSessionCommit(ctx, "session-commit-pg")
	if err != nil {
		t.Fatalf("GetLatestSessionCommit: %v", err)
	}
	assertSessionCommitEqual(t, got, want)

	// The unique index on (session_id, commit_sha) must hold on Postgres too
	// (ADR-0027 parity): the same commit observed by a second trigger (live
	// event, sweep, archive) is a no-op, not a duplicate row or an error.
	dupInserted, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
		ID: "commit-pg-duplicate", SessionID: "session-commit-pg", CommitSHA: want.CommitSHA,
		CommittedAt: want.CommittedAt,
	})
	if err != nil {
		t.Fatalf("CreateSessionCommit(duplicate): %v", err)
	}
	if dupInserted {
		t.Error("CreateSessionCommit(duplicate) inserted = true, want false (ON CONFLICT DO NOTHING)")
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ? AND commit_sha = ?`,
		"session-commit-pg", want.CommitSHA); got != 1 {
		t.Errorf("rows for %s after duplicate insert = %d, want 1", want.CommitSHA, got)
	}

	if _, err := repo.CreateSessionCommit(ctx, &models.SessionCommit{
		ID: "commit-pg-older", SessionID: "session-commit-pg", CommitSHA: "older",
		CommittedAt: want.CommittedAt.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreateSessionCommit(older): %v", err)
	}
	listed, err := repo.GetSessionCommits(ctx, "session-commit-pg")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(listed) != 2 || listed[0].ID != "commit-pg" || listed[1].ID != "commit-pg-older" {
		t.Fatalf("GetSessionCommits = %+v, want commit-pg then commit-pg-older", listed)
	}

	if err := repo.DeleteSessionCommit(ctx, "commit-pg-older"); err != nil {
		t.Fatalf("DeleteSessionCommit: %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`, "session-commit-pg"); got != 1 {
		t.Errorf("rows after delete = %d, want 1", got)
	}
	if _, err := repo.GetLatestSessionCommit(ctx, "session-commit-pg-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetLatestSessionCommit(missing) = %v, want sql.ErrNoRows", err)
	}
}

// TestPostgresSessionCommitsDedupeAndActivationMigration mirrors
// TestSessionCommitsDedupeAndActivationMigration (session_commits_migration_test.go)
// on Postgres: migrateSessionCommitsDedupeAndActivation is dialect-sensitive
// (ROW_NUMBER() OVER (PARTITION BY ...), CREATE UNIQUE INDEX IF NOT EXISTS,
// ON CONFLICT(key) DO NOTHING), and the SQLite test asserts index presence
// through sqlite_master, which does not exist on Postgres — this asserts the
// same end state through pg_indexes instead (ADR-0027 parity).
func TestPostgresSessionCommitsDedupeAndActivationMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-commits-migration-pg")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-commits-migration-pg", TaskID: "task-commits-migration-pg",
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// Simulate a legacy database: no unique index yet, and a duplicate
	// (session_id, commit_sha) pair that a plain INSERT (pre-ON CONFLICT)
	// could have produced, e.g. via a re-run archive capture.
	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS uniq_session_commits_session_sha`); err != nil {
		t.Fatalf("drop unique index to simulate legacy schema: %v", err)
	}

	earlier := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	insertRawCommit(t, repo, "commit-earliest-pg", "session-commits-migration-pg", "dup-sha-pg", earlier)
	insertRawCommit(t, repo, "commit-latest-pg", "session-commits-migration-pg", "dup-sha-pg", later)
	insertRawCommit(t, repo, "commit-unique-pg", "session-commits-migration-pg", "solo-sha-pg", later)

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`,
		"session-commits-migration-pg"); got != 3 {
		t.Fatalf("pre-migration commit rows = %d, want 3", got)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	var indexName string
	if err := repo.db.Get(&indexName, `
		SELECT indexname FROM pg_indexes WHERE indexname = 'uniq_session_commits_session_sha'
	`); err != nil {
		t.Fatalf("uniq_session_commits_session_sha index is missing: %v", err)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ? AND commit_sha = ?`,
		"session-commits-migration-pg", "dup-sha-pg"); got != 1 {
		t.Fatalf("duplicate (session_id, commit_sha) rows after migration = %d, want 1", got)
	}

	var survivorID string
	if err := repo.db.Get(&survivorID, repo.db.Rebind(`
		SELECT id FROM task_session_commits WHERE session_id = ? AND commit_sha = ?
	`), "session-commits-migration-pg", "dup-sha-pg"); err != nil {
		t.Fatalf("select dedupe survivor: %v", err)
	}
	if survivorID != "commit-earliest-pg" {
		t.Errorf("dedupe kept %q, want commit-earliest-pg (earliest created_at)", survivorID)
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
		`SELECT COUNT(1) FROM task_session_commits WHERE session_id = ?`,
		"session-commits-migration-pg"); got != 2 {
		t.Fatalf("commit rows after second replay = %d, want 2 (no re-duplication)", got)
	}
}

func TestPostgresEnvironmentAndExecutorProfileLifecycle(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	// environments.is_system is an INTEGER column written through
	// dialect.BoolToInt and scanned back as an int — worth pinning on Postgres.
	environment := &models.Environment{
		ID: "env-pg", Name: "PG env", Kind: models.EnvironmentKindDockerImage, IsSystem: true,
		WorktreeRoot: "~/kandev", ImageTag: "img:tag", Dockerfile: "FROM scratch",
		BuildConfig: map[string]string{"context": ".", "target": "runtime"},
	}
	if err := repo.CreateEnvironment(ctx, environment); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	gotEnv, err := repo.GetEnvironment(ctx, "env-pg")
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	assertEnvironmentEqual(t, gotEnv, environment)

	environment.IsSystem = false
	environment.BuildConfig = map[string]string{"context": "sub"}
	if err := repo.UpdateEnvironment(ctx, environment); err != nil {
		t.Fatalf("UpdateEnvironment: %v", err)
	}
	gotEnv, err = repo.GetEnvironment(ctx, "env-pg")
	if err != nil {
		t.Fatalf("GetEnvironment after update: %v", err)
	}
	if gotEnv.IsSystem {
		t.Error("IsSystem = true, want false after the update")
	}
	if len(gotEnv.BuildConfig) != 1 || gotEnv.BuildConfig["context"] != "sub" {
		t.Errorf("BuildConfig = %v, want {context: sub}", gotEnv.BuildConfig)
	}

	// Soft delete hides the row from both reads but keeps it on disk.
	if err := repo.DeleteEnvironment(ctx, "env-pg"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	if _, err := repo.GetEnvironment(ctx, "env-pg"); err == nil {
		t.Error("GetEnvironment returned a soft-deleted environment")
	}
	listedEnvs, err := repo.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	for _, candidate := range listedEnvs {
		if candidate.ID == "env-pg" {
			t.Fatalf("ListEnvironments returned the soft-deleted row: %+v", candidate)
		}
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM environments WHERE id = ?`, "env-pg"); got != 1 {
		t.Errorf("rows for the soft-deleted environment = %d, want 1", got)
	}

	// executor_profiles.env_vars is a nullable JSON text column read through
	// sql.NullString with a "null" sentinel check.
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-pg", Name: "PG executor",
		Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	}); err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	profile := &models.ExecutorProfile{
		ID: "profile-pg", ExecutorID: "executor-pg", Name: "PG profile", McpPolicy: "allow",
		Config: map[string]string{"cpus": "4"}, PrepareScript: "prepare", CleanupScript: "cleanup",
		EnvVars: []models.ProfileEnvVar{{Key: "PLAIN", Value: "v"}, {Key: "SECRET", SecretID: "s-1"}},
	}
	if err := repo.CreateExecutorProfile(ctx, profile); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	gotProfile, err := repo.GetExecutorProfile(ctx, "profile-pg")
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	assertExecutorProfileEqual(t, gotProfile, profile)

	scoped, err := repo.ListExecutorProfiles(ctx, "executor-pg")
	if err != nil {
		t.Fatalf("ListExecutorProfiles: %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("ListExecutorProfiles returned %d rows, want 1", len(scoped))
	}
	assertProfileEnvVars(t, "ListExecutorProfiles", scoped[0].EnvVars, profile.EnvVars)

	if err := repo.DeleteExecutorProfile(ctx, "profile-pg"); err != nil {
		t.Fatalf("DeleteExecutorProfile: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM executor_profiles WHERE id = ?`, "profile-pg"); got != 0 {
		t.Errorf("rows after delete = %d, want 0", got)
	}
	if err := repo.DeleteExecutorProfile(ctx, "profile-pg"); err == nil {
		t.Error("second DeleteExecutorProfile returned nil; a missing row must be reported")
	}
}
