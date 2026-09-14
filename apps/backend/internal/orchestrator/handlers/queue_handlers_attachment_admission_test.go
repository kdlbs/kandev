package handlers

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type admissionCleanupAssertingClaimer struct {
	store          queueAttachmentCleanupStore
	claimAttempts  atomic.Int32
	sawObligation  atomic.Bool
	releaseAttempt atomic.Int32
}

func (c *admissionCleanupAssertingClaimer) ClaimMessageAttachments(
	ctx context.Context,
	_, _ string,
	_ []v1.MessageAttachment,
) error {
	c.claimAttempts.Add(1)
	cleanups, err := c.store.ListAttachmentCleanups(ctx)
	if err == nil && len(cleanups) == 1 && cleanups[0].ClaimPending {
		c.sawObligation.Store(true)
	}
	return errors.New("claim outcome unavailable")
}

func (c *admissionCleanupAssertingClaimer) ReleaseMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	c.releaseAttempt.Add(1)
	return nil
}

func TestQueueAttachmentClaimFailureHasDurableCleanupBeforeClaim(t *testing.T) {
	handlers, queue, db := newPersistentCleanupQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	defer func() { _ = db.Close() }()
	claimer := &admissionCleanupAssertingClaimer{store: queue}
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})

	response, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "session-admission-cleanup",
		"task_id":    "task-admission-cleanup",
		"content":    "queued",
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-admission-cleanup",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, int32(1), claimer.claimAttempts.Load())
	require.True(t, claimer.sawObligation.Load(), "cleanup obligation must exist before the claim starts")
	require.Equal(t, int32(1), claimer.releaseAttempt.Load())
	require.Empty(t, queue.GetStatus(ctx, "session-admission-cleanup").Entries)
	cleanups, err := queue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Empty(t, cleanups)
}

func TestQueuedMessageFingerprintSurvivesSQLiteRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-fingerprint", "task-fingerprint", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "attachment-fingerprint"}},
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	_, reopenedQueue, reopenedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = reopenedDB.Close() }()
	stored, err := reopenedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	before, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	after, err := queuedMessageFingerprint(stored)
	require.NoError(t, err)
	require.Equal(t, before, after, "queued message changed during SQLite restart:\nbefore: %#v\nafter: %#v", entry, stored)
}

func TestAdmissionAttachmentCleanupRemovesEntryAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-admission-restart", "task-admission-restart", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-admission-restart",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "admission-restart",
		TaskID: entry.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: entry.Attachments,
	}))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	reloadedCleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Len(t, reloadedCleanups, 1)
	require.Equal(t, fingerprint, reloadedCleanups[0].EntryFingerprint)
	reloadedEntry, err := restartedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	reloadedFingerprint, err := queuedMessageFingerprint(reloadedEntry)
	require.NoError(t, err)
	require.Equal(t, fingerprint, reloadedFingerprint)
	claimer := &controlledCleanupClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		_, getErr := restartedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return errors.Is(getErr, messagequeue.ErrEntryNotFound) &&
			claimer.released.Load() == 1 && listErr == nil && len(cleanups) == 0
	}, time.Second, 10*time.Millisecond)
	cleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Empty(t, cleanups)
}

type admissionClaimRecoveryClaimer struct {
	claims   atomic.Int32
	releases atomic.Int32
}

func (c *admissionClaimRecoveryClaimer) ClaimMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	c.claims.Add(1)
	return nil
}

func (c *admissionClaimRecoveryClaimer) ReleaseMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	c.releases.Add(1)
	return nil
}

func TestPendingAdmissionClaimResumesWithoutRemovingEntry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-claim-restart", "task-claim-restart", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-claim-restart",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "claim-restart",
		TaskID: entry.TaskID, OwnerID: "owner", ClaimPending: true,
		EntryFingerprint: fingerprint, Attachments: entry.Attachments,
	}))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return listErr == nil && len(cleanups) == 0 && claimer.claims.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, claimer.releases.Load())
	current, err := restartedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	require.Equal(t, entry.ID, current.ID)
}

