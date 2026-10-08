package secrets

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

type credentialCloneStore interface {
	CopyCredentialTx(context.Context, *sqlx.Tx, string, string) error
}

func TestWorkspaceCloneCredential(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := t.Context()
	require.NoError(t, store.Create(ctx, &SecretWithValue{Secret: Secret{ID: "source", Name: "GitHub PAT"}, Value: "private-token"}))
	copier, ok := any(store).(credentialCloneStore)
	require.True(t, ok)
	tx, err := store.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, copier.CopyCredentialTx(ctx, tx, "source", "target"))
	var sourceNonce, targetNonce []byte
	require.NoError(t, tx.GetContext(ctx, &sourceNonce, `SELECT nonce FROM secrets WHERE id = 'source'`))
	require.NoError(t, tx.GetContext(ctx, &targetNonce, `SELECT nonce FROM secrets WHERE id = 'target'`))
	require.NotEqual(t, sourceNonce, targetNonce)
	require.NoError(t, tx.Rollback())
	_, err = store.Get(ctx, "target")
	require.ErrorIs(t, err, ErrNotFound)
	tx, err = store.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, copier.CopyCredentialTx(ctx, tx, "source", "target"))
	require.NoError(t, tx.Commit())
	require.NoError(t, store.Delete(ctx, "source"))
	value, err := store.Reveal(ctx, "target")
	require.NoError(t, err)
	require.Equal(t, "private-token", value)
}
