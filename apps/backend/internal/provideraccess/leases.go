package provideraccess

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrLeaseConflict    = errors.New("provider access lease idempotency conflict")
	ErrLeaseTooLong     = errors.New("provider access lease exceeds maximum lifetime")
	ErrGrantUnavailable = errors.New("provider access grant unavailable")
)

const maxProviderAccessLeaseLifetime = 5 * time.Minute

const leaseColumns = `id, grant_id, grant_generation, scope_key, managed_task_id,
 session_id, target_digest, approval_revision, connection_generation,
 idempotency_hash, expires_at, revoked_at, created_at`

// LeaseClaim requests a non-secret lease for one exact provider target.
type LeaseClaim struct {
	ID                   string
	GrantID              string
	GrantGeneration      int64
	Scope                GrantScope
	ManagedTaskID        string
	SessionID            string
	TargetDigest         string
	ApprovalRevision     uint64
	ConnectionGeneration string
	IdempotencyKey       string
	ExpiresAt            time.Time
}

// Lease is an opaque, non-secret admission receipt, not a provider credential.
type Lease struct {
	ID                   string
	GrantID              string
	GrantGeneration      int64
	Scope                GrantScope
	ManagedTaskID        string
	SessionID            string
	TargetDigest         string
	ApprovalRevision     uint64
	ConnectionGeneration string
	ExpiresAt            time.Time
	RevokedAt            *time.Time
}

type leaseRow struct {
	ID                   string        `db:"id"`
	GrantID              string        `db:"grant_id"`
	GrantGeneration      int64         `db:"grant_generation"`
	ScopeKey             string        `db:"scope_key"`
	ManagedTaskID        string        `db:"managed_task_id"`
	SessionID            string        `db:"session_id"`
	TargetDigest         string        `db:"target_digest"`
	ApprovalRevision     int64         `db:"approval_revision"`
	ConnectionGeneration string        `db:"connection_generation"`
	IdempotencyHash      string        `db:"idempotency_hash"`
	ExpiresAt            int64         `db:"expires_at"`
	RevokedAt            sql.NullInt64 `db:"revoked_at"`
	CreatedAt            int64         `db:"created_at"`
}

func (r leaseRow) lease(scope GrantScope) *Lease {
	result := &Lease{
		ID: r.ID, GrantID: r.GrantID, GrantGeneration: r.GrantGeneration,
		Scope: scope, ManagedTaskID: r.ManagedTaskID, SessionID: r.SessionID,
		TargetDigest: r.TargetDigest, ApprovalRevision: uint64(r.ApprovalRevision),
		ConnectionGeneration: r.ConnectionGeneration,
		ExpiresAt:            time.Unix(r.ExpiresAt, 0).UTC(),
	}
	if r.RevokedAt.Valid {
		revoked := time.Unix(r.RevokedAt.Int64, 0).UTC()
		result.RevokedAt = &revoked
	}
	return result
}

