package worktree

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSQLiteStore_ReinitializesSchemaOnPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first task schema init: %v", err)
	}
	if _, err := NewSQLiteStore(db, db); err != nil {
		t.Fatalf("first worktree schema init: %v", err)
	}
	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second task schema init: %v", err)
	}
	if _, err := NewSQLiteStore(db, db); err != nil {
		t.Fatalf("second worktree schema init: %v", err)
	}
}

func TestSQLiteStore_CreateWorktreeSerializesAgainstPostgresSessionRebind(t *testing.T) {
	ctx := context.Background()
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task schema: %v", err)
	}
	holderDB := openWorktreePostgresConnection(t, dsn, db)
	moverDB := openWorktreePostgresConnection(t, dsn, db)
	observerDB := openWorktreePostgresConnection(t, dsn, db)
	store, err := NewSQLiteStore(moverDB, moverDB)
	if err != nil {
		t.Fatalf("new worktree store: %v", err)
	}
	seeds := []struct {
		label     string
		statement string
	}{
		{"workspace", `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('workspace', 'Workspace', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
		{"original task", `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES ('task-original', 'workspace', 'Original', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
		{"rebound task", `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES ('task-rebound', 'workspace', 'Rebound', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
		{"original environment", `INSERT INTO task_environments (id, task_id, executor_type, status, workspace_path, created_at, updated_at)
			VALUES ('env-original', 'task-original', 'worktree', 'ready', '/tmp/original', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
		{"rebound environment", `INSERT INTO task_environments (id, task_id, executor_type, status, workspace_path, created_at, updated_at)
			VALUES ('env-rebound', 'task-rebound', 'worktree', 'ready', '/tmp/rebound', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
		{"session", `INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
			VALUES ('session-rebind', 'task-original', 'WAITING_FOR_INPUT', 'env-original', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`},
	}
	for _, seed := range seeds {
		if _, err := db.ExecContext(ctx, seed.statement); err != nil {
			t.Fatalf("seed %s: %v", seed.label, err)
		}
	}

	rebindTx, err := holderDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin rebind: %v", err)
	}
	defer func() { _ = rebindTx.Rollback() }()
	if _, err := rebindTx.ExecContext(ctx, `UPDATE task_sessions SET task_environment_id = 'env-rebound' WHERE id = 'session-rebind'`); err != nil {
		t.Fatalf("stage session rebind: %v", err)
	}

	var moverPID int
	if err := moverDB.GetContext(ctx, &moverPID, `SELECT pg_backend_pid()`); err != nil {
		t.Fatalf("read mover backend pid: %v", err)
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- store.CreateWorktree(ctx, &Worktree{
			ID: "wt-rebind", SessionID: "session-rebind", TaskEnvironmentID: "env-original",
			RepositoryID: "repo-rebind", BranchSlug: "branch-rebind", Path: "/tmp/rebound/repo",
			Branch: "feature/rebind", Status: StatusActive,
		})
	}()
	if err := waitForWorktreePostgresLock(ctx, observerDB, moverPID, finished); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
		t.Fatal("CreateWorktree returned while the session rebind still held the row lock")
	default:
	}
	if err := rebindTx.Commit(); err != nil {
		t.Fatalf("commit session rebind: %v", err)
	}
	select {
	case createErr := <-done:
		if !errors.Is(createErr, models.ErrWorkspaceReuseUnsafe) {
			t.Fatalf("CreateWorktree error = %v, want ErrWorkspaceReuseUnsafe", createErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CreateWorktree did not resume after the session rebind committed")
	}
	var count int
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM task_environment_repos WHERE worktree_id = 'wt-rebind'`); err != nil {
		t.Fatalf("count rebound inventory: %v", err)
	}
	if count != 0 {
		t.Fatalf("rebound worktree inventory rows = %d, want 0", count)
	}
}

func openWorktreePostgresConnection(t *testing.T, dsn string, schemaDB *sqlx.DB) *sqlx.DB {
	t.Helper()
	var schema string
	if err := schemaDB.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatalf("read postgres test schema: %v", err)
	}
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres test connection: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`SET search_path TO ` + schema); err != nil {
		t.Fatalf("set postgres test search path: %v", err)
	}
	return db
}

func waitForWorktreePostgresLock(ctx context.Context, observer *sqlx.DB, pid int, done <-chan struct{}) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return fmt.Errorf("CreateWorktree completed before backend %d entered a PostgreSQL lock wait", pid)
		default:
		}
		var waiting bool
		if err := observer.GetContext(deadlineCtx, &waiting, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE pid = $1 AND wait_event_type = 'Lock'
			)
		`, pid); err != nil {
			return fmt.Errorf("query PostgreSQL lock state for backend %d: %w", pid, err)
		}
		if waiting {
			return nil
		}
		select {
		case <-done:
			return fmt.Errorf("CreateWorktree completed before backend %d entered a PostgreSQL lock wait", pid)
		case <-ticker.C:
		case <-deadlineCtx.Done():
			return fmt.Errorf("timed out waiting for backend %d to enter a PostgreSQL lock wait", pid)
		}
	}
}
