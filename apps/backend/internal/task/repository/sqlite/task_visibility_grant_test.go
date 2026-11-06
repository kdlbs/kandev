package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskVisibilityGrantSurvivesRestartAndRevokes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "visibility-grant.db")
	open := func() (*Repository, *sqlx.DB) {
		connection, err := db.OpenSQLite(path)
		require.NoError(t, err)
		store := sqlx.NewDb(connection, "sqlite3")
		repo, err := NewWithDB(store, store, nil)
		require.NoError(t, err)
		return repo, store
	}
	repo, store := open()
	seedWorkspace(t, repo, "grant-workspace")
	task := &models.Task{ID: "grant-task", WorkspaceID: "grant-workspace", Origin: models.TaskOriginManual, Title: "Coordinator"}
	require.NoError(t, repo.CreateTask(ctx, task))
	now := time.Now().UTC()
	session := &models.TaskSession{ID: "grant-session", TaskID: task.ID, State: models.TaskSessionStateCompleted,
		StartedAt: now, CompletedAt: &now, UpdatedAt: now}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	grant := models.TaskVisibilityGrant{Version: 1, TaskID: task.ID, SessionID: session.ID, WorkspaceID: task.WorkspaceID}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyTaskVisibilityGrant, grant))
	require.NoError(t, store.Close())
	repo, store = open()
	t.Cleanup(func() { _ = store.Close() })
	loadedTask, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	loadedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.True(t, models.HasTaskVisibilityGrant(loadedTask, loadedSession))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyTaskVisibilityGrant, nil))
	loadedSession, err = repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.False(t, models.HasTaskVisibilityGrant(loadedTask, loadedSession))
}
