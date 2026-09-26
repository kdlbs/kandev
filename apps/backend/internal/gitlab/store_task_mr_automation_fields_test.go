package gitlab

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// TestStore_TaskMRAutomationFields_RoundTrip covers AC15's free fields
// (DetailedMergeStatus, ReviewerCount, UnapprovedReviewers) persisting and
// reading back through the normal UpsertTaskMR/GetTaskMR path.
func TestStore_TaskMRAutomationFields_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tm := newTestMR("task-1", "", "acme/api", 42)
	tm.DetailedMergeStatus = "mergeable"
	tm.ReviewerCount = 2
	tm.UnapprovedReviewers = 1
	if err := store.UpsertTaskMR(ctx, tm); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetTaskMR(ctx, "task-1", "", "acme/api", 42)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.DetailedMergeStatus != "mergeable" {
		t.Errorf("DetailedMergeStatus = %q, want mergeable", got.DetailedMergeStatus)
	}
	if got.ReviewerCount != 2 {
		t.Errorf("ReviewerCount = %d, want 2", got.ReviewerCount)
	}
	if got.UnapprovedReviewers != 1 {
		t.Errorf("UnapprovedReviewers = %d, want 1", got.UnapprovedReviewers)
	}
}

// TestStore_UpsertTaskMR_NeverClobbersUnresolvedDiscussions guards the
// intentional gap in UpsertTaskMR's ON CONFLICT clause: a value set via
// UpdateTaskMRUnresolvedDiscussions must survive a later lifecycle resync
// (which always upserts UnresolvedDiscussions=0, since GetMRStatus never
// fetches discussions). Without this guard, an automation-subscribed MR's
// unresolved-discussion count would reset to 0 on the very next poll.
func TestStore_UpsertTaskMR_NeverClobbersUnresolvedDiscussions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tm := newTestMR("task-1", "", "acme/api", 42)
	if err := store.UpsertTaskMR(ctx, tm); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if err := store.UpdateTaskMRUnresolvedDiscussions(ctx, tm.ID, 3); err != nil {
		t.Fatalf("UpdateTaskMRUnresolvedDiscussions: %v", err)
	}

	// Simulate the next lifecycle poll: same TaskMR pointer, UnresolvedDiscussions
	// left at its Go zero value (0), re-upserted.
	tm.MRTitle = "new title from resync"
	if err := store.UpsertTaskMR(ctx, tm); err != nil {
		t.Fatalf("resync upsert: %v", err)
	}

	got, err := store.GetTaskMR(ctx, "task-1", "", "acme/api", 42)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.UnresolvedDiscussions != 3 {
		t.Errorf("UnresolvedDiscussions = %d, want 3 (must survive an unrelated resync)", got.UnresolvedDiscussions)
	}
	if got.MRTitle != "new title from resync" {
		t.Errorf("MRTitle = %q, want the resync to still update unrelated columns", got.MRTitle)
	}
}

// TestMigrateTaskMRAutomationFields_PreExistingDBGetsNewColumns covers
// AC33's "DB created after ae31fa85a without the new columns" case: a
// gitlab_task_mrs table built from the pre-this-change CREATE TABLE
// (present but missing detailed_merge_status/reviewer_count/
// unapproved_reviewers/unresolved_discussions) must gain them via
// migrateTaskMRAutomationFields, not silently skip them because the table
// already exists.
func TestMigrateTaskMRAutomationFields_PreExistingDBGetsNewColumns(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gitlab-preexisting.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	if _, err := sqlxDB.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME);
		CREATE TABLE gitlab_task_mrs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			host TEXT NOT NULL DEFAULT '',
			project_path TEXT NOT NULL,
			mr_iid INTEGER NOT NULL,
			mr_url TEXT NOT NULL,
			mr_title TEXT NOT NULL,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			author_username TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT 'open',
			approval_state TEXT NOT NULL DEFAULT '',
			pipeline_state TEXT NOT NULL DEFAULT '',
			merge_status TEXT NOT NULL DEFAULT '',
			draft INTEGER NOT NULL DEFAULT 0,
			approval_count INTEGER DEFAULT 0,
			required_approvals INTEGER DEFAULT 0,
			pipeline_jobs_total INTEGER DEFAULT 0,
			pipeline_jobs_pass INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			merged_at DATETIME,
			closed_at DATETIME,
			last_synced_at DATETIME,
			updated_at DATETIME NOT NULL,
			UNIQUE(task_id, repository_id, project_path, mr_iid)
		);`); err != nil {
		t.Fatalf("create pre-existing gitlab_task_mrs: %v", err)
	}

	if _, err := NewStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("NewStore against pre-existing DB: %v", err)
	}

	columns, err := (&Store{db: sqlxDB, ro: sqlxDB}).tableColumns("gitlab_task_mrs")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	for _, want := range []string{"detailed_merge_status", "reviewer_count", "unapproved_reviewers", "unresolved_discussions"} {
		if _, ok := columns[want]; !ok {
			t.Errorf("column %q missing after migration on a pre-existing DB", want)
		}
	}
}
