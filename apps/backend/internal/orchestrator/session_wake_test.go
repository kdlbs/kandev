package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func newRepoForSessionWakeTests(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "session-wake.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqliterepo.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func TestDeliverSessionWakeQueuesServerMessageWithCoalesceKey(t *testing.T) {
	repo := newRepoForSessionWakeTests(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "task-wake", "session-wake", models.TaskSessionStateRunning)
	svc := createTestService(repo, &mockStepGetter{}, &mockTaskRepo{})

	status, err := svc.DeliverSessionWake(ctx, "task-wake", "session-wake", "wake-1", "WAKE:STANDUP")
	require.NoError(t, err)
	require.Equal(t, models.SessionWakeDeliveryQueued, status)

	queueStatus := svc.messageQueue.GetStatus(ctx, "session-wake")
	require.Len(t, queueStatus.Entries, 1)
	require.Equal(t, "WAKE:STANDUP", queueStatus.Entries[0].Content)
	require.Equal(t, messagequeue.QueuedByServer, queueStatus.Entries[0].QueuedBy)
	require.Equal(t, "session-wake:wake-1", queueStatus.Entries[0].Metadata[messagequeue.MetadataCoalesceKey])

	status, err = svc.DeliverSessionWake(ctx, "task-wake", "session-wake", "wake-1", "WAKE:CYCLE")
	require.NoError(t, err)
	require.Equal(t, models.SessionWakeDeliveryQueued, status)
	queueStatus = svc.messageQueue.GetStatus(ctx, "session-wake")
	require.Len(t, queueStatus.Entries, 1)
	require.Equal(t, "WAKE:CYCLE", queueStatus.Entries[0].Content)
}

func TestDeliverSessionWakeRejectsTerminalSessionWithoutQueueing(t *testing.T) {
	repo := newRepoForSessionWakeTests(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "task-wake", "session-done", models.TaskSessionStateCompleted)
	svc := createTestService(repo, &mockStepGetter{}, &mockTaskRepo{})

	status, err := svc.DeliverSessionWake(ctx, "task-wake", "session-done", "wake-1", "WAKE:DONE")
	require.ErrorIs(t, err, models.ErrSessionWakeTerminalDelivery)
	require.Equal(t, models.SessionWakeDeliveryTerminal, status)
	require.Empty(t, svc.messageQueue.GetStatus(ctx, "session-done").Entries)
}

func TestDeliverSessionWakeRejectsDeletedSessionAsTerminal(t *testing.T) {
	repo := newRepoForSessionWakeTests(t)
	ctx := context.Background()
	svc := createTestService(repo, &mockStepGetter{}, &mockTaskRepo{})

	status, err := svc.DeliverSessionWake(ctx, "task-missing", "session-missing", "wake-1", "WAKE:MISSING")
	require.ErrorIs(t, err, models.ErrSessionWakeTerminalDelivery)
	require.Equal(t, models.SessionWakeDeliveryTerminal, status)
}

func TestSessionWakeTerminalErrorWraps(t *testing.T) {
	err := models.NewSessionWakeTerminalDeliveryError("session missing")
	require.True(t, errors.Is(err, models.ErrSessionWakeTerminalDelivery))
}
