package messagequeue

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTransferSessionWithPreparationRollsBackPreparationError(t *testing.T) {
	ctx := context.Background()
	service := setupService(t)
	queued, err := service.QueueMessage(ctx, "session-old", "task", "handoff", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	prepareErr := errors.New("preparation failed after side effect")
	rollbackCalls := 0

	err = service.TransferSessionWithPreparation(
		ctx,
		"session-old",
		"session-new",
		func(context.Context) error {
			return prepareErr
		},
		func(context.Context) error {
			rollbackCalls++
			return nil
		},
	)

	require.ErrorIs(t, err, prepareErr)
	assert.Equal(t, 1, rollbackCalls)
	status := service.GetStatus(ctx, "session-old")
	require.Len(t, status.Entries, 1)
	assert.Equal(t, queued.ID, status.Entries[0].ID)
	assert.Empty(t, service.GetStatus(ctx, "session-new").Entries)
}

func TestDurableSessionTransferPersistsBeforeExternalPreparation(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	_, err := service.QueueMessage(
		ctx,
		"session-old",
		"task",
		"handoff",
		"",
		QueuedByUser,
		false,
		[]MessageAttachment{{AttachmentID: "attachment"}},
	)
	require.NoError(t, err)
	prepareErr := errors.New("external transfer failed")
	rollbackErr := errors.New("external rollback failed")

	err = service.TransferSessionWithDurablePreparation(
		ctx,
		"task",
		"session-old",
		"session-new",
		func(context.Context) error {
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			assert.Equal(t, "session-old", compensations[0].FromSessionID)
			return prepareErr
		},
		func(context.Context) error {
			return rollbackErr
		},
	)

	require.ErrorIs(t, err, prepareErr)
	require.ErrorIs(t, err, rollbackErr)
	compensations, listErr := service.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, compensations, 1)
	assert.Equal(t, "session-new", compensations[0].ToSessionID)
}

func TestDurableSessionTransferIncludesInFlightOrdinaryAttachment(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	queued, err := service.QueueMessage(
		ctx,
		"session-old",
		"task",
		"handoff",
		"",
		QueuedByUser,
		false,
		[]MessageAttachment{{AttachmentID: "attachment"}},
	)
	require.NoError(t, err)
	reserved, ok := service.ReserveQueued(ctx, queued.SessionID)
	require.True(t, ok)
	require.Equal(t, queued.ID, reserved.ID)
	assert.Empty(t, service.GetStatus(ctx, queued.SessionID).Entries)
	prepareCalls := 0

	err = service.TransferSessionWithDurablePreparation(
		ctx,
		queued.TaskID,
		queued.SessionID,
		"session-new",
		func(context.Context) error {
			prepareCalls++
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			assert.Equal(t, []string{queued.ID}, compensations[0].EntryIDs)
			return nil
		},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, prepareCalls)
	pending, err := service.ListPendingQueueDispatches(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "session-new", pending[0].Message.SessionID)
	assert.Equal(t, queued.ID, pending[0].Message.ID)
}

func TestDurableSessionTransferIncludesCleanupOnlyAttachmentClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	cleanup := AttachmentCleanup{
		SessionID: "session-cleanup-only-old", EntryID: "entry-cleanup-only",
		OperationID: "operation-cleanup-only", TaskID: "task-cleanup-only",
		Attachments: []MessageAttachment{{AttachmentID: "attachment-cleanup-only"}},
	}
	require.NoError(t, service.UpsertAttachmentCleanup(ctx, cleanup))
	prepareCalls := 0

	err := service.TransferSessionWithDurablePreparation(
		ctx,
		cleanup.TaskID,
		cleanup.SessionID,
		"session-cleanup-only-new",
		func(context.Context) error {
			prepareCalls++
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			assert.Equal(t, []string{cleanup.EntryID}, compensations[0].EntryIDs)
			return nil
		},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, prepareCalls)
	stored, err := service.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "session-cleanup-only-new", stored.CurrentSessionID)
}

