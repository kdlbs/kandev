package messagequeue

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestSQLiteReplaceSessionRejectsArchivedSnapshotEntries(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file:queue-lifecycle-regression?mode=memory&cache=shared&_foreign_keys=on")
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at DATETIME, updated_at DATETIME);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL);
		INSERT INTO tasks (id, archived_at, updated_at) VALUES ('task-archived', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO task_sessions (id, task_id) VALUES ('session-archived', 'task-archived');
	`)
	require.NoError(t, err)

	repo, err := NewSQLiteRepository(db, db)
	require.NoError(t, err)
	err = repo.ReplaceSession(context.Background(), "session-archived", []QueuedMessage{
		{ID: "restored", SessionID: "session-archived", TaskID: "task-archived", Position: 1, Content: "stale"},
	}, nil)
	require.ErrorIs(t, err, ErrTaskInactive)
	err = repo.ReplaceSession(context.Background(), "session-archived", nil, &PendingMove{
		TaskID: "task-archived",
	})
	require.ErrorIs(t, err, ErrTaskInactive)
}
