package secrets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/db/dialect"
	"io"
	"time"
)

const (
	// SessionMCPKeyID is the fixed install-wide ID of the session MCP derivation
	// key. The key lives in its own table, so no user-facing secret path
	// (list, read, update, delete) can reach it.
	SessionMCPKeyID = "kandev-session-mcp-key-v1"
	// SessionMCPKeySize is the key length in bytes.
	SessionMCPKeySize = 32
)

// ErrSessionKeyUnusable marks a stored key that cannot be decrypted or is not
// SessionMCPKeySize bytes. Only EnsureSessionMCPKey replaces such a key.
var ErrSessionKeyUnusable = errors.New("session mcp key is unusable")

// SessionKeyStore holds the backend-only key from which per-instance MCP tokens
// are derived.
type SessionKeyStore interface {
	// SessionMCPKey reads the key without creating or replacing it. It returns
	// ErrNotFound when absent and ErrSessionKeyUnusable when the stored key
	// cannot be decrypted or has the wrong length; any other error is a store
	// failure.
	SessionMCPKey(ctx context.Context) ([]byte, error)
	// EnsureSessionMCPKey returns the key, creating it when absent (two
	// concurrent first uses end with one key) and atomically replacing an
	// unusable one. replaced reports that this call replaced an unusable key.
	EnsureSessionMCPKey(ctx context.Context) (key []byte, replaced bool, err error)
}

var _ SessionKeyStore = (*sqliteStore)(nil)

func (s *sqliteStore) initSessionKeySchema() error {
	binaryType := dialect.BlobType(s.db.DriverName())
	_, err := s.db.Exec(fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS session_mcp_key (
		id              TEXT PRIMARY KEY,
		encrypted_value %s NOT NULL,
		nonce           %s NOT NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`, binaryType, binaryType))
	return err
}

type sessionKeyRow struct {
	ciphertext []byte
	nonce      []byte
}

func (s *sqliteStore) readSessionKeyRow(ctx context.Context) (*sessionKeyRow, error) {
	var row sessionKeyRow
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT encrypted_value, nonce FROM session_mcp_key WHERE id = ?`), SessionMCPKeyID).
		Scan(&row.ciphertext, &row.nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, SessionMCPKeyID)
	}
	if err != nil {
		return nil, fmt.Errorf("read session mcp key: %w", err)
	}
	return &row, nil
}

func (s *sqliteStore) openSessionKey(row *sessionKeyRow) ([]byte, error) {
	key, err := Decrypt(row.ciphertext, row.nonce, s.crypto.Key())
	if err != nil || len(key) != SessionMCPKeySize {
		return nil, ErrSessionKeyUnusable
	}
	return key, nil
}

// SessionMCPKey implements SessionKeyStore.
func (s *sqliteStore) SessionMCPKey(ctx context.Context) ([]byte, error) {
	row, err := s.readSessionKeyRow(ctx)
	if err != nil {
		return nil, err
	}
	return s.openSessionKey(row)
}

// EnsureSessionMCPKey implements SessionKeyStore. Callers race through the
// database, not a process lock: the create is INSERT ... ON CONFLICT DO NOTHING
// and a replacement is a compare-and-swap on the exact stored row, so the loser
// of either race rereads the winner's key.
func (s *sqliteStore) EnsureSessionMCPKey(ctx context.Context) ([]byte, bool, error) {
	row, err := s.readSessionKeyRow(ctx)
	switch {
	case err == nil:
		if key, kerr := s.openSessionKey(row); kerr == nil {
			return key, false, nil
		}
		return s.replaceSessionKey(ctx, row)
	case errors.Is(err, ErrNotFound):
		return s.createSessionKey(ctx)
	default:
		return nil, false, err
	}
}

func (s *sqliteStore) newSessionKeyCiphertext() (key, ciphertext, nonce []byte, err error) {
	key = make([]byte, SessionMCPKeySize)
	if _, err = io.ReadFull(rand.Reader, key); err != nil {
		return nil, nil, nil, fmt.Errorf("generate session mcp key: %w", err)
	}
	ciphertext, nonce, err = Encrypt(key, s.crypto.Key())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encrypt session mcp key: %w", err)
	}
	return key, ciphertext, nonce, nil
}

func (s *sqliteStore) createSessionKey(ctx context.Context) ([]byte, bool, error) {
	_, ct, nonce, err := s.newSessionKeyCiphertext()
	if err != nil {
		return nil, false, err
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO session_mcp_key (id, encrypted_value, nonce, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`), SessionMCPKeyID, ct, nonce, time.Now().UTC()); err != nil {
		return nil, false, fmt.Errorf("insert session mcp key: %w", err)
	}
	stored, err := s.SessionMCPKey(ctx)
	return stored, false, err
}

func (s *sqliteStore) replaceSessionKey(ctx context.Context, old *sessionKeyRow) ([]byte, bool, error) {
	_, ct, nonce, err := s.newSessionKeyCiphertext()
	if err != nil {
		return nil, false, err
	}
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE session_mcp_key SET encrypted_value = ?, nonce = ?, created_at = ?
		WHERE id = ? AND encrypted_value = ? AND nonce = ?`),
		ct, nonce, time.Now().UTC(), SessionMCPKeyID, old.ciphertext, old.nonce)
	if err != nil {
		return nil, false, fmt.Errorf("replace session mcp key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("replace session mcp key: %w", err)
	}
	// The stored key is the winner's when a concurrent caller replaced it first.
	stored, err := s.SessionMCPKey(ctx)
	return stored, n == 1, err
}
