package github

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// seedLegacyPRWatch inserts a row directly into the legacy
// UNIQUE(session_id, repository_id, branch) schema produced by
// openLegacyGitHubDB, bypassing the (already task-owned) Store API so tests
// can reproduce the pre-migration on-disk shape exactly.
func seedLegacyPRWatch(t *testing.T, db *sqlx.DB, w *PRWatch) {
	t.Helper()
	if w.ID == "" {
		t.Fatalf("seedLegacyPRWatch: id required")
	}
	_, err := db.Exec(`
		INSERT INTO github_pr_watches (
			id, session_id, task_id, repository_id, owner, repo, pr_number, branch,
			last_checked_at, last_comment_at, last_check_status, last_review_state,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.SessionID, w.TaskID, w.RepositoryID, w.Owner, w.Repo, w.PRNumber, w.Branch,
		w.LastCheckedAt, w.LastCommentAt, w.LastCheckStatus, w.LastReviewState,
		w.CreatedAt, w.UpdatedAt)
	if err != nil {
		t.Fatalf("seed legacy PR watch %s: %v", w.ID, err)
	}
}

// TestPRWatchMigration_DedupsFiftyResumedSessionWatches is acceptance
// criterion 1: fifty historical sessions resuming the same task/repository/
// branch must collapse to exactly one canonical searching watch, and the
// migration's aggregate counters must reflect that collapse.
func TestPRWatchMigration_DedupsFiftyResumedSessionWatches(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1');
	`); err != nil {
		t.Fatalf("seed workspace/task: %v", err)
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	const sessions = 50
	for i := 0; i < sessions; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		seedLegacyPRWatch(t, db, &PRWatch{
			ID:        fmt.Sprintf("watch-%03d", i),
			SessionID: fmt.Sprintf("session-%03d", i),
			TaskID:    "task-1",
			Owner:     "acme",
			Repo:      "repo",
			PRNumber:  0,
			Branch:    "feature/x",
			CreatedAt: ts,
			UpdatedAt: ts,
		})
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}

	stats := store.PRWatchMigrationStats()
	if stats == nil {
		t.Fatalf("expected migration stats to be populated")
	}
	if stats.RowsBefore != sessions {
		t.Fatalf("RowsBefore = %d, want %d", stats.RowsBefore, sessions)
	}
	if stats.RowsAfter != 1 {
		t.Fatalf("RowsAfter = %d, want 1", stats.RowsAfter)
	}
	if stats.DuplicatesRemoved != sessions-1 {
		t.Fatalf("DuplicatesRemoved = %d, want %d", stats.DuplicatesRemoved, sessions-1)
	}

	survivor, err := store.GetPRWatchByTaskRepoBranch(ctx, "task-1", "", "feature/x")
	if err != nil {
		t.Fatalf("get canonical watch: %v", err)
	}
	if survivor == nil {
		t.Fatalf("expected exactly one canonical searching watch to survive")
	}

	all, err := store.ListPRWatchesByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list watches by task: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("watches remaining for task-1 = %d, want 1", len(all))
	}

	// A second boot against the already-migrated schema must be a pure
	// no-op: no further rows removed, and the migration is skipped because
	// the legacy UNIQUE constraint no longer exists.
	store2, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("second migration pass: %v", err)
	}
	if stats2 := store2.PRWatchMigrationStats(); stats2 != nil {
		t.Fatalf("expected no-op second migration, got stats %+v", stats2)
	}
	all2, err := store2.ListPRWatchesByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list watches by task after second pass: %v", err)
	}
	if len(all2) != 1 {
		t.Fatalf("watches remaining after second pass = %d, want 1", len(all2))
	}
}

