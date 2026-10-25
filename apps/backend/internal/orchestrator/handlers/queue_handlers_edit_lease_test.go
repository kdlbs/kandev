package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWsUpdateMessageDoesNotReleaseUnclaimedAttachmentsOnLeaseConflict(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session", "task", "original", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	firstLease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, firstLease.LeaseID, "connection-a"))
	secondLease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-b")
	require.NoError(t, err)

	response, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-a"), createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
		"session_id":               entry.SessionID,
		"entry_id":                 entry.ID,
		"lease_id":                 firstLease.LeaseID,
		"operation_id":             "operation-1",
		"expected_target_revision": firstLease.TargetRevision,
		"content":                  "edited",
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "staged", Name: "staged.txt", MimeType: "text/plain",
		}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, claimer.claims)
	require.Empty(t, claimer.releases)
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, secondLease.LeaseID, "connection-b"))
}

func TestWsBeginEditReturnsEntryNotFoundForDrainedEntry(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session", "task", "original", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	_, ok := queue.TakeQueued(ctx, entry.SessionID)
	require.True(t, ok)

	response, err := handlers.wsBeginEdit(
		ws.WithConnectionID(ctx, "connection"),
		createTestMessage(t, ws.ActionMessageQueueEditBegin, map[string]interface{}{
			"session_id": entry.SessionID,
			"entry_id":   entry.ID,
		}),
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, queueErrorCodeEntryNotFound, parseError(t, response).Code)
}

func TestWsEndEditDispatchesOnlyAfterSuccessfulSave(t *testing.T) {
	drainer := &mockQueueDrainer{}
	handlers, queue := setupQueueHandlersWithDrainer(t, drainer)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session", "task", "original", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	_, err = queue.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)

	response, err := handlers.wsEndEdit(
		ws.WithConnectionID(ctx, "connection"),
		createTestMessage(t, ws.ActionMessageQueueEditEnd, map[string]interface{}{
			"session_id":           entry.SessionID,
			"entry_id":             entry.ID,
			"lease_id":             lease.LeaseID,
			"dispatch_if_auto_run": true,
		}),
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, drainer.autoRunCalls)
	require.Equal(t, entry.SessionID, drainer.sessionID)

	second, err := queue.QueueMessage(ctx, entry.SessionID, entry.TaskID, "not saved", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	secondLease, err := queue.BeginEdit(ctx, second.SessionID, second.ID, "connection")
	require.NoError(t, err)
	response, err = handlers.wsEndEdit(
		ws.WithConnectionID(ctx, "connection"),
		createTestMessage(t, ws.ActionMessageQueueEditEnd, map[string]interface{}{
			"session_id":           second.SessionID,
			"entry_id":             second.ID,
			"lease_id":             secondLease.LeaseID,
			"dispatch_if_auto_run": true,
		}),
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, drainer.autoRunCalls)
}
