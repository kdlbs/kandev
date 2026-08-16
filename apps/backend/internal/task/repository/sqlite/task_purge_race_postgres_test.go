package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// pgBackendPID returns the backend PID of the repository's single connection,
// used to scope pg_locks barrier polls to this test's worker: transactionid
// waits are server-global, so an unrelated waiter from a concurrently running
// package could otherwise false-trigger the barrier.
func pgBackendPID(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var pid int
	if err := db.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("pg_backend_pid: %v", err)
	}
	return pid
}

// waitForWaitingLocks polls pg_locks for the expected number of NOT-granted
// transactionid waits owned by the given backend PID, scoping the barrier to
// this test's worker. The poller must be the held transaction's connection
// (each test handle has a single connection busy inside its lock tx).
func waitForWaitingLocks(t *testing.T, poller interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, backendPID, want int, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := poller.QueryRowContext(context.Background(), `
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid' AND pid = $1
		`, backendPID).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never reached %d waiting lock(s)", what, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// newTaskPostgresRepoPair opens one isolated Postgres schema and constructs
// two task repositories over it (plus the queue tables the purge touches,
// which production creates on the same database), plus an extra
// single-connection DB handle on the same schema. Two instances simulate two
// backend processes sharing a queue database; the extra handle lets a test
// hold a queue_session_locks row on a separate connection while one of the
// worker pools is busy in its lock transaction.
func newTaskPostgresRepoPair(t *testing.T) (*Repository, *Repository, *sqlx.DB) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	setupDB, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if _, err := setupDB.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setupDB.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = setupDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = setupDB.Close()
	})
	open := func() *Repository {
		db, err := sqlx.Open("pgx", dsn)
		if err != nil {
			t.Fatalf("open postgres: %v", err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.Exec("SET search_path TO " + schema); err != nil {
			t.Fatalf("set postgres search_path %s: %v", schema, err)
		}
		// Task schema first: the queue repository resolves tasks-table
		// presence at construction, so it must exist by then for the
		// admission liveness guard to engage.
		repo, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("init task schema: %v", err)
		}
		if _, err := messagequeue.NewSQLiteRepository(db, db); err != nil {
			t.Fatalf("init queue schema: %v", err)
		}
		return repo
	}
	repoA, repoB := open(), open()
	dbC := mustOpenTaskTestDB(t, dsn, schema)
	return repoA, repoB, dbC
}

func mustOpenTaskTestDB(t *testing.T, dsn, schema string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set postgres search_path %s: %v", schema, err)
	}
	return db
}

// seedTaskWithSession creates a workspace, a task in it, and one session.
func seedTaskWithSession(t *testing.T, repo *Repository, taskID, workspaceID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Purge race"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: "wf-" + taskID,
		WorkflowStepID: "step-" + taskID, Title: taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateIdle}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

// TestPostgresRepository_DeleteTask_LocksEmptySessionDuringPurge proves
// DeleteTask captures the task's session set BEFORE deleting the task row
// (task_sessions cascades on deletion): the purge must lock the session even
// when its queue is empty, so a concurrent admission cannot survive the
// purge. The competing backend holds the session lock; DeleteTask blocks on
// it, proving the empty session was still locked.
func TestPostgresRepository_DeleteTask_LocksEmptySessionDuringPurge(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID = "task-del-race"
		sessID = "sess-del-race"
	)
	seedTaskWithSession(t, repoA, taskID, "ws-del-race", sessID)

	dbA := repoA.db
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('sess-del-race')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'sess-del-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	delPID := pgBackendPID(t, repoB.db)
	delDone := make(chan error, 1)
	go func() {
		delDone <- repoB.DeleteTask(ctx, taskID)
	}()

	waitForWaitingLocks(t, lockTx, delPID, 1, "DeleteTask on the empty session's lock")

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-delDone; err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := dbA.GetContext(ctx, &count, `SELECT count(*) FROM tasks WHERE id = 'task-del-race'`); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("task survived delete: count=%d", count)
	}
}

