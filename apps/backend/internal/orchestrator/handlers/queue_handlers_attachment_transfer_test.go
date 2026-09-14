package handlers

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/stretchr/testify/require"
)

func TestLiveMissingEntryCleanupRefreshesTransferredClaimSession(t *testing.T) {
	handlers, queue, db := newPersistentCleanupQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	defer func() { _ = db.Close() }()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	const (
		sourceSessionID      = "session-live-missing-old"
		destinationSessionID = "session-live-missing-new"
		taskID               = "task-live-missing"
		entryID              = "entry-live-missing"
		operationID          = "cleanup-live-missing"
	)
	attachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-live-missing",
		Name: "live-missing.txt", MimeType: "text/plain", SizeBytes: 1,
	}
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: sourceSessionID, EntryID: entryID, OperationID: operationID,
		TaskID: taskID, OwnerID: "owner", RemoveEntry: true, ClaimPending: true,
		Attachments: []messagequeue.MessageAttachment{attachment},
	}))
	claimer := &controlledCleanupClaimer{}
	claimer.failures.Store(1)
	pending := &pendingQueueAttachmentCleanup{
		key: pendingQueueAttachmentCleanupKey{
			sessionID: sourceSessionID, entryID: entryID, operationID: operationID,
		},
		req: wsUpdateMessageRequest{
			SessionID: sourceSessionID, EntryID: entryID, OperationID: operationID,
		},
		previous: &messagequeue.QueuedMessage{
			ID: entryID, SessionID: sourceSessionID, TaskID: taskID,
			Attachments: []messagequeue.MessageAttachment{attachment},
		},
		releaser: claimer, removeEntry: true, claimPending: true,
		currentSessionID: sourceSessionID, authCtx: ctx,
	}

	require.Error(t, handlers.runPendingAttachmentCleanup(pending))
	require.NoError(t, queue.TransferSession(ctx, sourceSessionID, destinationSessionID))
	require.NoError(t, handlers.runPendingAttachmentCleanup(pending))
	require.Equal(t, destinationSessionID, claimer.lastSession.Load())
}

type transferDuringCleanupUpsertQueue struct {
	*messagequeue.Service
	sourceSessionID       string
	destinationSessionID  string
	transferDone          chan error
	transferredBeforeSave atomic.Bool
}

func (q *transferDuringCleanupUpsertQueue) UpsertAttachmentCleanup(
	ctx context.Context,
	cleanup messagequeue.AttachmentCleanup,
) error {
	go func() {
		q.transferDone <- q.TransferSession(
			context.Background(), q.sourceSessionID, q.destinationSessionID,
		)
	}()
	select {
	case err := <-q.transferDone:
		q.transferredBeforeSave.Store(true)
		if err != nil {
			return err
		}
	case <-time.After(50 * time.Millisecond):
	}
	return q.Service.UpsertAttachmentCleanup(ctx, cleanup)
}

func TestCleanupPreparationHoldsAdmissionAgainstTransfer(t *testing.T) {
	handlers, queue, db := newPersistentCleanupQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	defer func() { _ = db.Close() }()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	entry, err := queue.QueueMessage(
		ctx, "session-prepare-transfer-old", "task-prepare-transfer", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-prepare-transfer",
			Name: "prepare-transfer.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	const destinationSessionID = "session-prepare-transfer-new"
	wrapped := &transferDuringCleanupUpsertQueue{
		Service: queue, sourceSessionID: entry.SessionID, destinationSessionID: destinationSessionID,
		transferDone: make(chan error, 1),
	}
	handlers.queueService = wrapped
	_, err = handlers.preparePendingAttachmentCleanup(
		ctx,
		wsUpdateMessageRequest{
			SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "prepare-transfer",
		},
		entry.TaskID,
		entry.Attachments,
		&controlledCleanupClaimer{},
	)
	require.NoError(t, err)
	require.False(t, wrapped.transferredBeforeSave.Load())
	select {
	case err := <-wrapped.transferDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("session transfer remained blocked after cleanup persistence")
	}
	cleanup, err := queue.GetAttachmentCleanup(
		ctx, entry.SessionID, entry.ID, "prepare-transfer",
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	require.Equal(t, destinationSessionID, cleanup.CurrentSessionID)
}
