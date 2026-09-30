package provideraccess

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrLeaseAlreadyRedeemed = errors.New("provider access lease already redeemed")

// MintClaim is derived from current Host authority, never from a plugin's
// asserted task, session, approval, or provider connection.
type MintClaim struct {
	LeaseID  string
	GrantID  string
	Expected FinalLeaseIdentity
}

// MintIntent is durable evidence that a provider call may have created a token.
// A persisted intent cannot be retried, including after a Host restart.
type MintIntent struct {
	LeaseID                string
	MintStartedAt          time.Time
	PossibleProviderExpiry time.Time
}

// ClaimMintIntent atomically verifies the current lease and records a one-shot
// intent before any call to the provider. A lost response remains unknown until
// the conservative provider expiry, and no retry can mint a second token.
func (s *Store) ClaimMintIntent(ctx context.Context, claim MintClaim) (*MintIntent, error) {
	if claim.LeaseID == "" || claim.GrantID == "" ||
		claim.LeaseID != strings.TrimSpace(claim.LeaseID) ||
		claim.GrantID != strings.TrimSpace(claim.GrantID) ||
		claim.Expected.GrantGeneration <= 0 ||
		claim.Expected.ApprovalRevision == 0 ||
		claim.Expected.ApprovalRevision > math.MaxInt64 {
		return nil, ErrGrantUnavailable
	}
	key, err := scopeKey(claim.Expected.Scope)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockMintClaimRows(ctx, tx, claim, key, now); err != nil {
		return nil, err
	}
	var existing int64
	err = tx.GetContext(ctx, &existing, tx.Rebind(`SELECT mint_started_at
  FROM provider_access_redemptions WHERE lease_id = ?`), claim.LeaseID)
	if err == nil {
		return nil, ErrLeaseAlreadyRedeemed
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// GitHub installation tokens expire within an hour. The extra minute covers
	// provider and Host clock skew without claiming a token was revoked.
	possibleExpiry := now.Add(time.Hour + time.Minute)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO provider_access_redemptions
  (lease_id, grant_id, mint_started_at, possible_provider_expiry)
  VALUES (?, ?, ?, ?)`), claim.LeaseID, claim.GrantID, now.Unix(), possibleExpiry.Unix())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MintIntent{LeaseID: claim.LeaseID, MintStartedAt: now,
		PossibleProviderExpiry: possibleExpiry}, nil
}

func lockMintClaimRows(ctx context.Context, tx *sqlx.Tx, claim MintClaim, key string, now time.Time) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_access_grants
  SET updated_at = updated_at WHERE id = ? AND scope_key = ? AND generation = ?
  AND revoked_at IS NULL AND expires_at > ?`),
		claim.GrantID, key, claim.Expected.GrantGeneration, now.Unix())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrGrantUnavailable
	}
	result, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_access_leases
  SET created_at = created_at WHERE id = ? AND grant_id = ?
  AND grant_generation = ? AND scope_key = ? AND managed_task_id = ?
  AND session_id = ? AND target_digest = ? AND approval_revision = ?
  AND connection_generation = ? AND revoked_at IS NULL AND expires_at > ?`),
		claim.LeaseID, claim.GrantID, claim.Expected.GrantGeneration, key,
		claim.Expected.ManagedTaskID, claim.Expected.SessionID,
		claim.Expected.TargetDigest, int64(claim.Expected.ApprovalRevision),
		claim.Expected.ConnectionGeneration, now.Unix())
	if err != nil {
		return err
	}
	count, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrGrantUnavailable
	}
	return nil
}

// GetMintIntent reports uncertainty without exposing or reconstructing a token.
func (s *Store) GetMintIntent(ctx context.Context, leaseID string) (*MintIntent, error) {
	var row struct {
		MintStartedAt          int64 `db:"mint_started_at"`
		PossibleProviderExpiry int64 `db:"possible_provider_expiry"`
	}
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT mint_started_at, possible_provider_expiry
  FROM provider_access_redemptions WHERE lease_id = ?`), leaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &MintIntent{LeaseID: leaseID,
		MintStartedAt:          time.Unix(row.MintStartedAt, 0).UTC(),
		PossibleProviderExpiry: time.Unix(row.PossibleProviderExpiry, 0).UTC()}, nil
}
