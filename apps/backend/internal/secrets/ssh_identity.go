package secrets

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"strings"
)

// SSHIdentityVault is an installation-owned credential capability. Each write
// participates in the caller's metadata transaction; it cannot address user secrets.
type SSHIdentityVault struct{ crypto *MasterKeyProvider }

func NewSSHIdentityVault(crypto *MasterKeyProvider) *SSHIdentityVault {
	return &SSHIdentityVault{crypto: crypto}
}

func validSSHIdentityID(id string) bool {
	suffix, ok := strings.CutPrefix(id, internalSSHIdentitySecretPrefix)
	return ok && suffix != "" && !strings.ContainsAny(suffix, " \t\n\r:")
}

func (v *SSHIdentityVault) Create(ctx context.Context, tx *sqlx.Tx, id string, key []byte) error {
	if !validSSHIdentityID(id) || len(key) == 0 || len(key) > 65536 {
		return errors.New("invalid SSH identity secret")
	}
	encrypted, nonce, err := Encrypt(key, v.crypto.Key())
	if err != nil {
		return errors.New("SSH credential encryption unavailable")
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO secrets (id,name,user_id,scope,workspace_id,encrypted_value,nonce) VALUES (?,?,'','global','',?,?)`), id, id, encrypted, nonce)
	return err
}

func (v *SSHIdentityVault) Reveal(ctx context.Context, tx *sqlx.Tx, id string) ([]byte, error) {
	if !validSSHIdentityID(id) {
		return nil, ErrNotFound
	}
	var row struct {
		Ciphertext []byte `db:"encrypted_value"`
		Nonce      []byte `db:"nonce"`
	}
	if err := tx.GetContext(ctx, &row, tx.Rebind(`SELECT encrypted_value,nonce FROM secrets WHERE id=?`), id); err != nil {
		return nil, ErrNotFound
	}
	key, err := Decrypt(row.Ciphertext, row.Nonce, v.crypto.Key())
	if err != nil {
		return nil, errors.New("SSH credential unavailable; restore the matching master key")
	}
	return key, nil
}

func (v *SSHIdentityVault) Delete(ctx context.Context, tx *sqlx.Tx, id string) error {
	if !validSSHIdentityID(id) {
		return ErrNotFound
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM secrets WHERE id=?`), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
