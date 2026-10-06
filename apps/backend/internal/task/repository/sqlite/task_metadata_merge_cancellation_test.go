package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type metadataBusyConnector struct {
	dsn    string
	onBusy func(string, error)
}

func (c metadataBusyConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.Driver().Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &metadataBusyConnection{SQLiteConn: conn.(*sqlite3.SQLiteConn), onBusy: c.onBusy}, nil
}
func (metadataBusyConnector) Driver() driver.Driver { return &sqlite3.SQLiteDriver{} }

type metadataBusyConnection struct {
	*sqlite3.SQLiteConn
	onBusy func(string, error)
}

func (c *metadataBusyConnection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.SQLiteConn.ExecContext(ctx, query, args)
	var busy sqlite3.Error
	if c.onBusy != nil && errors.As(err, &busy) && busy.Code == sqlite3.ErrBusy {
		c.onBusy(query, err)
	}
	return result, err
}

// @covers AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergeSQLiteCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata-cancel.db")
	setupCtx := context.Background()
	var cancelWaiter context.CancelFunc
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
	require.NoError(t, a.CreateWorkspace(setupCtx, &models.Workspace{ID: "merge-ws", Name: "Merge"}))
	require.NoError(t, a.CreateTask(setupCtx, &models.Task{ID: "subject", WorkspaceID: "merge-ws", Title: "Original", Metadata: map[string]interface{}{"keep": true}}))
	before, err := b.GetTask(setupCtx, "subject")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	defer cancelWaiter()
	holder, err := first.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(ctx, `UPDATE tasks SET id=id WHERE id='subject'`)
	require.NoError(t, err)
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
