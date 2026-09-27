package provideraccess

import (
	"context"
	"errors"
	"time"
)

// WorkspaceFenceResult names in-memory tokens that the runtime must attempt
// to revoke. Unknown mints have no token bytes and remain expiry-bounded.
type WorkspaceFenceResult struct {
	ExposedLeaseIDs     []string
	UnknownMintLeaseIDs []string
}

// FenceWorkspace prevents new grants and redemption before workspace rows
// disappear. It removes ordinary ledger data while retaining the grant/lease
// parents of unexpired bearer and unknown-mint receipts until provider expiry.
func (s *Store) FenceWorkspace(ctx context.Context, workspaceID string) (WorkspaceFenceResult, error) {
	if workspaceID == "" {
		return WorkspaceFenceResult{}, errors.New("workspace is required")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return WorkspaceFenceResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockWorkspace(ctx, tx, s.db.DriverName(), workspaceID); err != nil {
		return WorkspaceFenceResult{}, err
	}
	now := time.Now().UTC().Unix()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO provider_access_workspace_fences
  (workspace_id, fenced_at) VALUES (?, ?) ON CONFLICT(workspace_id) DO NOTHING`),
		workspaceID, now); err != nil {
		return WorkspaceFenceResult{}, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_access_grants
  SET revoked_at = ?, updated_at = ? WHERE workspace_id = ? AND revoked_at IS NULL`),
		now, now, workspaceID); err != nil {
		return WorkspaceFenceResult{}, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_access_leases
  SET revoked_at = ? WHERE grant_id IN
  (SELECT id FROM provider_access_grants WHERE workspace_id = ?) AND revoked_at IS NULL`),
		now, workspaceID); err != nil {
		return WorkspaceFenceResult{}, err
	}
	var result WorkspaceFenceResult
	if err := tx.SelectContext(ctx, &result.ExposedLeaseIDs, tx.Rebind(`SELECT e.lease_id
  FROM provider_access_exposures e JOIN provider_access_grants g ON g.id = e.grant_id
  WHERE g.workspace_id = ? AND e.provider_expires_at > ?
  AND e.revoked_at_provider IS NULL`), workspaceID, now); err != nil {
		return WorkspaceFenceResult{}, err
	}
	if err := tx.SelectContext(ctx, &result.UnknownMintLeaseIDs, tx.Rebind(`SELECT r.lease_id
  FROM provider_access_redemptions r
  JOIN provider_access_grants g ON g.id = r.grant_id
  LEFT JOIN provider_access_exposures e ON e.lease_id = r.lease_id
  WHERE g.workspace_id = ? AND r.possible_provider_expiry > ?
  AND e.lease_id IS NULL`), workspaceID, now); err != nil {
		return WorkspaceFenceResult{}, err
	}
	statements := []string{
		`DELETE FROM provider_access_audit WHERE workspace_id = ?`,
		`DELETE FROM provider_access_exposures WHERE grant_id IN
   (SELECT id FROM provider_access_grants WHERE workspace_id = ?)
   AND provider_expires_at <= ?`,
		`DELETE FROM provider_access_redemptions WHERE grant_id IN
   (SELECT id FROM provider_access_grants WHERE workspace_id = ?)
   AND possible_provider_expiry <= ?`,
		`DELETE FROM provider_access_leases WHERE grant_id IN
   (SELECT id FROM provider_access_grants WHERE workspace_id = ?)
   AND id NOT IN (SELECT lease_id FROM provider_access_exposures)
   AND id NOT IN (SELECT lease_id FROM provider_access_redemptions)`,
		`DELETE FROM provider_access_grants WHERE workspace_id = ?
   AND id NOT IN (SELECT grant_id FROM provider_access_leases)`,
	}
	for i, stmt := range statements {
		var execErr error
		if i == 1 || i == 2 {
			_, execErr = tx.ExecContext(ctx, tx.Rebind(stmt), workspaceID, now)
		} else {
			_, execErr = tx.ExecContext(ctx, tx.Rebind(stmt), workspaceID)
		}
		if execErr != nil {
			return WorkspaceFenceResult{}, execErr
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkspaceFenceResult{}, err
	}
	return result, nil
}
