package worktree

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/physicalclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.8
func TestPostgresAliasIdentity(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO workspaces (id,name,created_at,updated_at) VALUES ('workspace','Workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO tasks (id,workspace_id,title,created_at,updated_at) VALUES ('first','workspace','First',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('second','workspace','Second',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO task_environments (id,task_id,executor_type,status,workspace_path,created_at,updated_at) VALUES ('env-first','first','worktree','ready','/first',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('env-second','second','worktree','ready','/second',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO task_sessions (id,task_id,state,task_environment_id,started_at,updated_at) VALUES ('second','second','COMPLETED','env-second',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	holderDB := openWorktreePostgresConnection(t, dsn, db)
	writer := openWorktreePostgresConnection(t, dsn, db)
	observer := openWorktreePostgresConnection(t, dsn, db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	holder, err := holderDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if err := physicalclaim.Lock(ctx, holder, "physical"); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, `INSERT INTO task_environment_repos (id,task_environment_id,repository_id,branch_slug,worktree_id,status,created_at,updated_at) VALUES ('first','env-first','repo','main','physical','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLiteStore(writer, writer)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := writer.Get(&pid, `SELECT pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- s.CreateWorktree(ctx, &Worktree{ID: "physical", SessionID: "second", RepositoryID: "repo", BranchSlug: "main", Path: "/second/repo", Branch: "main"})
	}()
	t.Cleanup(func() { cancel(); _ = holder.Rollback(); <-finished })
	if err := waitForWorktreePostgresLock(ctx, observer, pid, finished); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	var ambiguous *models.WorktreeIdentityAmbiguousError
	if err := <-done; !errors.As(err, &ambiguous) {
		t.Fatalf("competing claim committed: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM task_environment_repos WHERE worktree_id='physical'`); err != nil || count != 1 {
		t.Fatalf("claims=%d: %v", count, err)
	}
}