// IssueLease atomically admits or replays one non-credential lease under a live grant.
func (s *Store) IssueLease(ctx context.Context, claim LeaseClaim) (*Lease, error) {
	key, err := validateLeaseClaim(claim)
	if err != nil {
		return nil, err
	}
	idempotency := sha256.Sum256([]byte(claim.IdempotencyKey))
	idempotencyHash := hex.EncodeToString(idempotency[:])
	now := time.Now().UTC()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// This write serializes admission with replacement and revocation on both databases.
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_access_grants
  SET updated_at = updated_at WHERE id = ? AND scope_key = ? AND generation = ?
  AND revoked_at IS NULL AND expires_at > ?`),
		claim.GrantID, key, claim.GrantGeneration, now.Unix())
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrGrantUnavailable
	}
	if err := checkLeaseWithinGrant(ctx, tx, claim); err != nil {
		return nil, err
	}
	var existing leaseRow
	err = tx.GetContext(ctx, &existing, tx.Rebind(`SELECT `+leaseColumns+`
  FROM provider_access_leases WHERE grant_id = ? AND idempotency_hash = ?`),
		claim.GrantID, idempotencyHash)
	if err == nil {
		if !existing.matches(claim, key) || existing.RevokedAt.Valid || existing.ExpiresAt <= now.Unix() {
			return nil, ErrLeaseConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return existing.lease(claim.Scope), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO provider_access_leases (`+leaseColumns+`)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		claim.ID, claim.GrantID, claim.GrantGeneration, key, claim.ManagedTaskID,
		claim.SessionID, claim.TargetDigest, int64(claim.ApprovalRevision),
		claim.ConnectionGeneration, idempotencyHash, claim.ExpiresAt.Unix(), nil, now.Unix())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return (&leaseRow{
		ID: claim.ID, GrantID: claim.GrantID, GrantGeneration: claim.GrantGeneration,
		ManagedTaskID: claim.ManagedTaskID, SessionID: claim.SessionID,
		TargetDigest: claim.TargetDigest, ApprovalRevision: int64(claim.ApprovalRevision),
		ConnectionGeneration: claim.ConnectionGeneration,
		ExpiresAt:            claim.ExpiresAt.Unix(),
	}).lease(claim.Scope), nil
}

func checkLeaseWithinGrant(ctx context.Context, tx *sqlx.Tx, claim LeaseClaim) error {
	var grantExpiresAt int64
	if err := tx.GetContext(ctx, &grantExpiresAt, tx.Rebind(`SELECT expires_at
  FROM provider_access_grants WHERE id = ?`), claim.GrantID); err != nil {
		return err
	}
	if claim.ExpiresAt.Unix() > grantExpiresAt {
		return ErrGrantUnavailable
	}
	return nil
}

func validateLeaseClaim(claim LeaseClaim) (string, error) {
	key, err := scopeKey(claim.Scope)
	if err != nil {
		return "", err
	}
	for _, value := range []string{claim.ID, claim.GrantID, claim.ManagedTaskID,
		claim.SessionID, claim.TargetDigest, claim.ConnectionGeneration, claim.IdempotencyKey} {
		if value == "" || value != strings.TrimSpace(value) {
			return "", errors.New("complete provider access lease claim is required")
		}
	}
	now := time.Now()
	if claim.GrantGeneration <= 0 || claim.ApprovalRevision == 0 ||
		claim.ApprovalRevision > math.MaxInt64 || !claim.ExpiresAt.After(now) {
		return "", errors.New("valid provider access lease generation, approval and expiry are required")
	}
	if claim.ExpiresAt.After(now.Add(maxProviderAccessLeaseLifetime)) {
		return "", ErrLeaseTooLong
	}
	return key, nil
}

func (r leaseRow) matches(claim LeaseClaim, key string) bool {
	return r.GrantGeneration == claim.GrantGeneration && r.ScopeKey == key &&
		r.ManagedTaskID == claim.ManagedTaskID && r.SessionID == claim.SessionID &&
		r.TargetDigest == claim.TargetDigest && r.ApprovalRevision == int64(claim.ApprovalRevision) &&
		r.ConnectionGeneration == claim.ConnectionGeneration && r.ExpiresAt == claim.ExpiresAt.Unix()
}

type activeLeaseRow struct {
	leaseRow
	PluginInstallationID string `db:"plugin_installation_id"`
	PluginID             string `db:"plugin_id"`
	WorkspaceID          string `db:"workspace_id"`
	ConversationKey      string `db:"conversation_key"`
	TargetTaskID         string `db:"target_task_id"`
	RepositoryID         string `db:"repository_id"`
	Provider             string `db:"provider"`
	Purpose              string `db:"purpose"`
}

// GetActiveLease is a single-snapshot inspection read, not final redemption authority.
func (s *Store) GetActiveLease(ctx context.Context, id string) (*Lease, error) {
	var row activeLeaseRow
	now := time.Now().Unix()
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT l.*,
  g.plugin_installation_id, g.plugin_id, g.workspace_id, g.conversation_key,
  g.target_task_id, g.repository_id, g.provider, g.purpose
  FROM provider_access_leases l JOIN provider_access_grants g ON g.id = l.grant_id
  WHERE l.id = ? AND l.revoked_at IS NULL AND l.expires_at > ?
  AND g.revoked_at IS NULL AND g.expires_at > ? AND g.generation = l.grant_generation`),
		id, now, now)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.lease(GrantScope{
		PluginInstallationID: row.PluginInstallationID, PluginID: row.PluginID,
		WorkspaceID: row.WorkspaceID, ConversationKey: row.ConversationKey,
		TargetTaskID: row.TargetTaskID, RepositoryID: row.RepositoryID,
		Provider: row.Provider, Purpose: row.Purpose,
	}), nil
}
