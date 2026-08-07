package gitlab

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// newLegacyMRWatchStore opens a fresh SQLite DB, creates gitlab_mr_watches
// with the legacy UNIQUE(session_id, repository_id) constraint (pre-dating
// branch-awareness), seeds one row, then hands it to NewStore so the
// migration path under test runs exactly as it does on a real upgrade.
func newLegacyMRWatchStore(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gitlab-legacy.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME);
		CREATE TABLE gitlab_mr_watches (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			project_path TEXT NOT NULL,
			mr_iid INTEGER NOT NULL,
			branch TEXT NOT NULL,
			last_checked_at DATETIME,
			last_note_at DATETIME,
			last_pipeline_state TEXT DEFAULT '',
			last_approval_state TEXT DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(session_id, repository_id)
		)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := sqlxDB.Exec(`
		INSERT INTO gitlab_mr_watches (
			id, session_id, task_id, repository_id, project_path, mr_iid, branch, created_at, updated_at
		) VALUES ('watch-1', 'sess-1', 'task-1', 'repo-1', 'group/proj', 5, 'feat/a', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestMigrateMRWatchUniqueKey_Legacy(t *testing.T) {
	store := newLegacyMRWatchStore(t)
	ctx := context.Background()

	// The pre-existing row survived the rebuild.
	w, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "feat/a")
	if err != nil {
		t.Fatalf("get migrated watch: %v", err)
	}
	if w == nil || w.ID != "watch-1" || w.MRIID != 5 {
		t.Fatalf("expected migrated watch-1 with iid=5, got %+v", w)
	}

	// The new constraint now allows two branches on the same (session, repo).
	second := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 6, Branch: "feat/b",
	}
	if err := store.CreateMRWatch(ctx, second); err != nil {
		t.Fatalf("create second-branch watch after migration: %v", err)
	}

	all, err := store.ListMRWatchesBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 watches after migration + new branch, got %d", len(all))
	}
}

func TestMigrateMRWatchUniqueKey_ReplayIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gitlab-replay.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME)`); err != nil {
		t.Fatalf("create base schema: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	w := &MRWatch{SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/proj", MRIID: 1, Branch: "main"}
	if err := store.CreateMRWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Re-running createTables (as boot does on every start) must be a no-op:
	// the fresh-DB table already has the new constraint, so no legacy
	// substring match triggers a rebuild, and the row survives untouched.
	if err := store.createTables(); err != nil {
		t.Fatalf("replay createTables: %v", err)
	}

	got, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "main")
	if err != nil {
		t.Fatalf("get watch after replay: %v", err)
	}
	if got == nil || got.ID != w.ID {
		t.Fatalf("expected watch to survive replay untouched, got %+v", got)
	}
}

func TestMigrateMRWatchUniqueKey_FreshDBAllowsMultiBranch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w1 := &MRWatch{SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/proj", MRIID: 1, Branch: "feat/a"}
	w2 := &MRWatch{SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/proj", MRIID: 2, Branch: "feat/b"}
	if err := store.CreateMRWatch(ctx, w1); err != nil {
		t.Fatalf("create watch 1: %v", err)
	}
	if err := store.CreateMRWatch(ctx, w2); err != nil {
		t.Fatalf("create watch 2 (different branch, same session+repo): %v", err)
	}

	all, err := store.ListMRWatchesBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 watches on a fresh DB, got %d", len(all))
	}
}

// The rebuild DROPs gitlab_mr_watches, and SQLite drops a table's indexes
// along with it — including the one createTablesSQL created earlier in the
// same createTables() call. Without an explicit re-create, a migrating boot
// leaves the table unindexed until the next process start.
func TestMigrateMRWatchUniqueKey_PreservesTaskIndex(t *testing.T) {
	store := newLegacyMRWatchStore(t)

	var names []string
	if err := store.ro.Select(&names,
		`SELECT name FROM sqlite_master
		 WHERE type='index' AND tbl_name='gitlab_mr_watches' AND name NOT LIKE 'sqlite_%'`); err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	for _, n := range names {
		if n == "idx_gitlab_mr_watches_task_id" {
			return
		}
	}
	t.Fatalf("idx_gitlab_mr_watches_task_id missing after migration, got %v", names)
}