// TestPRWatchMigration_PrefersDiscoveredOverSearching is acceptance
// criterion 8: when a duplicate group contains both a still-searching row
// and a row that already discovered its PR, the discovered row must survive
// so in-flight PR state (checks/review watermarks) is not thrown away.
func TestPRWatchMigration_PrefersDiscoveredOverSearching(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1');
	`); err != nil {
		t.Fatalf("seed workspace/task: %v", err)
	}

	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-searching", SessionID: "session-a", TaskID: "task-1",
		Owner: "acme", Repo: "repo", PRNumber: 0, Branch: "feature/x",
		CreatedAt: older, UpdatedAt: newer, // newer UpdatedAt, but still searching
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-discovered", SessionID: "session-b", TaskID: "task-1",
		Owner: "acme", Repo: "repo", PRNumber: 42, Branch: "feature/x",
		LastCheckStatus: "success", LastReviewState: "approved",
		CreatedAt: older, UpdatedAt: older, // older UpdatedAt, but already discovered
	})

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}

	survivor, err := store.GetPRWatchByTaskRepoPRNumber(ctx, "task-1", "", 42)
	if err != nil {
		t.Fatalf("get discovered watch: %v", err)
	}
	if survivor == nil {
		t.Fatalf("expected the discovered watch to survive migration")
	}
	if survivor.ID != "watch-discovered" {
		t.Fatalf("survivor ID = %q, want watch-discovered", survivor.ID)
	}
	if survivor.LastCheckStatus != "success" || survivor.LastReviewState != "approved" {
		t.Fatalf("survivor watermarks = %+v, want preserved success/approved", survivor)
	}

	stillSearching, err := store.GetPRWatchByTaskRepoBranch(ctx, "task-1", "", "feature/x")
	if err != nil {
		t.Fatalf("get searching watch: %v", err)
	}
	if stillSearching != nil {
		t.Fatalf("expected searching duplicate to be removed, got %+v", stillSearching)
	}
}

// TestPRWatchMigration_MergesCrossBranchDiscoveredCollision covers the pass-2
// case where two different branches (e.g. before/after a rename) each
// independently discovered the SAME pull request. Migration must collapse
// them to one discovered row keyed by (task_id, repository_id, pr_number),
// preserving the newest watermarks.
func TestPRWatchMigration_MergesCrossBranchDiscoveredCollision(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1');
	`); err != nil {
		t.Fatalf("seed workspace/task: %v", err)
	}

	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-old-branch", SessionID: "session-a", TaskID: "task-1",
		Owner: "acme", Repo: "repo", PRNumber: 7, Branch: "feature/old-name",
		LastCheckStatus: "pending",
		CreatedAt:       older, UpdatedAt: older,
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-new-branch", SessionID: "session-b", TaskID: "task-1",
		Owner: "acme", Repo: "repo", PRNumber: 7, Branch: "feature/new-name",
		LastCheckStatus: "success",
		CreatedAt:       newer, UpdatedAt: newer,
	})

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}

	all, err := store.ListPRWatchesByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("watches remaining = %d, want 1 (cross-branch PR collision must merge)", len(all))
	}
	if all[0].LastCheckStatus != "success" {
		t.Fatalf("survivor LastCheckStatus = %q, want newest (success)", all[0].LastCheckStatus)
	}
}

// TestPRWatchMigration_RemovesDetachedRepositoryOrphans covers the
// repository-detachment half of the orphan sweep: a watch whose repository
// is no longer attached to its task must be removed even though the task
// itself still exists.
func TestPRWatchMigration_RemovesDetachedRepositoryOrphans(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE task_repositories (task_id TEXT NOT NULL, repository_id TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0);
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1');
		INSERT INTO task_repositories (task_id, repository_id, position) VALUES ('task-1', 'repo-attached', 0);
	`); err != nil {
		t.Fatalf("seed workspace/task/repositories: %v", err)
	}

	now := time.Now().UTC()
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-attached", SessionID: "session-a", TaskID: "task-1",
		RepositoryID: "repo-attached", Owner: "acme", Repo: "repo", Branch: "main",
		CreatedAt: now, UpdatedAt: now,
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-detached", SessionID: "session-b", TaskID: "task-1",
		RepositoryID: "repo-detached", Owner: "acme", Repo: "repo", Branch: "main",
		CreatedAt: now, UpdatedAt: now,
	})

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}

	stats := store.PRWatchMigrationStats()
	if stats == nil || stats.OrphansRemoved != 1 {
		t.Fatalf("stats = %+v, want OrphansRemoved = 1", stats)
	}

	if got, err := store.GetPRWatch(ctx, "watch-detached"); err != nil || got != nil {
		t.Fatalf("expected detached-repository watch to be removed, got %+v (err %v)", got, err)
	}
	if got, err := store.GetPRWatch(ctx, "watch-attached"); err != nil || got == nil {
		t.Fatalf("expected attached-repository watch to survive, got %+v (err %v)", got, err)
	}
}

// TestPRWatchMigration_ClearsProvenanceForMissingSessions covers the
// session_id optionality change: a row whose session no longer exists must
// survive with its session_id cleared, not be deleted or treated as
// ineligible for polling.
func TestPRWatchMigration_ClearsProvenanceForMissingSessions(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE sessions (id TEXT PRIMARY KEY);
		INSERT INTO workspaces (id) VALUES ('ws-1');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1');
		INSERT INTO sessions (id) VALUES ('session-alive');
	`); err != nil {
		t.Fatalf("seed workspace/task/sessions: %v", err)
	}

	now := time.Now().UTC()
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-alive-session", SessionID: "session-alive", TaskID: "task-1",
		Owner: "acme", Repo: "repo", Branch: "main", CreatedAt: now, UpdatedAt: now,
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-dead-session", SessionID: "session-completed-and-gone", TaskID: "task-1",
		Owner: "acme", Repo: "repo", Branch: "other", CreatedAt: now, UpdatedAt: now,
	})

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}

	got, err := store.GetPRWatch(ctx, "watch-dead-session")
	if err != nil {
		t.Fatalf("get watch with dead session: %v", err)
	}
	if got == nil {
		t.Fatalf("expected watch with a missing session to survive (session_id is provenance only)")
	}
	if got.SessionID != "" {
		t.Fatalf("SessionID = %q, want cleared to empty", got.SessionID)
	}
}