func TestDurableSessionTransferFencesConcurrentSourceInsert(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	_, err := service.QueueMessage(
		ctx,
		"session-old",
		"task",
		"handoff",
		"",
		QueuedByUser,
		false,
		[]MessageAttachment{{AttachmentID: "attachment"}},
	)
	require.NoError(t, err)
	prepared := make(chan struct{})
	release := make(chan struct{})
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- service.TransferSessionWithDurableAttachmentPreparation(
			ctx,
			"task",
			"session-old",
			"session-new",
			func(context.Context, []string) error {
				close(prepared)
				<-release
				return nil
			},
			nil,
		)
	}()
	<-prepared

	err = repo.Insert(ctx, &QueuedMessage{
		ID: "concurrent", SessionID: "session-old", TaskID: "task",
		Content: "concurrent", QueuedBy: QueuedByUser,
	}, DefaultMaxPerSession)
	require.ErrorContains(t, err, "session transfer in progress")
	close(release)
	require.NoError(t, <-transferDone)
}

func TestAttachmentCleanupFollowsActiveSessionTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		UpsertAttachmentCleanup(context.Context, AttachmentCleanup) error
		GetAttachmentCleanup(context.Context, string, string, string) (*AttachmentCleanup, error)
	})
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-operation",
		TaskID:      "task", FromSessionID: "session-old", ToSessionID: "session-new",
	}))
	cleanup := AttachmentCleanup{
		SessionID: "session-old", CurrentSessionID: "session-old",
		EntryID: "entry", OperationID: "operation", TaskID: "task",
		Attachments: []MessageAttachment{{AttachmentID: "attachment"}},
	}

	require.NoError(t, persistence.UpsertAttachmentCleanup(ctx, cleanup))
	stored, err := persistence.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "session-new", stored.CurrentSessionID)
}

func TestAttachmentCleanupDeleteWaitsForActiveSessionTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		UpsertAttachmentCleanup(context.Context, AttachmentCleanup) error
		DeleteAttachmentCleanup(context.Context, string, string, string) error
		GetAttachmentCleanup(context.Context, string, string, string) (*AttachmentCleanup, error)
	})
	cleanup := AttachmentCleanup{
		SessionID: "session-old", EntryID: "entry", OperationID: "operation", TaskID: "task",
		Attachments: []MessageAttachment{{AttachmentID: "attachment"}},
	}
	require.NoError(t, persistence.UpsertAttachmentCleanup(ctx, cleanup))
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-operation",
		TaskID:      "task", FromSessionID: "session-old", ToSessionID: "session-new",
	}))

	err := persistence.DeleteAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.ErrorIs(t, err, ErrSessionTransferInProgress)
	stored, err := persistence.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
}

func TestActiveSessionTransferFencesExistingQueueMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, Repository, *QueuedMessage, *QueuedMessage) error
	}{
		{
			name: "edit",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				return repo.UpdateContent(ctx, first.SessionID, first.ID, "edited", nil, QueuedByUser)
			},
		},
		{
			name: "purge",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.PurgeSession(ctx, first.SessionID)
				return err
			},
		},
		{
			name: "purge task",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.PurgeTask(ctx, first.TaskID)
				return err
			},
		},
		{
			name: "merge",
			mutate: func(ctx context.Context, repo Repository, _, second *QueuedMessage) error {
				_, err := repo.MergeIntoAbove(ctx, second.SessionID, second.ID, QueuedByUser)
				return err
			},
		},
		{
			name: "reorder",
			mutate: func(ctx context.Context, repo Repository, first, second *QueuedMessage) error {
				return repo.ReorderEntries(ctx, first.SessionID, []string{second.ID, first.ID})
			},
		},
		{
			name: "take pending move",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.TakePendingMove(ctx, first.SessionID)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t)
			first := insertTestEntry(t, repo, "session-old", "task", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-old", "task", "second", QueuedByUser, nil, nil)
			require.NoError(t, repo.SetPendingMove(ctx, first.SessionID, &PendingMove{
				MoveID: "move", TaskID: "task", WorkflowID: "workflow", WorkflowStepID: "step",
			}))
			persistence := repo.(interface {
				UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
			})
			require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-operation",
				TaskID:      "task", FromSessionID: first.SessionID, ToSessionID: "session-new",
			}))

			err := test.mutate(ctx, repo, first, second)

			require.ErrorIs(t, err, ErrSessionTransferInProgress)
		})
	}
}