type failSecondCleanupUpsertQueue struct {
	QueueService
	service *messagequeue.Service
	upserts atomic.Int32
}

func (q *failSecondCleanupUpsertQueue) AttachmentCleanupPersistenceAvailable() bool { return true }

func (q *failSecondCleanupUpsertQueue) UpsertAttachmentCleanup(
	ctx context.Context,
	cleanup messagequeue.AttachmentCleanup,
) error {
	if q.upserts.Add(1) == 2 {
		return errors.New("cleanup state update unavailable")
	}
	return q.service.UpsertAttachmentCleanup(ctx, cleanup)
}

func (q *failSecondCleanupUpsertQueue) DeleteAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) error {
	return q.service.DeleteAttachmentCleanup(ctx, sessionID, entryID, operationID)
}

func (q *failSecondCleanupUpsertQueue) ListAttachmentCleanups(
	ctx context.Context,
) ([]messagequeue.AttachmentCleanup, error) {
	return q.service.ListAttachmentCleanups(ctx)
}

func (q *failSecondCleanupUpsertQueue) WithSessionAdmission(
	ctx context.Context,
	sessionID string,
	fn func(context.Context) error,
) error {
	return q.service.WithSessionAdmission(ctx, sessionID, fn)
}

func (q *failSecondCleanupUpsertQueue) RemoveEntryWithEntry(
	ctx context.Context,
	sessionID, entryID string,
) (*messagequeue.QueuedMessage, error) {
	return q.service.RemoveEntryWithEntry(ctx, sessionID, entryID)
}

func TestAdmissionClaimFailureRemovesEntryWhenCleanupStateUpdateFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	handlers, queue, db := newPersistentCleanupQueue(t, dbPath)
	handlers.queueService = &failSecondCleanupUpsertQueue{QueueService: queue, service: queue}
	claimer := &admissionCleanupAssertingClaimer{store: queue}
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})

	response, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "session-admission-update-failure",
		"task_id":    "task-admission-update-failure",
		"content":    "queued",
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-admission-update-failure",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, queue.GetStatus(ctx, "session-admission-update-failure").Entries)
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	recoveryClaimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(recoveryClaimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return listErr == nil && len(cleanups) == 0 && recoveryClaimer.releases.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, recoveryClaimer.claims.Load())
}

func TestAdmissionCleanupDoesNotMutateReplacementEntryAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	original, err := queue.QueueMessage(
		ctx, "session-cleanup-fence", "task-cleanup-fence", "original", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-original",
			Name: "original.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(original)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: original.SessionID, EntryID: original.ID, OperationID: "admission-fenced",
		TaskID: original.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: original.Attachments,
	}))
	require.NoError(t, queue.UpdateMessageWithMetadata(
		ctx, original.SessionID, original.ID, "replacement",
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-replacement",
			Name: "replacement.txt", MimeType: "text/plain", SizeBytes: 1,
		}}, nil, messagequeue.QueuedByUser,
	))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return listErr == nil && len(cleanups) == 0 && claimer.releases.Load() == 1
	}, time.Second, 10*time.Millisecond)
	current, err := restartedQueue.GetEntry(ctx, original.SessionID, original.ID)
	require.NoError(t, err)
	require.Equal(t, "replacement", current.Content)
	require.Zero(t, claimer.claims.Load())
}

func TestAdmissionCleanupStillRemovesReorderedEntryAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	target, err := queue.QueueMessage(
		ctx, "session-cleanup-reorder", "task-cleanup-reorder", "target", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-reorder",
			Name: "reorder.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	other, err := queue.QueueMessage(
		ctx, target.SessionID, target.TaskID, "other", "",
		messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(target)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: target.SessionID, EntryID: target.ID, OperationID: "admission-reordered",
		TaskID: target.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: target.Attachments,
	}))
	require.NoError(t, queue.ReorderEntries(ctx, target.SessionID, []string{other.ID, target.ID}))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		_, getErr := restartedQueue.GetEntry(ctx, target.SessionID, target.ID)
		return errors.Is(getErr, messagequeue.ErrEntryNotFound) && claimer.releases.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestAdmissionCleanupFollowsTransferredEntryAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-cleanup-transfer-old", "task-cleanup-transfer", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-transfer",
			Name: "transfer.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "admission-transferred",
		TaskID: entry.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: entry.Attachments,
	}))
	const destinationSessionID = "session-cleanup-transfer-new"
	require.NoError(t, queue.TransferSession(ctx, entry.SessionID, destinationSessionID))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		_, getErr := restartedQueue.GetEntry(ctx, destinationSessionID, entry.ID)
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return errors.Is(getErr, messagequeue.ErrEntryNotFound) &&
			claimer.releases.Load() == 1 && listErr == nil && len(cleanups) == 0
	}, time.Second, 10*time.Millisecond)
	cleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Empty(t, cleanups)
}

