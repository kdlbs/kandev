package messagequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionTransferCompensationRejectsActiveReplacement(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		DeleteSessionTransferCompensation(context.Context, string, string, string, string) error
		ListSessionTransferCompensations(context.Context) ([]SessionTransferCompensation, error)
	})
	first := SessionTransferCompensation{
		OperationID: "transfer-first",
		TaskID:      "task", FromSessionID: "session-old", ToSessionID: "session-new",
		EntryIDs: []string{"entry-first"},
	}
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, first))

	replacement := first
	replacement.OperationID = "transfer-replacement"
	replacement.EntryIDs = []string{"entry-replacement"}
	err := persistence.UpsertSessionTransferCompensation(ctx, replacement)

	require.ErrorIs(t, err, ErrSessionTransferInProgress)
	stored, listErr := persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, first.EntryIDs, stored[0].EntryIDs)
	first.EntryIDs = []string{"entry-final"}
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, first))
	stored, listErr = persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, first.EntryIDs, stored[0].EntryIDs)
	err = persistence.DeleteSessionTransferCompensation(
		ctx, replacement.OperationID, first.TaskID, first.FromSessionID, first.ToSessionID,
	)
	require.ErrorIs(t, err, ErrSessionTransferOwnershipLost)
	stored, listErr = persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, first.OperationID, stored[0].OperationID)
	require.NoError(t, persistence.DeleteSessionTransferCompensation(
		ctx, first.OperationID, first.TaskID, first.FromSessionID, first.ToSessionID,
	))
	stored, listErr = persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	assert.Empty(t, stored)
}

func TestMaintainSessionTransferCompensationLeaseStopIsIdempotent(t *testing.T) {
	ctx := context.Background()
	service := setupService(t)
	compensation := SessionTransferCompensation{
		OperationID:   "transfer-operation",
		TaskID:        "task",
		FromSessionID: "session-old",
		ToSessionID:   "session-new",
	}
	stopCtx, stop := service.MaintainSessionTransferCompensationLease(
		ctx, compensation, compensation.OperationID,
	)
	require.NoError(t, stopCtx.Err())
	require.NoError(t, stop())
	require.NoError(t, stop())
}

func TestDurableSessionTransferRetainsCompensationAfterLeaseLoss(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	service.sessionTransferCompensationLeaseRenewInterval = time.Millisecond
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
	preparationStarted := make(chan struct{})
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- service.TransferSessionWithDurableAttachmentPreparation(
			ctx,
			"task",
			"session-old",
			"session-new",
			func(prepareCtx context.Context, _ []string) error {
				close(preparationStarted)
				<-prepareCtx.Done()
				return prepareCtx.Err()
			},
			func(context.Context, []string) error {
				return errors.New("rollback blocked")
			},
		)
	}()
	<-preparationStarted
	compensations, err := service.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, compensations, 1)
	_, err = sqlRepo.db.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_owner = 'another-owner'
		WHERE operation_id = ?
	`, compensations[0].OperationID)
	require.NoError(t, err)

	select {
	case err = <-transferDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "session transfer lease lost")
	case <-time.After(2 * time.Second):
		t.Fatal("transfer did not stop after lease loss")
	}
	compensations, err = service.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, compensations, 1)
}
func TestDurableSessionTransferKeepsCompensationAfterLeaseLossAndRollback(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	service.sessionTransferCompensationLeaseRenewInterval = time.Millisecond
	entry, err := service.QueueMessage(
		ctx, "session-old", "task", "handoff", "", QueuedByUser, false,
		[]MessageAttachment{{AttachmentID: "attachment"}},
	)
	require.NoError(t, err)
	preparationStarted := make(chan struct{})
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- service.TransferSessionWithDurableAttachmentPreparation(
			ctx, entry.TaskID, entry.SessionID, "session-new",
			func(prepareCtx context.Context, _ []string) error {
				close(preparationStarted)
				<-prepareCtx.Done()
				return prepareCtx.Err()
			},
			nil,
		)
	}()
	<-preparationStarted
	compensations, err := service.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, compensations, 1)
	_, err = sqlRepo.db.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_owner = 'another-owner'
		WHERE operation_id = ?
	`, compensations[0].OperationID)
	require.NoError(t, err)
	select {
	case err = <-transferDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "session transfer lease lost")
	case <-time.After(2 * time.Second):
		t.Fatal("transfer did not stop after lease loss")
	}
	compensations, err = service.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, compensations, 1)
}

