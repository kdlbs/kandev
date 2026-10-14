package sqlite

import (
	"context"
	"database/sql"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestTaskGitHubIssueSQLiteCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata-cancel.db")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	defer cancelWaiter()
	type contention struct {
		query string
		err   error
	}
	observed := make(chan contention, 1)
	open := func(onBusy func(string, error)) *sqlx.DB {
		connector := metadataBusyConnector{dsn: "file:" + path + "?cache=private&_busy_timeout=5000&_foreign_keys=on", onBusy: onBusy}
		db := sqlx.NewDb(sql.OpenDB(connector), "sqlite3")
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	first := open(nil)
	// Observe the genuine driver's busy error before cancelling at its return
	// boundary. No SQL outcome is replaced, and pool checkout is not lock proof.
	second := open(func(query string, err error) { observed <- contention{query, err}; cancelWaiter() })
	a, err := NewWithDB(first, first, nil)
	require.NoError(t, err)
	b := NewWithInitializedDB(second, second, nil)
	require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "merge-ws", Name: "Merge"}))
	require.NoError(t, a.CreateTask(ctx, &models.Task{ID: "subject", WorkspaceID: "merge-ws", Title: "Original", Metadata: map[string]interface{}{"keep": true}}))
	before, err := b.GetTask(ctx, "subject")
	require.NoError(t, err)
	holder, err := first.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(ctx, `UPDATE tasks SET id=id WHERE id='subject'`)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, updateErr := b.UpdateTaskGitHubIssue(waiterCtx, "subject", &models.TaskGitHubIssueLink{URL: "https://github.com/acme/api/issues/42", Number: 42, Owner: "acme", Repo: "api"})
		result <- updateErr
	}()
	joined := false
	defer func() {
		cancelWaiter()
		_ = holder.Rollback()
		if !joined {
			<-result
		}
	}()
	err = <-result
	joined = true
	require.ErrorIs(t, err, context.Canceled)
	select {
	case actual := <-observed:
		require.Equal(t, `UPDATE tasks SET id = id WHERE 0`, actual.query)
		var busy sqlite3.Error
		require.ErrorAs(t, actual.err, &busy)
		require.Equal(t, sqlite3.ErrBusy, busy.Code)
	case <-ctx.Done():
		t.Fatal("no actual SQLite writer contention observed", ctx.Err())
	}
	require.NoError(t, holder.Commit())
	after, err := b.GetTask(ctx, "subject")
	require.NoError(t, err)
	require.Equal(t, before, after)
}