// TestCreatePRWatch_DedupsAcrossResumedSessions is acceptance criteria 1/5:
// two sessions calling the service layer for the same task/repository/branch
// must reuse the single canonical watch rather than creating a duplicate,
// regardless of session identity.
func TestCreatePRWatch_DedupsAcrossResumedSessions(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()

	first, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "acme", "repo", 0, "feature/x")
	if err != nil {
		t.Fatalf("create PR watch (session-1): %v", err)
	}
	second, err := svc.CreatePRWatch(ctx, "session-2", "task-1", "repo-1", "acme", "repo", 0, "feature/x")
	if err != nil {
		t.Fatalf("create PR watch (session-2): %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same canonical watch across sessions, got %s and %s", first.ID, second.ID)
	}

	all, err := store.ListPRWatchesByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("watches for task-1 = %d, want 1", len(all))
	}
}

// TestEnsurePRWatch_DedupsAcrossResumedSessions mirrors the above for the
// EnsurePRWatch entry point used by push-detection / worktree resume paths.
func TestEnsurePRWatch_DedupsAcrossResumedSessions(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()

	first, err := svc.EnsurePRWatch(ctx, "session-1", "task-1", "repo-1", "acme", "repo", "feature/x")
	if err != nil {
		t.Fatalf("ensure PR watch (session-1): %v", err)
	}
	second, err := svc.EnsurePRWatch(ctx, "session-2", "task-1", "repo-1", "acme", "repo", "feature/x")
	if err != nil {
		t.Fatalf("ensure PR watch (session-2): %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same canonical watch across sessions, got %s and %s", first.ID, second.ID)
	}

	all, err := store.ListPRWatchesByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("watches for task-1 = %d, want 1", len(all))
	}
}

// TestListActivePRWatches_ReviewTaskSurvivesCompletedSession is acceptance
// criterion 2: a task in Review whose originating session has completed (or
// disappeared entirely) must remain in ListActivePRWatches as long as the
// task itself is not archived/deleted. Watch eligibility must not depend on
// session state; archiving the task is what finally stops polling.
func TestListActivePRWatches_ReviewTaskSurvivesCompletedSession(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO tasks (id, workspace_id, archived_at) VALUES ('task-in-review', 'ws-1', NULL)`); err != nil {
		t.Fatalf("seed review task: %v", err)
	}

	watch := &PRWatch{
		// SessionID intentionally references a session that has already
		// completed and been cleaned up elsewhere; it must not gate
		// eligibility here.
		SessionID: "session-long-gone", TaskID: "task-in-review",
		Owner: "acme", Repo: "repo", PRNumber: 99, Branch: "feature/x",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	active, err := store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list active watches: %v", err)
	}
	if len(active) != 1 || active[0].ID != watch.ID {
		t.Fatalf("active watches = %+v, want the Review task's watch to remain monitored", active)
	}

	// Archiving the task (the actual eligibility boundary) is what removes
	// it from polling — not session completion.
	if _, err := store.db.ExecContext(ctx,
		`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = 'task-in-review'`); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	activeAfterArchive, err := store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list active watches after archive: %v", err)
	}
	if len(activeAfterArchive) != 0 {
		t.Fatalf("active watches after archive = %+v, want none", activeAfterArchive)
	}
}

// TestPRWatchCanonicalIndexes_EnforceUniquenessAtDatabaseLevel is defense in
// depth for acceptance criterion 5: even if a caller bypassed the
// service-layer dedup in getExistingCanonicalPRWatch (e.g. a future
// regression, or a second backend process racing this one), the partial
// unique indexes installed by applyIdempotentSchemaIndexes must themselves
// reject a second searching row and a second discovered row for the same
// canonical key.
func TestPRWatchCanonicalIndexes_EnforceUniquenessAtDatabaseLevel(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.CreatePRWatch(ctx, &PRWatch{
		ID: "watch-searching-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "repo", PRNumber: 0, Branch: "feature/x",
	}); err != nil {
		t.Fatalf("create first searching watch: %v", err)
	}
	err := store.CreatePRWatch(ctx, &PRWatch{
		ID: "watch-searching-2", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "repo", PRNumber: 0, Branch: "feature/x",
	})
	if err == nil {
		t.Fatalf("expected UNIQUE violation inserting a second searching watch for the same (task, repository, branch)")
	}

	if err := store.CreatePRWatch(ctx, &PRWatch{
		ID: "watch-discovered-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "repo", PRNumber: 42, Branch: "feature/y",
	}); err != nil {
		t.Fatalf("create first discovered watch: %v", err)
	}
	err = store.CreatePRWatch(ctx, &PRWatch{
		ID: "watch-discovered-2", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "repo", PRNumber: 42, Branch: "feature/z",
	})
	if err == nil {
		t.Fatalf("expected UNIQUE violation inserting a second discovered watch for the same (task, repository, pr_number)")
	}
}