func TestActiveSessionTransferRejectsNonOwnerQueueTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	entry := insertTestEntry(t, repo, "session-old", "task", "first", QueuedByUser, nil, nil)
	persistent := repo.(*sqliteRepository)
	require.NoError(t, persistent.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-owner",
		TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
	}))

	err := repo.TransferSession(ctx, entry.SessionID, "session-third")

	require.ErrorIs(t, err, ErrSessionTransferInProgress)
	stored, listErr := repo.ListBySession(ctx, entry.SessionID)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, entry.ID, stored[0].ID)
}

func TestActiveSessionTransferFencesDispatchSettlement(t *testing.T) {
	tests := []struct {
		name   string
		settle func(context.Context, *sqliteRepository, *QueuedMessage) error
	}{
		{name: "mark accepted", settle: func(ctx context.Context, repo *sqliteRepository, msg *QueuedMessage) error {
			return repo.MarkPendingQueueDispatchAccepted(ctx, msg)
		}},
		{name: "delete", settle: func(ctx context.Context, repo *sqliteRepository, msg *QueuedMessage) error {
			return repo.DeletePendingQueueDispatch(ctx, msg)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repository := newTestSQLiteRepo(t)
			persistent := repository.(*sqliteRepository)
			entry := insertTestEntry(t, repository, "session-old", "task", "first", QueuedByUser, nil, nil)
			reserved, err := repository.ReserveHead(ctx, entry.SessionID)
			require.NoError(t, err)
			require.NotNil(t, reserved)
			require.NoError(t, persistent.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-owner",
				TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
			}))

			err = test.settle(ctx, persistent, reserved)

			require.ErrorIs(t, err, ErrSessionTransferInProgress)
			pending, listErr := persistent.ListPendingQueueDispatches(ctx)
			require.NoError(t, listErr)
			require.Len(t, pending, 1)
			assert.False(t, pending[0].Accepted)
		})
	}
}

func TestActiveSessionTransferFencesSendNowSettlement(t *testing.T) {
	tests := []struct {
		name   string
		settle func(context.Context, *sqliteRepository, *SendNowClaim) error
	}{
		{name: "mark accepted", settle: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.MarkPendingSendNowClaimAccepted(ctx, claim)
		}},
		{name: "delete", settle: func(ctx context.Context, repo *sqliteRepository, claim *SendNowClaim) error {
			return repo.DeletePendingSendNowClaim(ctx, claim)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repository := newTestSQLiteRepo(t)
			persistent := repository.(*sqliteRepository)
			entry := insertTestEntry(t, repository, "session-old", "task", "first", QueuedByUser, nil, nil)
			claim, err := repository.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry})
			require.NoError(t, err)
			require.NoError(t, persistent.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-owner",
				TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
			}))

			err = test.settle(ctx, persistent, claim)

			require.ErrorIs(t, err, ErrSessionTransferInProgress)
			pending, listErr := persistent.ListPendingSendNowClaims(ctx)
			require.NoError(t, listErr)
			require.Len(t, pending, 1)
			assert.False(t, pending[0].Accepted)
		})
	}
}

func setupCrossRepositoryEditTransferFence(
	t *testing.T,
	withLease bool,
) (*Service, *QueuedMessage, *QueueEditLease) {
	t.Helper()
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	service := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := service.QueueMessage(
		ctx, "session-old", "task", "first", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	var lease *QueueEditLease
	if withLease {
		lease, err = service.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
		require.NoError(t, err)
	}
	require.NoError(t, persistent.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-owner",
		TaskID:      entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
	}))
	return service, entry, lease
}

func TestActiveSessionTransferFencesEditLeaseLifecycle(t *testing.T) {
	ctx := context.Background()
	t.Run("begin", func(t *testing.T) {
		service, entry, _ := setupCrossRepositoryEditTransferFence(t, false)
		lease, err := service.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
		require.ErrorIs(t, err, ErrSessionTransferInProgress)
		assert.Nil(t, lease)
	})
	t.Run("renew", func(t *testing.T) {
		service, entry, lease := setupCrossRepositoryEditTransferFence(t, true)
		renewed, err := service.RenewEdit(
			ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection",
		)
		require.ErrorIs(t, err, ErrSessionTransferInProgress)
		assert.Nil(t, renewed)
		stored, getErr := service.GetEditLease(ctx, entry.SessionID, entry.ID)
		require.NoError(t, getErr)
		assert.Equal(t, lease.LeaseGeneration, stored.LeaseGeneration)
	})
	t.Run("end", func(t *testing.T) {
		service, entry, lease := setupCrossRepositoryEditTransferFence(t, true)
		err := service.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection")
		require.ErrorIs(t, err, ErrSessionTransferInProgress)
		_, getErr := service.GetEditLease(ctx, entry.SessionID, entry.ID)
		require.NoError(t, getErr)
	})
}

