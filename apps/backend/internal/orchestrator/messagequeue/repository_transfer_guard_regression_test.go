package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The exported transfer guard is the cross-package fence task repositories
// rely on: while a durable compensation row exists, attachment
// release/transfer/cleanup mutations on either session must fail closed.
func TestGuardSessionTransferRejectsActiveTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		DeleteSessionTransferCompensation(context.Context, string, string, string, string) error
	})
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "guard-regression-op",
		TaskID:      "guard-regression-task", FromSessionID: "guard-old", ToSessionID: "guard-new",
	}))

	guard := func(sessionID string) error {
		tx, err := sqlRepo.db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		// LockSessionInTransaction embeds the transfer guard, so the lock
		// itself must fail closed on either transfer session.
		return LockSessionInTransaction(ctx, tx, sqlRepo.db, sessionID)
	}
	require.ErrorIs(t, guard("guard-old"), ErrSessionTransferInProgress)
	require.ErrorIs(t, guard("guard-new"), ErrSessionTransferInProgress)
	require.NoError(t, guard("guard-unrelated"))

	// The exported guard also fails closed when the caller holds the raw
	// session lock without going through LockSessionInTransaction.
	func() {
		tx, err := sqlRepo.db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		require.NoError(t, lockSessionTxIn(ctx, tx, sqlRepo.db, "guard-old"))
		require.ErrorIs(t, GuardSessionTransferInTransaction(ctx, tx, sqlRepo.db, "guard-old"), ErrSessionTransferInProgress)
	}()

	require.NoError(t, persistence.DeleteSessionTransferCompensation(
		ctx, "guard-regression-op", "guard-regression-task", "guard-old", "guard-new",
	))
	require.NoError(t, guard("guard-old"))
	require.NoError(t, guard("guard-new"))
}
