package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergeSQLiteCancellation(t *testing.T) {
	// A private-cache connection waits on the actual file writer, rather than a pool semaphore.
	path := filepath.Join(t.TempDir(), "metadata-cancel.db")
	open := func() *sqlx.DB {
		db, err := sqlx.Open("sqlite3", "file:"+path+"?cache=private&_busy_timeout=5000&_foreign_keys=on")
		require.NoError(t, err)
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	first, second := open(), open()
	a, err := NewWithDB(first, first, nil)
	require.NoError(t, err)
	b := NewWithInitializedDB(second, second, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "merge-ws", Name: "Merge"}))
	require.NoError(t, a.CreateTask(ctx, &models.Task{ID: "subject", WorkspaceID: "merge-ws", Title: "Original", Metadata: map[string]interface{}{"keep": true}}))
	before, err := b.GetTask(ctx, "subject")
	require.NoError(t, err)
	holder, err := first.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(ctx, `UPDATE tasks SET id=id WHERE id='subject'`)
	require.NoError(t, err)
	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- b.MergeTaskMetadata(waiterCtx, "subject", map[string]interface{}{"a": 1, "b": 2}) }()
	joined := false
	defer func() {
		cancelWaiter()
		_ = holder.Rollback()
		if !joined {
			<-result
		}
	}()
	require.Eventually(t, func() bool { return second.Stats().InUse == 1 }, time.Second, time.Millisecond)
	cancelWaiter()
	err = <-result
	joined = true
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, holder.Commit())
	after, err := b.GetTask(ctx, "subject")
	require.NoError(t, err)
	require.Equal(t, before, after)
}