// TestPostgresRepository_WorkspaceCascade_GuardsTaskRowsBeforeSessionLocks
// proves the cascade establishes the global task-row -> session-lock order:
// it must block on a held task row BEFORE taking any queue session lock,
// otherwise lifecycle admission (task row first, then session lock) and the
// cascade deadlock on Postgres.
func TestPostgresRepository_WorkspaceCascade_GuardsTaskRowsBeforeSessionLocks(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const taskID = "task-cascade-race"
	seedTaskWithSession(t, repoA, taskID, "ws-cascade-race", "sess-cascade-race")

	dbA := repoA.db
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM tasks WHERE id = 'task-cascade-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock task row: %v", err)
	}

	cascadePID := pgBackendPID(t, repoB.db)
	cascadeDone := make(chan error, 1)
	go func() {
		_, _, err := repoB.deleteWorkspaceCascade(ctx, "ws-cascade-race", nil, nil)
		cascadeDone <- err
	}()

	waitForWaitingLocks(t, lockTx, cascadePID, 1, "cascade on the task row guard (lock order)")

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-cascadeDone; err != nil {
		t.Fatalf("workspace cascade: %v", err)
	}
	var count int
	if err := dbA.GetContext(ctx, &count, `SELECT count(*) FROM workspaces WHERE id = 'ws-cascade-race'`); err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if count != 0 {
		t.Fatalf("workspace survived cascade: count=%d", count)
	}
}

// TestPostgresRepository_QueueAdmissionRejectedAfterTaskDelete proves the
// ordinary queue admission task-liveness guard: after DeleteTask commits, a
// stale admission targeting the deleted task's session is rejected with
// ErrTaskInactive and no queue row survives.
func TestPostgresRepository_QueueAdmissionRejectedAfterTaskDelete(t *testing.T) {
	repoA, _, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	seedTaskWithSession(t, repoA, "task-post-del", "ws-post-del", "sess-post-del")
	queueRepo, err := messagequeue.NewSQLiteRepository(repoA.db, repoA.db)
	if err != nil {
		t.Fatalf("init queue repo: %v", err)
	}

	if err := repoA.DeleteTask(ctx, "task-post-del"); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	err = queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
		SessionID: "sess-post-del", TaskID: "task-post-del", Content: "orphan", QueuedBy: messagequeue.QueuedByUser,
	}, 0)
	if !errors.Is(err, messagequeue.ErrTaskInactive) {
		t.Fatalf("post-delete admission err = %v, want ErrTaskInactive", err)
	}
	count, err := queueRepo.CountBySession(ctx, "sess-post-del")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("queue survived task delete: count=%d", count)
	}
}

