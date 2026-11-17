package github

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPRWatchRetirement_PostgresConcurrency(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	primaryDB := testutil.OpenIsolatedPostgres(t, dsn)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	if _, err := primaryDB.ExecContext(ctx, `
		CREATE TABLE workspaces (id TEXT PRIMARY KEY);
		CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, archived_at TIMESTAMPTZ);
		INSERT INTO workspaces (id) VALUES ('ws-retirement-pg');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-retirement-pg', 'ws-retirement-pg');
	`); err != nil {
		t.Fatalf("seed PostgreSQL prerequisites: %v", err)
	}
	primary, err := NewStore(primaryDB, primaryDB)
	require.NoError(t, err)
	var schema string
	require.NoError(t, primaryDB.GetContext(ctx, &schema, `SELECT current_schema()`))
	scopedDSN := testutil.PostgresDSNWithSearchPath(t, dsn, schema)
	workerDB := openPRWatchRetirementPostgresConnection(t, scopedDSN)
	observerDB := openPRWatchRetirementPostgresConnection(t, scopedDSN)
	monitorDB := openPRWatchRetirementPostgresConnection(t, scopedDSN)
	worker, err := NewStore(workerDB, workerDB)
	require.NoError(t, err)
	observer, err := NewStore(observerDB, observerDB)
	require.NoError(t, err)

	watch := &PRWatch{
		WorkspaceID: "ws-retirement-pg", SessionID: "session-retirement-pg", TaskID: "task-retirement-pg",
		Owner: "acme", Repo: "widget", Branch: "finished",
	}
	require.NoError(t, primary.CreatePRWatch(ctx, watch))
	var workerPID, observerPID int
	require.NoError(t, workerDB.GetContext(ctx, &workerPID, `SELECT pg_backend_pid()`))
	require.NoError(t, observerDB.GetContext(ctx, &observerPID, `SELECT pg_backend_pid()`))

	resultEntered := make(chan struct{})
	releaseResult := make(chan struct{})
	var releaseResultOnce sync.Once
	release := func() { releaseResultOnce.Do(func() { close(releaseResult) }) }
	t.Cleanup(release)
	resultDone := make(chan struct{})
	var resultAccepted bool
	var resultErr error
	go func() {
		defer close(resultDone)
		resultAccepted, resultErr = primary.ApplyPRWatchResult(ctx, watch.ID, func(tx *sqlx.Tx, _ *PRWatch) error {
			close(resultEntered)
			<-releaseResult
			_, err := tx.ExecContext(ctx,
				`UPDATE github_pr_watches SET last_check_status = 'applied-before-retirement' WHERE id = $1`, watch.ID)
			return err
		})
	}()
	select {
	case <-resultEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	retireDone := make(chan struct{})
	var retired *PRWatch
	var retireErr error
	go func() {
		defer close(retireDone)
		retired, retireErr = worker.RetirePRWatch(ctx, watch.ID, time.Now().UTC())
	}()
	waitForPRWatchAdvisoryWait(t, ctx, monitorDB, workerPID, retireDone)
	release()
	joinPRWatchPostgresWork(t, ctx, resultDone)
	joinPRWatchPostgresWork(t, ctx, retireDone)
	require.NoError(t, resultErr)
	require.True(t, resultAccepted)
	require.NoError(t, retireErr)
	require.NotNil(t, retired)
	require.NotNil(t, retired.RetiredAt)
	stored, err := observer.GetPRWatch(ctx, watch.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, "applied-before-retirement", stored.LastCheckStatus)
	require.NotNil(t, stored.RetiredAt)

	branchLock := fmt.Sprintf("github-pr-watch-branch:%q:%q:%q", watch.TaskID, watch.RepositoryID, watch.Branch)
	holder, err := primaryDB.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	require.NoError(t, acquirePRWatchAdvisoryLock(ctx, holder, branchLock))
	resumeDone := make(chan struct{})
	var resumed *PRWatch
	var resumeErr error
	go func() {
		defer close(resumeDone)
		resumed, resumeErr = worker.ResumePRWatch(ctx, watch.ID, time.Now().UTC())
	}()
	createDone := make(chan struct{})
	createErr := make(chan error, 1)
	go func() {
		defer close(createDone)
		createErr <- observer.CreatePRWatch(ctx, &PRWatch{
			WorkspaceID: watch.WorkspaceID, SessionID: "session-racing-create", TaskID: watch.TaskID,
			Owner: watch.Owner, Repo: watch.Repo, Branch: watch.Branch,
		})
	}()
	waitForPRWatchAdvisoryWait(t, ctx, monitorDB, workerPID, resumeDone)
	waitForPRWatchAdvisoryWait(t, ctx, monitorDB, observerPID, createDone)
	require.NoError(t, holder.Commit())
	joinPRWatchPostgresWork(t, ctx, resumeDone)
	joinPRWatchPostgresWork(t, ctx, createDone)
	require.NoError(t, resumeErr)
	require.NotNil(t, resumed)
	require.NotEqual(t, watch.ID, resumed.ID)
	require.Error(t, <-createErr, "the concurrent create must not create a second canonical watch")

	staleAccepted, err := worker.ApplyPRWatchResult(ctx, watch.ID, func(*sqlx.Tx, *PRWatch) error {
		t.Fatal("pre-resume result was admitted against the new lifetime")
		return nil
	})
	require.NoError(t, err)
	require.False(t, staleAccepted)
	watches, err := observer.ListPRWatchesByTaskIncludingRetired(ctx, watch.TaskID)
	require.NoError(t, err)
	require.Len(t, watches, 1)
	require.Equal(t, resumed.ID, watches[0].ID)
}

func openPRWatchRetirementPostgresConnection(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	raw, err := internaldb.OpenPostgres(dsn, 1, 1)
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "pgx")
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func waitForPRWatchAdvisoryWait(t *testing.T, ctx context.Context, observer *sqlx.DB, pid int, done <-chan struct{}) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waitType, waitEvent string
		err := observer.QueryRowContext(ctx,
			`SELECT wait_event_type, wait_event FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&waitType, &waitEvent)
		if err == nil && waitType == "Lock" && waitEvent == "advisory" {
			return
		}
		select {
		case <-done:
			t.Fatalf("PostgreSQL operation completed before waiting on the shared PR watch lock (wait=%s/%s, err=%v)", waitType, waitEvent, err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func joinPRWatchPostgresWork(t *testing.T, ctx context.Context, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
