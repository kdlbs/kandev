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
//
// The legacy-backfill path (applyLegacyBackfillTx) must receive this same
// tx too: a legacy Auggie-shaped row's backfill issues a nested read of its
// own (the agent's name), and that nested read is a row the export resolves
// a portable descriptor from just as much as the profile row itself, so
// AC-29 covers it. Routing it through r.ro instead would let it observe a
// different snapshot than the rest of the export.
func (r *sqliteRepository) GetAgentProfileTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.AgentProfile, bool, error) {
	row := tx.QueryRowContext(ctx, tx.Rebind(agentProfileSelectColumns+` WHERE id = ? AND deleted_at IS NULL`), id)
	profile, err := scanAgentProfile(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return r.applyLegacyBackfillTx(ctx, tx, profile), true, nil
}

// GetAgentTx is GetAgent's transaction-accepting counterpart, used only by
// applyLegacyBackfillTx so the legacy backfill's nested agent-name lookup
// observes the same snapshot as the rest of an AC-29 export.
func (r *sqliteRepository) GetAgentTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.Agent, error) {
	row := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, tui_config, created_at, updated_at
		FROM agents WHERE id = ?
	`), id)
	return scanAgent(row)
}

// applyLegacyBackfillTx is applyLegacyBackfill's transaction-accepting
// counterpart. See GetAgentProfileTx for why the nested read needs the tx.
func (r *sqliteRepository) applyLegacyBackfillTx(ctx context.Context, tx *sqlx.Tx, profile *models.AgentProfile) *models.AgentProfile {
	if profile == nil || profile.CLIFlags != nil {
		return profile
	}
	if !profile.AllowIndexing {
		profile.CLIFlags = []models.CLIFlag{}
		return profile
	}
	agent, err := r.GetAgentTx(ctx, tx, profile.AgentID)
	profile.CLIFlags = legacyCLIFlagsForAgent(agent, err)
	return profile
}