// TestPostgresRepository_AutoMergeCandidateIntoAbove_RejectedAfterArchive
// proves the full-queue fold re-guards the task row in its own transaction:
// an archive committing between the failed insert and the fold must make the
// fold fail with ErrTaskInactive instead of accepting a message the purge
// will silently delete. The competing backend holds the task row (simulating
// the archive mid-flight), the fold blocks on the guard, the archive commits,
// and the fold is rejected.
func TestPostgresRepository_AutoMergeCandidateIntoAbove_RejectedAfterArchive(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID = "task-fold-arch-race"
		sessID = "sess-fold-arch-race"
	)
	seedTaskWithSession(t, repoA, taskID, "ws-fold-arch-race", sessID)
	queueRepoA, err := messagequeue.NewSQLiteRepository(repoA.db, repoA.db)
	if err != nil {
		t.Fatalf("init queue repo A: %v", err)
	}
	if err := queueRepoA.Insert(ctx, &messagequeue.QueuedMessage{
		SessionID: sessID, TaskID: taskID, Content: "first", QueuedBy: messagequeue.QueuedByUser,
	}, 0); err != nil {
		t.Fatalf("seed tail: %v", err)
	}
	queueRepoB, err := messagequeue.NewSQLiteRepository(repoB.db, repoB.db)
	if err != nil {
		t.Fatalf("init queue repo B: %v", err)
	}

	// Simulate an archive that holds the task row while it purges.
	dbA := repoA.db
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		SELECT id FROM tasks WHERE id = 'task-fold-arch-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock task row: %v", err)
	}

	foldPID := pgBackendPID(t, repoB.db)
	foldDone := make(chan error, 1)
	candidate := &messagequeue.QueuedMessage{
		SessionID: sessID, TaskID: taskID, Content: "second", QueuedBy: messagequeue.QueuedByUser,
	}
	go func() {
		merged, didMerge, err := queueRepoB.AutoMergeCandidateIntoAbove(ctx, candidate)
		if err != nil {
			foldDone <- err
			return
		}
		foldDone <- fmt.Errorf("fold succeeded (didMerge=%v merged=%+v), want ErrTaskInactive after archive", didMerge, merged)
	}()

	waitForWaitingLocks(t, lockTx, foldPID, 1, "fold on the task row guard")

	// The archive commits (task archived) while the fold waits on the guard.
	if _, err := lockTx.ExecContext(ctx, `
		UPDATE tasks SET archived_at = now(), updated_at = now() WHERE id = 'task-fold-arch-race'
	`); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit archive: %v", err)
	}

	if err := <-foldDone; !errors.Is(err, messagequeue.ErrTaskInactive) {
		t.Fatalf("fold after archive err = %v, want ErrTaskInactive", err)
	}
}

// TestPostgresRepository_DeleteTask_MissingTaskReturnsErrTaskNotFound proves
// the task-row lock preserves the ErrTaskNotFound classification: on
// PostgreSQL a missing task surfaces as sql.ErrNoRows from the FOR UPDATE
// before any RowsAffected check.
func TestPostgresRepository_DeleteTask_MissingTaskReturnsErrTaskNotFound(t *testing.T) {
	repoA, _, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	if err := repoA.DeleteTask(ctx, "no-such-task"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("DeleteTask missing task err = %v, want ErrTaskNotFound", err)
	}
}

