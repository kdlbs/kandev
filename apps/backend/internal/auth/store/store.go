package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("auth: not found")

// Store persists auth entities. Reads go through the reader pool; writes and
// read-after-write go through the writer.
type Store struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// New creates the auth store and initializes its schema.
func New(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// --- identities ---

// CreateIdentity inserts a login identity for a user.
func (s *Store) CreateIdentity(ctx context.Context, identity *LoginIdentity) error {
	if identity.ID == "" {
		identity.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	identity.CreatedAt = now
	identity.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO auth_identities (id, user_id, provider, subject, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), identity.ID, identity.UserID, identity.Provider, identity.Subject, identity.PasswordHash, identity.CreatedAt, identity.UpdatedAt)
	return err
}

// GetLocalIdentity returns the local (email+password) identity for a user.
func (s *Store) GetLocalIdentity(ctx context.Context, userID string) (*LoginIdentity, error) {
	var identity LoginIdentity
	err := s.ro.GetContext(ctx, &identity, s.ro.Rebind(`
		SELECT * FROM auth_identities WHERE user_id = ? AND provider = ?
	`), userID, ProviderLocal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &identity, err
}

// GetIdentityByProviderSubject returns the identity for an external provider's
// subject claim (an OIDC issuer / SAML entity as provider, the IdP subject as
// subject). ErrNotFound when no row matches. The (provider, subject) pair is
// unique, so this resolves at most one identity.
func (s *Store) GetIdentityByProviderSubject(ctx context.Context, provider, subject string) (*LoginIdentity, error) {
	var identity LoginIdentity
	err := s.ro.GetContext(ctx, &identity, s.ro.Rebind(`
		SELECT * FROM auth_identities WHERE provider = ? AND subject = ?
	`), provider, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &identity, err
}

// UpdatePasswordHash replaces the password hash of a user's local identity.
func (s *Store) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE auth_identities SET password_hash = ?, updated_at = ?
		WHERE user_id = ? AND provider = ?
	`), passwordHash, time.Now().UTC(), userID, ProviderLocal)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CountAdminIdentities reports how many active admin users have a local
// identity. Zero means the instance is in setup mode when auth is required.
func (s *Store) CountAdminIdentities(ctx context.Context) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(1)
		FROM auth_identities i
		JOIN users u ON u.id = i.user_id
		WHERE u.role = ? AND u.status = ?
	`), "admin", "active")
	return count, err
}

// --- sessions ---

// CreateSession inserts a browser session.
func (s *Store) CreateSession(ctx context.Context, session *Session) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO auth_sessions (id, user_id, token_sha256, created_at, expires_at, last_seen_at, user_agent, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), session.ID, session.UserID, session.TokenHash, session.CreatedAt, session.ExpiresAt, session.LastSeenAt, session.UserAgent, session.IP)
	return err
}

// GetSessionByTokenHash resolves a session by credential digest.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	var session Session
	err := s.ro.GetContext(ctx, &session, s.ro.Rebind(`
		SELECT * FROM auth_sessions WHERE token_sha256 = ?
	`), tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &session, err
}

// TouchSession updates activity timestamps (sliding expiry) and refreshes the
// stored client IP when a non-empty one is passed (empty never clobbers a
// recorded value).
func (s *Store) TouchSession(ctx context.Context, id string, lastSeen, expires time.Time, ip string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE auth_sessions SET last_seen_at = ?, expires_at = ?,
			ip = CASE WHEN ? = '' THEN ip ELSE ? END
		WHERE id = ?
	`), lastSeen, expires, ip, ip, id)
	return err
}

// ListSessionsByUser returns a user's sessions, newest first.
func (s *Store) ListSessionsByUser(ctx context.Context, userID string) ([]*Session, error) {
	sessions := []*Session{}
	err := s.ro.SelectContext(ctx, &sessions, s.ro.Rebind(`
		SELECT * FROM auth_sessions WHERE user_id = ? ORDER BY last_seen_at DESC
	`), userID)
	return sessions, err
}

// DeleteSession removes one session owned by userID.
func (s *Store) DeleteSession(ctx context.Context, id, userID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM auth_sessions WHERE id = ? AND user_id = ?
	`), id, userID)
	return err
}

