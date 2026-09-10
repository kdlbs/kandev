package handlers

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func setupSQLiteQueueAttachmentCleanup(t *testing.T) (*QueueHandlers, *messagequeue.Service, messagequeue.Repository, messagequeue.QueueSessionIdentity, *recordingQueueAttachmentClaimer) {
	t.Helper()
	raw, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "queue.db")+"?_foreign_keys=on")
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at TIMESTAMP, updated_at TIMESTAMP);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			queue_incarnation_id TEXT NOT NULL
		);
	`)
	require.NoError(t, err)
	identity := messagequeue.QueueSessionIdentity{
		TaskID: "task-cleanup", SessionID: "session-cleanup", SessionIncarnationID: "incarnation-cleanup",
	}
	_, err = database.Exec(`INSERT INTO tasks (id) VALUES (?)`, identity.TaskID)
	require.NoError(t, err)
	_, err = database.Exec(
		`INSERT INTO task_sessions (id, task_id, queue_incarnation_id) VALUES (?, ?, ?)`,
		identity.SessionID, identity.TaskID, identity.SessionIncarnationID,
	)
	require.NoError(t, err)

	repository, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	service := messagequeue.NewService(repository, 10, logger.Default())
	service.SetAutoMergeEnabled(false)
	handlers := NewQueueHandlers(
		service, &mockEventBus{}, logger.Default(), nil, allowQueueIdentityAccess{}, nil,
	)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	return handlers, service, repository, identity, claimer
}

func queuedFileAttachment(id string) messagequeue.MessageAttachment {
	return messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: id, Name: id + ".txt", MimeType: "text/plain", DeliveryMode: "path",
	}
}

func TestQueueRemovalReleasesOnlyUnreferencedAttachmentsSQLite(t *testing.T) {
	handlers, service, _, identity, claimer := setupSQLiteQueueAttachmentCleanup(t)
	ctx := context.Background()
	removed, err := service.QueueMessageWithMetadataForSession(
		ctx, identity, "remove", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{queuedFileAttachment("shared"), queuedFileAttachment("remove-only")}, nil,
	)
	require.NoError(t, err)
	_, err = service.QueueMessageWithMetadataForSession(
		ctx, identity, "retain", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{queuedFileAttachment("shared")}, nil,
	)
	require.NoError(t, err)

	response, err := handlers.wsRemoveEntry(ctx, createTestMessage(t, ws.ActionMessageQueueRemove, map[string]interface{}{
		"task_id": identity.TaskID, "session_id": identity.SessionID,
		"session_incarnation_id": identity.SessionIncarnationID, "entry_id": removed.ID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, []string{"remove-only"}, claimer.releases)
}

func TestQueueCancelAllReleasesOnlyUnreferencedAttachmentsSQLite(t *testing.T) {
	handlers, service, repository, identity, claimer := setupSQLiteQueueAttachmentCleanup(t)
	ctx := context.Background()
	_, err := service.QueueMessageWithMetadataForSession(
		ctx, identity, "in flight", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{queuedFileAttachment("shared")}, nil,
	)
	require.NoError(t, err)
	reserved, _, err := repository.ReserveHeadForDeliveryIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.NotNil(t, reserved)
	_, err = service.QueueMessageWithMetadataForSession(
		ctx, identity, "remove", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{queuedFileAttachment("shared"), queuedFileAttachment("remove-only")}, nil,
	)
	require.NoError(t, err)

	response, err := handlers.wsCancelAll(ctx, createTestMessage(t, ws.ActionMessageQueueCancel, map[string]interface{}{
		"task_id": identity.TaskID, "session_id": identity.SessionID,
		"session_incarnation_id": identity.SessionIncarnationID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, []string{"remove-only"}, claimer.releases)
	stored, err := repository.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.True(t, stored[0].IsReservedInFlight())
}
