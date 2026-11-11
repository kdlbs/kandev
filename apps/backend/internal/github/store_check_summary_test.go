package github

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func newCheckSummaryTaskPR(t *testing.T, store *Store, summary *CheckSummary) *TaskPR {
	t.Helper()
	tp := &TaskPR{
		TaskID: "task-check-store", WorkspaceID: "ws-check-store",
		Owner: "acme", Repo: "widget", PRNumber: 61,
		PRURL: "https://github.com/acme/widget/pull/61", PRTitle: "check summary",
		HeadBranch: "feature/checks", BaseBranch: "main", AuthorLogin: "octocat",
		State: "open", HeadSHA: "head-a", CheckSummary: summary, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(context.Background(), tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	return tp
}

func TestStoreCheckSummaryRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	tp := newCheckSummaryTaskPR(t, store, &CheckSummary{HeadSHA: "head-a", Observed: 3, Skipped: 3})

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.CheckSummary == nil || *got.CheckSummary != (CheckSummary{HeadSHA: "head-a", Observed: 3, Skipped: 3}) {
		t.Fatalf("round-trip check summary = %#v", got.CheckSummary)
	}
	if got.CheckSummaryJSON == "" {
		t.Fatal("round-trip check summary JSON is empty")
	}

	got.CheckSummary = &CheckSummary{HeadSHA: "head-a", Observed: 1, Skipped: 0}
	if err := store.UpdateTaskPR(ctx, got); err != nil {
		t.Fatalf("UpdateTaskPR: %v", err)
	}
	updated, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID after update: %v", err)
	}
	if updated.CheckSummary == nil || updated.CheckSummary.Observed != 1 || updated.CheckSummary.Skipped != 0 {
		t.Fatalf("updated check summary = %#v", updated.CheckSummary)
	}
}

func TestStoreCheckSummaryLegacyRowIsUnknown(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	tp := newCheckSummaryTaskPR(t, store, nil)
	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.CheckSummary != nil {
		t.Fatalf("legacy check summary = %#v, want nil", got.CheckSummary)
	}
}

func TestStoreCheckSummaryRejectsMalformedPersistedJSON(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	tp := newCheckSummaryTaskPR(t, store, &CheckSummary{HeadSHA: "head-a", Observed: 2, Skipped: 2})
	if _, err := store.db.Exec(
		store.db.Rebind(`UPDATE github_task_prs SET check_summary = ? WHERE id = ?`),
		`{"head_sha":"head-a","observed":1,"skipped":5}`, tp.ID,
	); err != nil {
		t.Fatalf("seed malformed summary: %v", err)
	}
	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.CheckSummary != nil {
		t.Fatalf("malformed summary hydrated as %#v, want nil", got.CheckSummary)
	}
}

const preCheckSummaryTaskPRsDDL = `
	CREATE TABLE github_task_prs (
		id TEXT PRIMARY KEY,
		github_host TEXT NOT NULL DEFAULT 'github.com',
		workspace_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		pr_url TEXT NOT NULL,
		pr_title TEXT NOT NULL,
		head_branch TEXT NOT NULL,
		base_branch TEXT NOT NULL,
		head_sha TEXT NOT NULL DEFAULT '',
		author_login TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'open',
		review_state TEXT NOT NULL DEFAULT '',
		checks_state TEXT NOT NULL DEFAULT '',
		mergeable_state TEXT NOT NULL DEFAULT '',
		has_merge_conflicts BOOLEAN,
		merge_queue_state TEXT NOT NULL DEFAULT '',
		merge_queue_position INTEGER,
		merge_queue_entry_id TEXT NOT NULL DEFAULT '',
		merge_queue_entry_head_sha TEXT NOT NULL DEFAULT '',
		merge_queue_estimated_time_to_merge_seconds INTEGER,
		merge_queue_last_removal_id TEXT NOT NULL DEFAULT '',
		merge_queue_last_removed_at DATETIME,
		merge_queue_last_removal_reason TEXT NOT NULL DEFAULT '',
		merge_queue_last_removal_before_sha TEXT NOT NULL DEFAULT '',
		review_count INTEGER DEFAULT 0,
		pending_review_count INTEGER DEFAULT 0,
		required_reviews INTEGER,
		comment_count INTEGER DEFAULT 0,
		unresolved_review_threads INTEGER DEFAULT 0,
		checks_total INTEGER DEFAULT 0,
		checks_passing INTEGER DEFAULT 0,
		additions INTEGER DEFAULT 0,
		deletions INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		merged_at DATETIME,
		closed_at DATETIME,
		last_synced_at DATETIME,
		detached_at DATETIME,
		updated_at DATETIME NOT NULL,
		is_draft BOOLEAN,
		changed_files INTEGER,
		merged_by_login TEXT,
		closed_by_login TEXT,
		auto_merge_observed_at DATETIME,
		source TEXT NOT NULL DEFAULT '',
		workflow_attention TEXT NOT NULL DEFAULT '',
		UNIQUE(task_id, repository_id, pr_number)
	)`

func TestStoreCheckSummaryMigrationReplay(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "check-summary-migration.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := db.Exec(preCheckSummaryTaskPRsDDL); err != nil {
		t.Fatalf("create pre-migration github_task_prs: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`INSERT INTO tasks (id, workspace_id, archived_at) VALUES (?, ?, NULL)`),
		"task-legacy", "ws-check"); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		"pr-legacy", "ws-check", "task-legacy", "acme", "widget", 7,
		"https://github.com/acme/widget/pull/7", "legacy", "feature", "main", "octocat", "open", now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("first NewStore: %v", err)
	}
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("second NewStore (replay): %v", err)
	}
	columns, err := store.tableColumns("github_task_prs")
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	if _, ok := columns["check_summary"]; !ok {
		t.Fatal("github_task_prs is missing check_summary after migration")
	}
	got, err := store.GetTaskPRByID(context.Background(), "pr-legacy")
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.CheckSummary != nil {
		t.Fatalf("legacy row summary = %#v, want nil", got.CheckSummary)
	}
}

func TestStoreCheckSummaryLegacyTableRebuildPreservesValue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "check-summary-rebuild.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	legacyDDL := preCheckSummaryTaskPRsDDL + `;`
	legacyDDL = strings.Replace(legacyDDL, "UNIQUE(task_id, repository_id, pr_number)", "UNIQUE(task_id, pr_number)", 1)
	if _, err := db.Exec(legacyDDL); err != nil {
		t.Fatalf("create legacy github_task_prs: %v", err)
	}
	// A database caught mid-upgrade already has the additive column but still
	// carries the legacy unique constraint the rebuild replaces.
	if _, err := db.Exec(`ALTER TABLE github_task_prs ADD COLUMN check_summary TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatalf("add check_summary to legacy table: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`INSERT INTO tasks (id, workspace_id, archived_at) VALUES (?, ?, NULL)`),
		"task-rebuild", "ws-check"); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, head_sha, created_at, updated_at, check_summary
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		"pr-rebuild", "ws-check", "task-rebuild", "acme", "widget", 8,
		"https://github.com/acme/widget/pull/8", "rebuild", "feature", "main", "octocat", "open", "head-a",
		now, now, `{"head_sha":"head-a","observed":2,"skipped":2}`); err != nil {
		t.Fatalf("seed rebuild row: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("NewStore (rebuild): %v", err)
	}
	got, err := store.GetTaskPRByID(context.Background(), "pr-rebuild")
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.CheckSummary == nil || got.CheckSummary.Skipped != 2 {
		t.Fatalf("rebuilt check summary = %#v, want skipped=2", got.CheckSummary)
	}
}