func TestRemoteSessionTransferRejectsStaleEditLeaseRenewal(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	transferService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	editService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-old", "task", "first", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	lease, err := editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	require.NoError(t, transferService.TransferSessionWithDurablePreparation(
		ctx, entry.TaskID, entry.SessionID, "session-new", nil, nil,
	))

	renewed, err := editService.RenewEdit(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection",
	)

	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	assert.Nil(t, renewed)
}

func TestRemoteSessionTransferRejectsDuplicateEditReplay(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	transferService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	editService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-old", "task", "before", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	lease, err := editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	finalizeErr := errors.New("finalize failed")
	finalizeCalls := 0
	finalize := func(context.Context, *QueuedMessage) error {
		finalizeCalls++
		if finalizeCalls == 1 {
			return finalizeErr
		}
		return nil
	}
	_, err = editService.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation", "connection",
		lease.TargetRevision, "after", nil, nil, nil, nil, finalize,
	)
	require.ErrorIs(t, err, finalizeErr)
	require.NoError(t, transferService.TransferSessionWithDurablePreparation(
		ctx, entry.TaskID, entry.SessionID, "session-new", nil, nil,
	))

	_, err = editService.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation", "connection",
		lease.TargetRevision, "after", nil, nil, nil, nil, finalize,
	)

	require.ErrorIs(t, err, ErrEntryNotFound)
	assert.Equal(t, 1, finalizeCalls)
}

func TestRemoteEditLeaseBlocksHeadReservation(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	editService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	drainService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-1", "task", "queued", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	_, err = editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)

	reserved, ok := drainService.ReserveQueued(ctx, entry.SessionID)

	assert.False(t, ok)
	assert.Nil(t, reserved)
	status := editService.GetStatus(ctx, entry.SessionID)
	require.Len(t, status.Entries, 1)
	assert.Equal(t, entry.ID, status.Entries[0].ID)
}

func TestRemoteEditLeaseBlocksTargetedDrain(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	editService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	drainService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-1", "task", "queued", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	_, err = editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)

	taken, ok, err := drainService.TakeQueuedEntry(ctx, entry.SessionID, entry.ID)

	require.ErrorIs(t, err, ErrEditConflict)
	assert.False(t, ok)
	assert.Nil(t, taken)
}

func TestRemoteEditLeaseBlocksSendNowClaim(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	editService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	sendNowService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-1", "task", "queued", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	_, err = editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)

	claim, err := sendNowService.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry})

	require.ErrorIs(t, err, ErrEditConflict)
	assert.Nil(t, claim)
}

func TestRemoteEditLeaseBlocksEntryRemoval(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	editService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	removeService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-1", "task", "queued", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	_, err = editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)

	removed, err := removeService.RemoveEntryWithEntry(ctx, entry.SessionID, entry.ID)

	require.ErrorIs(t, err, ErrEditConflict)
	assert.Nil(t, removed)
}

func TestRemoteDisconnectReleasesDurableEditLease(t *testing.T) {
	ctx := context.Background()
	repository := newTestSQLiteRepo(t)
	persistent := repository.(*sqliteRepository)
	secondRepository, err := NewSQLiteRepository(persistent.db, persistent.ro)
	require.NoError(t, err)
	firstService := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
	secondService := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
	entry, err := firstService.QueueMessage(
		ctx, "session-1", "task", "queued", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	_, err = firstService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)

	require.Equal(t, 1, firstService.ReleaseEditLeasesForConnection("connection"))
	_, err = secondService.BeginEdit(ctx, entry.SessionID, entry.ID, "new-connection")

	require.NoError(t, err)
}

