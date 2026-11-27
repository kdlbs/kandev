package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.22
func TestFreshSessionRecoveryProjectionWithoutGeneration(t *testing.T) {
	repo := freshSessionProjectionRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Fresh session"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}))
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	projection := dto.FromTaskSession(session)
	require.NoError(t, dto.EnrichSessionRecoveryBlocks(ctx, &projection, session, repo))
	require.NotNil(t, projection.SessionRecoveryBlocks)
	require.Empty(t, projection.SessionRecoveryBlocks)
	_, err = repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound, "projection must not create a generation")
}

func freshSessionProjectionRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "projection.db"))
	require.NoError(t, err)
	sqlDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo, err := sqlite.NewWithDB(sqlDB, sqlDB, nil)
	require.NoError(t, err)
	return repo
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.24
func TestFreshSessionRecoveryProjectionUsesCurrentGeneration(t *testing.T) {
	repo := freshSessionProjectionRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Recovery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}))
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	for _, generation := range []int64{1, 2} {
		require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
			SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: generation,
			NativeSessionID: "native", CreationReason: "initial",
		}))
	}
	for _, block := range []*models.SessionRecoveryBlock{
		{ID: "current", IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 2, State: models.RecoveryBlockOpen},
		{ID: "old-generation", IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, State: models.RecoveryBlockOpen},
		{ID: "old-incarnation", IncarnationID: "old", ExpectedGeneration: 2, State: models.RecoveryBlockOpen},
		{ID: "resolved", IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 2, State: models.RecoveryBlockResolved},
	} {
		block.SessionID, block.Reason = session.ID, block.ID
		require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	}
	projection := dto.FromTaskSession(session)
	require.NoError(t, dto.EnrichSessionRecoveryBlocks(ctx, &projection, session, repo))
	require.Len(t, projection.SessionRecoveryBlocks, 1)
	require.Equal(t, "current", projection.SessionRecoveryBlocks[0].ID)
}
