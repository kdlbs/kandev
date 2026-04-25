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