func TestRemoteEditLeaseBlocksUncoveredMutations(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		ctx := context.Background()
		repository := newTestSQLiteRepo(t).(*sqliteRepository)
		secondRepository, err := NewSQLiteRepository(repository.db, repository.ro)
		require.NoError(t, err)
		owner := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
		other := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
		entry, err := owner.QueueMessage(ctx, "session-1", "task", "before", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		_, err = owner.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
		require.NoError(t, err)

		err = other.UpdateMessageWithMetadata(ctx, entry.SessionID, entry.ID, "after", nil, nil, QueuedByUser)

		require.ErrorIs(t, err, ErrEditConflict)
	})

	t.Run("append", func(t *testing.T) {
		ctx := context.Background()
		repository := newTestSQLiteRepo(t).(*sqliteRepository)
		secondRepository, err := NewSQLiteRepository(repository.db, repository.ro)
		require.NoError(t, err)
		owner := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
		other := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
		entry, err := owner.QueueMessage(ctx, "session-1", "task", "before", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		_, err = owner.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
		require.NoError(t, err)

		_, _, err = other.AppendContent(ctx, entry.SessionID, entry.TaskID, "after", "", QueuedByUser, false, nil)

		require.ErrorIs(t, err, ErrEditConflict)
	})

	t.Run("coalesce replacement", func(t *testing.T) {
		ctx := context.Background()
		repository := newTestSQLiteRepo(t).(*sqliteRepository)
		secondRepository, err := NewSQLiteRepository(repository.db, repository.ro)
		require.NoError(t, err)
		owner := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
		other := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
		entry, _, err := owner.QueueMessageWithCoalesceKey(ctx, "session-1", "task", "before", "", QueuedByUser, false, nil, nil, "key", true)
		require.NoError(t, err)
		lease, err := owner.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
		require.NoError(t, err)

		replacement, replaced, err := other.QueueMessageWithCoalesceKey(ctx, entry.SessionID, entry.TaskID, "after", "", QueuedByUser, false, nil, nil, "key", true)
		require.NoError(t, err)
		require.True(t, replaced)
		require.Equal(t, entry.ID, replacement.ID)
		_, err = owner.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation", "connection", lease.TargetRevision, "stale", nil, nil)
		require.ErrorIs(t, err, ErrEditLeaseNotFound)
	})

	t.Run("automatic merge", func(t *testing.T) {
		ctx := context.Background()
		repository := newTestSQLiteRepo(t).(*sqliteRepository)
		secondRepository, err := NewSQLiteRepository(repository.db, repository.ro)
		require.NoError(t, err)
		owner := newAutoMergeTestServiceWithRepository(t, repository, DefaultMaxPerSession)
		other := newAutoMergeTestServiceWithRepository(t, secondRepository, DefaultMaxPerSession)
		owner.SetAutoMergeEnabled(false)
		first, err := owner.QueueMessage(ctx, "session-1", "task", "first", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		second, err := owner.QueueMessage(ctx, first.SessionID, first.TaskID, "second", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		_, err = owner.BeginEdit(ctx, second.SessionID, second.ID, "connection")
		require.NoError(t, err)
		other.SetAutoMergeEnabled(true)

		queued, err := other.QueueMessage(ctx, first.SessionID, first.TaskID, "third", "", QueuedByUser, false, nil)

		require.NoError(t, err)
		require.NotEqual(t, first.ID, queued.ID)
		status := owner.GetStatus(ctx, first.SessionID)
		require.Len(t, status.Entries, 3)
		require.Equal(t, "first", status.Entries[0].Content)
	})

	t.Run("full queue candidate merge", func(t *testing.T) {
		ctx := context.Background()
		repository := newTestSQLiteRepo(t).(*sqliteRepository)
		secondRepository, err := NewSQLiteRepository(repository.db, repository.ro)
		require.NoError(t, err)
		owner := newAutoMergeTestServiceWithRepository(t, repository, 1)
		other := newAutoMergeTestServiceWithRepository(t, secondRepository, 1)
		first, err := owner.QueueMessage(ctx, "session-1", "task", "first", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		_, err = owner.BeginEdit(ctx, first.SessionID, first.ID, "connection")
		require.NoError(t, err)

		_, err = other.QueueMessage(ctx, first.SessionID, first.TaskID, "second", "", QueuedByUser, false, nil)

		require.ErrorIs(t, err, ErrQueueFull)
		status := owner.GetStatus(ctx, first.SessionID)
		require.Len(t, status.Entries, 1)
		require.Equal(t, "first", status.Entries[0].Content)
	})
}