type transferReadObservingQueue struct {
	*messagequeue.Service
	destinationSessionID string
	destinationReads     atomic.Int32
}

func (q *transferReadObservingQueue) GetEntry(
	ctx context.Context,
	sessionID, entryID string,
) (*messagequeue.QueuedMessage, error) {
	if sessionID == q.destinationSessionID {
		q.destinationReads.Add(1)
	}
	return q.Service.GetEntry(ctx, sessionID, entryID)
}

func TestTransferredAdmissionCleanupRetainsObligationOnContentMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-cleanup-transfer-mismatch-old", "task-cleanup-transfer-mismatch", "original", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-transfer-mismatch",
			Name: "transfer-mismatch.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "admission-transfer-mismatch",
		TaskID: entry.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: entry.Attachments,
	}))
	const destinationSessionID = "session-cleanup-transfer-mismatch-new"
	require.NoError(t, queue.TransferSession(ctx, entry.SessionID, destinationSessionID))
	require.NoError(t, queue.UpdateMessageWithMetadata(
		ctx, destinationSessionID, entry.ID, "replacement", entry.Attachments, nil, messagequeue.QueuedByUser,
	))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	observedQueue := &transferReadObservingQueue{
		Service: restartedQueue, destinationSessionID: destinationSessionID,
	}
	restarted.queueService = observedQueue
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		return observedQueue.destinationReads.Load() >= 2
	}, time.Second, 10*time.Millisecond)
	current, err := restartedQueue.GetEntry(ctx, destinationSessionID, entry.ID)
	require.NoError(t, err)
	require.Equal(t, "replacement", current.Content)
	cleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Len(t, cleanups, 1)
	require.Zero(t, claimer.releases.Load())
}

func TestAdmissionCleanupWaitsForAnyActiveEditLease(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-admission-lease", "task-admission-lease", "queued", "",
		messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	pending := &pendingQueueAttachmentCleanup{
		req: wsUpdateMessageRequest{
			SessionID: entry.SessionID, EntryID: entry.ID, LeaseID: "superseded-lease",
		},
		authCtx: ctx,
	}

	require.True(t, handlers.editLeaseActive(pending))
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection"))
	require.False(t, handlers.editLeaseActive(pending))
}

type leaseAfterCleanupPrecheckQueue struct {
	QueueService
	service *messagequeue.Service
	active  atomic.Bool
}

func (q *leaseAfterCleanupPrecheckQueue) GetEditLease(
	context.Context,
	string,
	string,
) (*messagequeue.QueueEditLease, error) {
	if !q.active.Load() {
		return nil, messagequeue.ErrEditLeaseNotFound
	}
	return &messagequeue.QueueEditLease{LeaseID: "successor-lease"}, nil
}