// DeleteSessionsByUser removes all sessions for a user, optionally keeping one
// (used by change-password to preserve the current session).
func (s *Store) DeleteSessionsByUser(ctx context.Context, userID, keepSessionID string) error {
	if keepSessionID == "" {
		_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM auth_sessions WHERE user_id = ?`), userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM auth_sessions WHERE user_id = ? AND id != ?
	`), userID, keepSessionID)
	return err
}

// DeleteExpiredSessions garbage-collects expired rows.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM auth_sessions WHERE expires_at < ?`), now)
	return err
}

// --- personal access tokens ---

// CreateAPIToken inserts a PAT record.
func (s *Store) CreateAPIToken(ctx context.Context, token *APIToken) error {
	if token.ID == "" {
		token.ID = uuid.New().String()
	}
	token.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO auth_api_tokens (id, user_id, name, prefix, token_sha256, created_at, last_used_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), token.ID, token.UserID, token.Name, token.Prefix, token.TokenHash, token.CreatedAt, token.LastUsedAt, token.ExpiresAt)
	return err
}

// GetAPITokenByHash resolves a PAT by credential digest.
func (s *Store) GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error) {
	var token APIToken
	err := s.ro.GetContext(ctx, &token, s.ro.Rebind(`
		SELECT * FROM auth_api_tokens WHERE token_sha256 = ?
	`), tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &token, err
}

// TouchAPIToken records usage.
func (s *Store) TouchAPIToken(ctx context.Context, id string, usedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE auth_api_tokens SET last_used_at = ? WHERE id = ?
	`), usedAt, id)
	return err
}

// ListAPITokensByUser returns a user's PATs, newest first.
func (s *Store) ListAPITokensByUser(ctx context.Context, userID string) ([]*APIToken, error) {
	tokens := []*APIToken{}
	err := s.ro.SelectContext(ctx, &tokens, s.ro.Rebind(`
		SELECT * FROM auth_api_tokens WHERE user_id = ? ORDER BY created_at DESC
	`), userID)
	return tokens, err
}

// DeleteAPIToken removes one PAT owned by userID.
func (s *Store) DeleteAPIToken(ctx context.Context, id, userID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM auth_api_tokens WHERE id = ? AND user_id = ?
	`), id, userID)
	return err
}

// DeleteAPITokensByUser removes all PATs for a user (account disable cascade).
func (s *Store) DeleteAPITokensByUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM auth_api_tokens WHERE user_id = ?`), userID)
	return err
}

// --- invites ---

// CreateInvite inserts an invite record.
func (s *Store) CreateInvite(ctx context.Context, invite *Invite) error {
	if invite.ID == "" {
		invite.ID = uuid.New().String()
	}
	invite.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO auth_invites (id, token_sha256, email, role, org_id, created_by, created_at, expires_at, used_by, used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), invite.ID, invite.TokenHash, invite.Email, invite.Role, invite.OrgID, invite.CreatedBy, invite.CreatedAt, invite.ExpiresAt, invite.UsedBy, invite.UsedAt)
	return err
}

// GetInviteByTokenHash resolves an invite by credential digest.
func (s *Store) GetInviteByTokenHash(ctx context.Context, tokenHash string) (*Invite, error) {
	var invite Invite
	err := s.ro.GetContext(ctx, &invite, s.ro.Rebind(`
		SELECT * FROM auth_invites WHERE token_sha256 = ?
	`), tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &invite, err
}

// MarkInviteUsed consumes an invite exactly once; returns ErrNotFound if it
// was already used (guards concurrent accepts).
func (s *Store) MarkInviteUsed(ctx context.Context, id, usedBy string, usedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE auth_invites SET used_by = ?, used_at = ? WHERE id = ? AND used_by IS NULL
	`), usedBy, usedAt, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListInvites returns all invites, newest first.
func (s *Store) ListInvites(ctx context.Context) ([]*Invite, error) {
	invites := []*Invite{}
	err := s.ro.SelectContext(ctx, &invites, `SELECT * FROM auth_invites ORDER BY created_at DESC`)
	return invites, err
}

// DeleteInvite removes an invite.
func (s *Store) DeleteInvite(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM auth_invites WHERE id = ?`), id)
	return err
}