// TestPostgresRepository_DeleteTask_SerializesWithSessionCreation proves
// DeleteTask takes the task-row creation barrier BEFORE capturing the session
// set: a session created mid-flight (while the task row is locked) must be
// included in the purge, otherwise its queue survives task deletion. The
// competing backend holds the task row, DeleteTask blocks on it, a new
// session is committed, and the subsequent purge must block on that new
// session's lock (proving it was captured).
func TestPostgresRepository_DeleteTask_SerializesWithSessionCreation(t *testing.T) {
	repoA, repoB, dbC := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID = "task-del-sess-race"
		sessID = "sess-del-sess-race"
	)
	seedTaskWithSession(t, repoA, taskID, "ws-del-sess-race", sessID)

	dbA := repoA.db
	// Simulate CreateTaskSession holding the task-row creation barrier.
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		SELECT id FROM tasks WHERE id = 'task-del-sess-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock task row: %v", err)
	}

	delPID := pgBackendPID(t, repoB.db)
	delDone := make(chan error, 1)
	go func() {
		delDone <- repoB.DeleteTask(ctx, taskID)
	}()
	waitForWaitingLocks(t, lockTx, delPID, 1, "DeleteTask on the task row barrier")

	// Hold the NEW session's queue lock on a SEPARATE connection BEFORE
	// releasing the task row: DeleteTask must not be able to pass the session
	// lock in the gap between the two acquisitions.
	lockTx2, err := dbC.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin second lock tx: %v", err)
	}
	defer func() { _ = lockTx2.Rollback() }()
	if _, err := lockTx2.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('sess-new')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx2.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'sess-new' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock new session: %v", err)
	}

	// A new session commits while DeleteTask waits on the task row.
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES ('sess-new', 'task-del-sess-race', now(), now())
	`); err != nil {
		t.Fatalf("insert session mid-delete: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit session creation: %v", err)
	}

	// With the fix the purge captured the new session (capture happens after
	// the barrier) and blocks on the held lock; without the fix the stale
	// capture misses it and DeleteTask completes, so the barrier times out.
	waitForWaitingLocks(t, lockTx2, delPID, 1, "DeleteTask purge on the mid-flight session")

	if err := lockTx2.Commit(); err != nil {
		t.Fatalf("commit second lock tx: %v", err)
	}
	if err := <-delDone; err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := dbA.GetContext(ctx, &count, `SELECT count(*) FROM tasks WHERE id = 'task-del-sess-race'`); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("task survived delete: count=%d", count)
	}
}

// TestPostgresRepository_WorkspaceCascade_SerializesWithTaskCreation proves
// the cascade locks the workspace row BEFORE inventorying its tasks: a task
// created mid-cascade is either visible to the inventory (and purged) or
// blocks until the cascade commits. The competing backend holds the workspace
// row, the cascade blocks on it, a new task+session commits, and the purge
// must then block on that new session's lock (proving the inventory saw it).
func TestPostgresRepository_WorkspaceCascade_SerializesWithTaskCreation(t *testing.T) {
	repoA, repoB, dbC := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		wsID   = "ws-cascade-sess-race"
		taskID = "task-cascade-sess-race"
	)
	seedTaskWithSession(t, repoA, taskID, wsID, "sess-cascade-sess-race")

	dbA := repoA.db
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		SELECT id FROM workspaces WHERE id = 'ws-cascade-sess-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock workspace row: %v", err)
	}

	cascadePID := pgBackendPID(t, repoB.db)
	cascadeDone := make(chan error, 1)
	go func() {
		_, _, err := repoB.deleteWorkspaceCascade(ctx, wsID, nil, nil)
		cascadeDone <- err
	}()
	waitForWaitingLocks(t, lockTx, cascadePID, 1, "cascade on the workspace row")

	// Hold the new task's session queue lock on a SEPARATE connection BEFORE
	// releasing the workspace row: the cascade must not be able to pass the
	// session lock in the gap between the two acquisitions.
	lockTx2, err := dbC.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin second lock tx: %v", err)
	}
	defer func() { _ = lockTx2.Rollback() }()
	if _, err := lockTx2.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('sess-cascade-new')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx2.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'sess-cascade-new' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock new session: %v", err)
	}

	// A new task with a session commits while the cascade waits on the
	// workspace row.
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, created_at, updated_at)
		VALUES ('task-cascade-new', 'ws-cascade-sess-race', 'wf', 'step', 'new', now(), now())
	`); err != nil {
		t.Fatalf("insert task mid-cascade: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES ('sess-cascade-new', 'task-cascade-new', now(), now())
	`); err != nil {
		t.Fatalf("insert session mid-cascade: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit task creation: %v", err)
	}

	// With the fix the cascade's inventory (taken after the workspace lock)
	// includes the new task and its purge blocks on the held lock; without the
	// fix the stale inventory misses it and the cascade completes, so the
	// barrier times out.
	waitForWaitingLocks(t, lockTx2, cascadePID, 1, "cascade purge on the mid-cascade task session")

	if err := lockTx2.Commit(); err != nil {
		t.Fatalf("commit second lock tx: %v", err)
	}
	if err := <-cascadeDone; err != nil {
		t.Fatalf("workspace cascade: %v", err)
	}
	var count int
	if err := dbA.GetContext(ctx, &count, `SELECT count(*) FROM workspaces WHERE id = 'ws-cascade-sess-race'`); err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if count != 0 {
		t.Fatalf("workspace survived cascade: count=%d", count)
	}
}