func (q *leaseAfterCleanupPrecheckQueue) WithSessionAdmission(
	ctx context.Context,
	sessionID string,
	fn func(context.Context) error,
) error {
	q.active.Store(true)
	return q.service.WithSessionAdmission(ctx, sessionID, fn)
}
func TestAdmissionCleanupRechecksLeaseInsideAdmission(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	entry, err := queue.QueueMessage(
		ctx, "session-cleanup-toctou", "task-cleanup-toctou", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-toctou",
			Name: "toctou.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	releaser := &controlledCleanupClaimer{}
	wrapped := &leaseAfterCleanupPrecheckQueue{QueueService: queue, service: queue}
	handlers.queueService = wrapped
	pending := &pendingQueueAttachmentCleanup{
		req: wsUpdateMessageRequest{
			SessionID: entry.SessionID, EntryID: entry.ID, LeaseID: "superseded-lease",
		},
		previous: &messagequeue.QueuedMessage{
			ID: entry.ID, SessionID: entry.SessionID, TaskID: entry.TaskID,
			Attachments: entry.Attachments,
		},
		releaser: releaser, removeEntry: true, claimPending: true,
		entryFingerprint: fingerprint, authCtx: ctx,
	}

	require.False(t, handlers.editLeaseActive(pending))
	require.Error(t, handlers.runPendingAttachmentCleanup(pending))
	_, err = queue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	require.Zero(t, releaser.released.Load())
}

type insertionDuringCleanupReleaseClaimer struct {
	queue                 *messagequeue.Service
	sessionID             string
	taskID                string
	attachment            messagequeue.MessageAttachment
	insertedBeforeRelease atomic.Bool
	insertDone            chan error
}

func (c *insertionDuringCleanupReleaseClaimer) ReleaseMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	go func() {
		_, err := c.queue.QueueMessage(
			context.Background(), c.sessionID, c.taskID, "successor", "",
			messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{c.attachment},
		)
		c.insertDone <- err
	}()
	select {
	case err := <-c.insertDone:
		c.insertedBeforeRelease.Store(true)
		return err
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func TestMissingEntryCleanupHoldsAdmissionThroughRelease(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	attachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-missing-admission",
		Name: "missing-admission.txt", MimeType: "text/plain", SizeBytes: 1,
	}
	releaser := &insertionDuringCleanupReleaseClaimer{
		queue: queue, sessionID: "session-missing-admission", taskID: "task-missing-admission",
		attachment: attachment, insertDone: make(chan error, 1),
	}
	pending := &pendingQueueAttachmentCleanup{
		key: pendingQueueAttachmentCleanupKey{
			sessionID: releaser.sessionID, entryID: "missing-entry", operationID: "missing-admission",
		},
		req: wsUpdateMessageRequest{
			SessionID: releaser.sessionID, EntryID: "missing-entry", OperationID: "missing-admission",
		},
		previous: &messagequeue.QueuedMessage{
			ID: "missing-entry", SessionID: releaser.sessionID, TaskID: releaser.taskID,
			Attachments: []messagequeue.MessageAttachment{attachment},
		},
		releaser: releaser, removeEntry: true, claimPending: true,
		authCtx: authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"}),
	}

	require.NoError(t, handlers.runPendingAttachmentCleanup(pending))
	require.False(t, releaser.insertedBeforeRelease.Load())
	select {
	case err := <-releaser.insertDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("successor insertion remained blocked after cleanup release")
	}
}

func TestTransferredSupersededCleanupChecksDestinationLease(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	oldAttachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-transfer-old",
		Name: "old.txt", MimeType: "text/plain", SizeBytes: 1,
	}
	newAttachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-transfer-new",
		Name: "new.txt", MimeType: "text/plain", SizeBytes: 1,
	}
	entry, err := queue.QueueMessage(
		ctx, "session-superseded-transfer-old", "task-superseded-transfer", "updated", "",
		messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{newAttachment},
	)
	require.NoError(t, err)
	const destinationSessionID = "session-superseded-transfer-new"
	require.NoError(t, queue.TransferSession(ctx, entry.SessionID, destinationSessionID))
	lease, err := queue.BeginEdit(ctx, destinationSessionID, entry.ID, "successor-connection")
	require.NoError(t, err)
	releaser := &controlledCleanupClaimer{}
	pending := &pendingQueueAttachmentCleanup{
		key: pendingQueueAttachmentCleanupKey{
			sessionID: entry.SessionID, entryID: entry.ID, operationID: "superseded-transfer",
		},
		req: wsUpdateMessageRequest{
			SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "superseded-transfer",
		},
		previous: &messagequeue.QueuedMessage{
			ID: entry.ID, SessionID: entry.SessionID, TaskID: entry.TaskID,
			Attachments: []messagequeue.MessageAttachment{oldAttachment},
		},
		releaser: releaser, authCtx: ctx,
	}

	require.Error(t, handlers.runPendingAttachmentCleanup(pending))
	require.Equal(t, destinationSessionID, handlers.pendingAttachmentCleanupSessionID(pending))
	require.Zero(t, releaser.released.Load())
	require.NoError(t, queue.EndEdit(ctx, destinationSessionID, entry.ID, lease.LeaseID, "successor-connection"))
}

