package store

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// GetAgentProfileTx is GetAgentProfile's transaction-accepting counterpart:
// the automations YAML export (AC-29) opens one read transaction spanning
// several stores and passes it through here rather than letting this method
// open its own, so the profile read observes the same snapshot as every
// other read in the export. A missing row is reported as found=false rather
// than an error - AC-19's partial-resolution rule decides what that means.
func (r *sqliteRepository) GetAgentProfileTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.AgentProfile, bool, error) {
	row := tx.QueryRowContext(ctx, tx.Rebind(agentProfileSelectColumns+` WHERE id = ? AND deleted_at IS NULL`), id)
	profile, err := scanAgentProfile(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return r.applyLegacyBackfill(ctx, profile), true, nil
}
