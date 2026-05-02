package github

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "github.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestTaskPR_PerRepoStorage(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	prFront := &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-front",
		Owner: "kdlbs", Repo: "kandev", PRNumber: 100,
		PRURL:   "https://github.com/kdlbs/kandev/pull/100",
		PRTitle: "frontend changes", HeadBranch: "feat/x", BaseBranch: "main",
		State: "open", CreatedAt: now,
	}
	prBack := &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-back",
		Owner: "kdlbs", Repo: "kandev-backend", PRNumber: 200,
		PRURL:   "https://github.com/kdlbs/kandev-backend/pull/200",
		PRTitle: "backend changes", HeadBranch: "feat/x", BaseBranch: "main",
		State: "open", CreatedAt: now,
	}

	if err := store.CreateTaskPR(ctx, prFront); err != nil {
		t.Fatalf("create front PR: %v", err)
	}
	if err := store.CreateTaskPR(ctx, prBack); err != nil {
		t.Fatalf("create back PR: %v", err)
	}

	gotFront, err := store.GetTaskPRByRepository(ctx, "task-1", "repo-front")
	if err != nil {
		t.Fatalf("get front: %v", err)
	}
	if gotFront == nil || gotFront.PRNumber != 100 {
		t.Errorf("expected front PR #100, got %+v", gotFront)
	}

	gotBack, err := store.GetTaskPRByRepository(ctx, "task-1", "repo-back")
	if err != nil {
		t.Fatalf("get back: %v", err)
	}
	if gotBack == nil || gotBack.PRNumber != 200 {
		t.Errorf("expected back PR #200, got %+v", gotBack)
	}

	all, err := store.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 PRs for task, got %d", len(all))
	}
}

func TestTaskPR_ReplaceTaskPR_ScopedByRepository(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-2", RepositoryID: "repo-a",
		Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-2", RepositoryID: "repo-b",
		Owner: "o", Repo: "r2", PRNumber: 2, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create B: %v", err)
	}

	// Replace only repo-a's PR — repo-b must survive.
	if err := store.ReplaceTaskPR(ctx, &TaskPR{
		TaskID: "task-2", RepositoryID: "repo-a",
		Owner: "o", Repo: "r", PRNumber: 99, CreatedAt: now,
	}); err != nil {
		t.Fatalf("replace A: %v", err)
	}

	all, _ := store.ListTaskPRsByTask(ctx, "task-2")
	if len(all) != 2 {
		t.Fatalf("expected 2 PRs after scoped replace, got %d", len(all))
	}
	bySpec := map[string]int{}
	for _, p := range all {
		bySpec[p.RepositoryID] = p.PRNumber
	}
	if bySpec["repo-a"] != 99 {
		t.Errorf("expected repo-a updated to 99, got %d", bySpec["repo-a"])
	}
	if bySpec["repo-b"] != 2 {
		t.Errorf("expected repo-b unchanged at 2, got %d", bySpec["repo-b"])
	}
}

// Regression: a task that pre-dated the multi-repo schema kept a row with
// repository_id=”, then a later sync inserted a NEW row under the resolved
// repository_id — leaving two rows for the same PR and "+2" badges on the
// kanban card. backfillTaskPRsRepositoryID must dedup the legacy row.
func TestBackfillTaskPRsRepositoryID_DedupsLegacyEmptyRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert a legacy row (repository_id = '') and a per-repo row for the
	// same (task, owner, repo, pr_number). The unique index is
	// (task_id, repository_id, pr_number), so both rows can coexist.
	if err := store.CreateTaskPR(ctx, &TaskPR{
		ID: "legacy", TaskID: "task-x", RepositoryID: "",
		Owner: "kdlbs", Repo: "kandev", PRNumber: 767, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{
		ID: "scoped", TaskID: "task-x", RepositoryID: "repo-1",
		Owner: "kdlbs", Repo: "kandev", PRNumber: 767, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create scoped: %v", err)
	}

	if err := store.backfillTaskPRsRepositoryID(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	all, _ := store.ListTaskPRsByTask(ctx, "task-x")
	if len(all) != 1 {
		t.Fatalf("expected 1 PR after dedup, got %d", len(all))
	}
	if all[0].RepositoryID != "repo-1" {
		t.Errorf("expected scoped row to win, got repository_id=%q", all[0].RepositoryID)
	}

	// Idempotent: second pass is a no-op.
	if err := store.backfillTaskPRsRepositoryID(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	all2, _ := store.ListTaskPRsByTask(ctx, "task-x")
	if len(all2) != 1 {
		t.Errorf("idempotent expected 1 PR, got %d", len(all2))
	}
}

// When task_repositories does not exist (e.g. github-store unit tests that
// init only this package's schema), backfill must skip the cross-package
// UPDATE rather than erroring out.
func TestBackfillTaskPRsRepositoryID_SkipsWhenTaskRepositoriesAbsent(t *testing.T) {
	store := newTestStore(t)
	if err := store.backfillTaskPRsRepositoryID(); err != nil {
		t.Fatalf("backfill on store without task_repositories: %v", err)
	}
}

// Covers the UPDATE path: a legacy row with repository_id=” and NO per-repo
// counterpart should get its repository_id stamped from task_repositories,
// not deleted. This is the most common case in real upgrades — a task that
// has only ever had one PR (so no duplicate to dedup) but pre-dates the
// per-repo schema.
func TestBackfillTaskPRsRepositoryID_BackfillsOrphanRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.db.Exec(`CREATE TABLE task_repositories (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL, position INTEGER DEFAULT 0
	)`); err != nil {
		t.Fatalf("create task_repositories: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO task_repositories VALUES ('r1','task-y','repo-abc',0)`,
	); err != nil {
		t.Fatalf("seed task_repositories: %v", err)
	}

	// Orphan legacy row: no per-repo counterpart, so the dedup DELETE leaves
	// it alone and the UPDATE has to do the healing.
	if err := store.CreateTaskPR(ctx, &TaskPR{
		ID: "legacy2", TaskID: "task-y", RepositoryID: "",
		Owner: "o", Repo: "r", PRNumber: 1, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create legacy: %v", err)
	}

	if err := store.backfillTaskPRsRepositoryID(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	all, _ := store.ListTaskPRsByTask(ctx, "task-y")
	if len(all) != 1 {
		t.Fatalf("expected 1 row, got %d", len(all))
	}
	if all[0].RepositoryID != "repo-abc" {
		t.Errorf("expected backfilled repository_id='repo-abc', got %q", all[0].RepositoryID)
	}
}
