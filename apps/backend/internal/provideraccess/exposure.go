package provideraccess

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ExposureReceipt records only the provider authority that has left the Host.
// It never contains token bytes or information sufficient to recreate a token.
type ExposureReceipt struct {
	LeaseID             string
	GrantID             string
	Provider            string
	ProviderPrincipalID string
	RepositoryID        string
	PermissionProfile   string
	ProviderExpiresAt   time.Time
}

// ExposureState separates a fenced lease from a bearer revoked by the provider.
type ExposureState string

const (
	ExposureUnknown           ExposureState = "unknown"
	ExposureActive            ExposureState = "exported"
	ExposureResidual          ExposureState = "expiry_bounded_residual"
	ExposureRevokedAtProvider ExposureState = "revoked_at_provider"
	ExposureProviderExpired   ExposureState = "provider_expired"
)

// RecordExposureReceipt stores a non-secret receipt for one live, exact lease.
// A future redemption path must write this before returning bearer material.
func (s *Store) RecordExposureReceipt(ctx context.Context, receipt ExposureReceipt) error {
	if err := validateExposureReceipt(receipt); err != nil {
		return err
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO provider_access_exposures (
  lease_id, grant_id, provider, provider_principal_id, repository_id,
  permission_profile, provider_expires_at, exported_at)
  SELECT l.id, l.grant_id, g.provider, ?, g.repository_id, ?, ?, ?
  FROM provider_access_leases l JOIN provider_access_grants g ON g.id = l.grant_id
  WHERE l.id = ? AND l.grant_id = ? AND g.provider = ? AND g.repository_id = ?
  AND l.revoked_at IS NULL AND l.expires_at > ? AND g.revoked_at IS NULL
  AND g.expires_at > ? AND g.generation = l.grant_generation`),
		receipt.ProviderPrincipalID, receipt.PermissionProfile,
		receipt.ProviderExpiresAt.Unix(), now.Unix(), receipt.LeaseID,
		receipt.GrantID, receipt.Provider, receipt.RepositoryID, now.Unix(), now.Unix())
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
	return nil
}

func validateExposureReceipt(receipt ExposureReceipt) error {
	for _, value := range []string{receipt.LeaseID, receipt.GrantID, receipt.ProviderPrincipalID,
		receipt.RepositoryID} {
		if value == "" || value != strings.TrimSpace(value) {
			return errors.New("complete provider exposure identity is required")
		}
	}
	if receipt.Provider != "github" || receipt.PermissionProfile != "github_actions_rerun" {
		return errors.New("unsupported provider exposure profile")
	}
	now := time.Now().UTC()
	if !receipt.ProviderExpiresAt.After(now) || receipt.ProviderExpiresAt.After(now.Add(time.Hour+time.Minute)) {
		return errors.New("invalid provider token expiry")
	}
	return nil
}

// RecordRevocationResult records the attempted provider result, not a lease policy decision.
// confirmed must be true only after a successful provider revocation response.
func (s *Store) RecordRevocationResult(ctx context.Context, leaseID string, at time.Time, confirmed bool) error {
	if leaseID == "" || at.IsZero() {
		return errors.New("lease and revocation attempt time are required")
	}
	var confirmedAt any
	if confirmed {
		confirmedAt = at.Unix()
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE provider_access_exposures
  SET revocation_attempted_at = ?, revoked_at_provider = ?
  WHERE lease_id = ? AND revoked_at_provider IS NULL`), at.Unix(), confirmedAt, leaseID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// ExposureStateAt reports provider invalidation or the remaining bearer window.
func (s *Store) ExposureStateAt(ctx context.Context, leaseID string, at time.Time) (ExposureState, error) {
	var row struct {
		ProviderExpiresAt     int64         `db:"provider_expires_at"`
		LeaseExpiresAt        int64         `db:"lease_expires_at"`
		RevocationAttemptedAt sql.NullInt64 `db:"revocation_attempted_at"`
		RevokedAtProvider     sql.NullInt64 `db:"revoked_at_provider"`
		LeaseRevokedAt        sql.NullInt64 `db:"lease_revoked_at"`
		GrantRevokedAt        sql.NullInt64 `db:"grant_revoked_at"`
	}
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT e.provider_expires_at,
  l.expires_at AS lease_expires_at, e.revocation_attempted_at,
  e.revoked_at_provider, l.revoked_at AS lease_revoked_at,
  g.revoked_at AS grant_revoked_at
  FROM provider_access_exposures e
  JOIN provider_access_leases l ON l.id = e.lease_id
  JOIN provider_access_grants g ON g.id = e.grant_id
  WHERE e.lease_id = ?`), leaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ExposureUnknown, nil
	}
	if err != nil {
		return ExposureUnknown, err
	}
	if row.RevokedAtProvider.Valid {
		return ExposureRevokedAtProvider, nil
	}
	if at.Unix() >= row.ProviderExpiresAt {
		return ExposureProviderExpired, nil
	}
	if at.Unix() >= row.LeaseExpiresAt || row.RevocationAttemptedAt.Valid ||
		row.LeaseRevokedAt.Valid || row.GrantRevokedAt.Valid {
		return ExposureResidual, nil
	}
	return ExposureActive, nil
}