func TestEditLeaseCleanupIsDurableAcrossSessionReplacement(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	otherRepo, err := NewSQLiteRepository(sqlRepo.db, sqlRepo.ro)
	require.NoError(t, err)
	editService := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	replaceService := newAutoMergeTestServiceWithRepository(t, otherRepo, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-replace", "task-replace", "before", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	lease, err := editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	replacement := *entry
	replacement.Content = "after"
	require.NoError(t, replaceService.RestoreSession(
		ctx, entry.SessionID, []QueuedMessage{replacement}, nil,
	))
	_, err = editService.UpdateMessageWithLease(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation",
		"connection", lease.TargetRevision, "stale", nil, nil,
	)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
}

func TestEditLeaseCleanupIsDurableAcrossTaskPurge(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	otherRepo, err := NewSQLiteRepository(sqlRepo.db, sqlRepo.ro)
	require.NoError(t, err)
	editService := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	purgeService := newAutoMergeTestServiceWithRepository(t, otherRepo, DefaultMaxPerSession)
	entry, err := editService.QueueMessage(
		ctx, "session-purge", "task-purge", "before", "", QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	lease, err := editService.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	_, err = purgeService.PurgeTask(ctx, entry.TaskID)
	require.NoError(t, err)
	replacement := *entry
	replacement.Content = "restored"
	require.NoError(t, purgeService.RestoreSession(
		ctx, entry.SessionID, []QueuedMessage{replacement}, nil,
	))
	_, err = editService.UpdateMessageWithLease(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation",
		"connection", lease.TargetRevision, "stale", nil, nil,
	)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
}

func TestExpiredSessionTransferLeaseCannotDeleteCompensation(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	compensation := SessionTransferCompensation{
		OperationID:   "expired-operation",
		TaskID:        "task",
		FromSessionID: "session-old",
		ToSessionID:   "session-new",
	}
	require.NoError(t, repo.UpsertSessionTransferCompensation(ctx, compensation))
	_, err := repo.db.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_lease_expires_at = ?
		WHERE operation_id = ?
	`, time.Now().UTC().Add(-time.Second), compensation.OperationID)
	require.NoError(t, err)
	require.ErrorIs(t, repo.DeleteSessionTransferCompensation(
		ctx, compensation.OperationID, compensation.TaskID,
		compensation.FromSessionID, compensation.ToSessionID,
	), ErrSessionTransferOwnershipLost)
	stored, err := repo.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, stored, 1)
}
func TestExpiredSessionTransferLeaseCannotRenew(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	compensation := SessionTransferCompensation{
		OperationID:   "expired-renew-operation",
		TaskID:        "task",
		FromSessionID: "session-old",
		ToSessionID:   "session-new",
	}
	require.NoError(t, repo.UpsertSessionTransferCompensation(ctx, compensation))
	_, err := repo.db.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_lease_expires_at = ?
		WHERE operation_id = ?
	`, time.Now().UTC().Add(-time.Second), compensation.OperationID)
	require.NoError(t, err)

	require.ErrorIs(t, repo.renewSessionTransferCompensationLease(
		ctx, compensation, compensation.OperationID,
	), ErrSessionTransferOwnershipLost)
	ownerID, err := repo.claimSessionTransferCompensationRecovery(ctx, compensation)
	require.NoError(t, err)
	require.NotEqual(t, compensation.OperationID, ownerID)
}

func TestSessionTransferCommitRejectsExpiredLease(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	compensation := SessionTransferCompensation{
		OperationID:   "expired-operation",
		TaskID:        "task",
		FromSessionID: "session-old",
		ToSessionID:   "session-new",
	}
	require.NoError(t, repo.UpsertSessionTransferCompensation(ctx, compensation))
	tx, err := repo.beginAuthorizedSessionTransferTx(
		ctx, compensation.FromSessionID, compensation.ToSessionID, compensation.OperationID,
	)
	require.NoError(t, err)
	_, err = tx.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_lease_expires_at = ?
		WHERE operation_id = ?
	`, time.Now().UTC().Add(-time.Second), compensation.OperationID)
	require.NoError(t, err)
	err = commitAuthorizedSessionTransferTx(
		ctx, tx, repo.db, compensation.FromSessionID,
		compensation.ToSessionID, compensation.OperationID,
	)
	require.ErrorIs(t, err, ErrSessionTransferOwnershipLost)
	require.NoError(t, tx.Rollback())
}