func TestTransferredCleanupWakesWhenDestinationEditEnds(t *testing.T) {
	handlers, queue, db := newPersistentCleanupQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	defer func() { _ = db.Close() }()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	attachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-wake-transfer",
		Name: "wake-transfer.txt", MimeType: "text/plain", SizeBytes: 1,
	}
	entry, err := queue.QueueMessage(
		ctx, "session-wake-transfer-old", "task-wake-transfer", "queued", "",
		messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{attachment},
	)
	require.NoError(t, err)
	key := pendingQueueAttachmentCleanupKey{
		sessionID: entry.SessionID, entryID: entry.ID, operationID: "wake-transfer",
	}
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: key.operationID,
		TaskID: entry.TaskID, OwnerID: "owner", Attachments: []messagequeue.MessageAttachment{attachment},
	}))
	pending := &pendingQueueAttachmentCleanup{
		key: key,
		req: wsUpdateMessageRequest{
			SessionID: entry.SessionID, EntryID: entry.ID, OperationID: key.operationID,
		},
		currentSessionID: entry.SessionID,
		authCtx:          ctx,
		wake:             make(chan struct{}, 1),
	}
	handlers.attachmentCleanupMu.Lock()
	handlers.pendingAttachmentCleanup[key] = pending
	handlers.attachmentCleanupMu.Unlock()
	const destinationSessionID = "session-wake-transfer-new"
	require.NoError(t, queue.TransferSession(ctx, entry.SessionID, destinationSessionID))
	const connectionID = "connection-wake-transfer"
	lease, err := queue.BeginEdit(ctx, destinationSessionID, entry.ID, connectionID)
	require.NoError(t, err)

	response, err := handlers.wsEndEdit(
		ws.WithConnectionID(ctx, connectionID),
		createTestMessage(t, ws.ActionMessageQueueEditEnd, map[string]string{
			"session_id": destinationSessionID,
			"entry_id":   entry.ID,
			"lease_id":   lease.LeaseID,
		}),
	)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	select {
	case <-pending.wake:
	case <-time.After(time.Second):
		t.Fatal("destination edit end did not wake transferred cleanup")
	}
}

func TestMissingEntryCleanupUsesTransferredClaimSessionAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	handlers, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	const (
		sourceSessionID      = "session-missing-transfer-old"
		destinationSessionID = "session-missing-transfer-new"
		taskID               = "task-missing-transfer"
		entryID              = "missing-transfer-entry"
		operationID          = "missing-transfer-cleanup"
	)
	attachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "attachment-missing-transfer",
		Name: "missing-transfer.txt", MimeType: "text/plain", SizeBytes: 1,
	}
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: sourceSessionID, EntryID: entryID, OperationID: operationID,
		TaskID: taskID, OwnerID: "owner", RemoveEntry: true, ClaimPending: true,
		Attachments: []messagequeue.MessageAttachment{attachment},
	}))
	firstClaimer := &controlledCleanupClaimer{}
	firstClaimer.failures.Store(1)
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
		releaser: firstClaimer, removeEntry: true, claimPending: true, authCtx: ctx,
	}
	require.Error(t, handlers.runPendingAttachmentCleanup(pending))
	require.NoError(t, queue.TransferSession(ctx, sourceSessionID, destinationSessionID))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	recoveryClaimer := &controlledCleanupClaimer{}
	restarted.SetAttachmentClaimer(recoveryClaimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
		return err == nil && len(cleanups) == 0 && recoveryClaimer.attempts.Load() > 0
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, destinationSessionID, recoveryClaimer.lastSession.Load())
}
