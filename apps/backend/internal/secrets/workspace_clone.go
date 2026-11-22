package secrets

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// CredentialCopier copies one authorized integration credential into a
// caller-owned transaction without exposing its plaintext to the coordinator.
type CredentialCopier interface {
	CopyCredentialTx(context.Context, *sqlx.Tx, string, string) error
}

func (s *sqliteStore) CopyCredentialTx(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) error {
	var name string
	var ciphertext, nonce []byte
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT name, encrypted_value, nonce FROM secrets WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`), sourceID, scopeOwner(ctx), scopeOwner(ctx)).Scan(&name, &ciphertext, &nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	plaintext, err := Decrypt(ciphertext, nonce, s.crypto.Key())
	if err != nil {
		return err
	}
	defer clear(plaintext)
	encrypted, newNonce, err := Encrypt(plaintext, s.crypto.Key())
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO secrets (id,name,user_id,scope,workspace_id,encrypted_value,nonce,created_at,updated_at) VALUES (?,?,?,'global','',?,?,?,?)`), targetID, name, scopeOwner(ctx), encrypted, newNonce, now, now)
	return err
}
