package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

const dynamicProfileOwnerKind = "dynamic"

const profileOwnerQuery = `SELECT p.id, p.agent_id, a.name, p.name, p.workspace_id, p.enabled, a.supports_mcp
 FROM agent_profiles p JOIN agents a ON a.id = p.agent_id
 WHERE p.deleted_at IS NULL AND (p.workspace_id = '' OR p.workspace_id = ?)`

// ListMCPProfileOwners reads settings identities without configuration or legacy backfill.
func (r *sqliteRepository) ListMCPProfileOwners(ctx context.Context, workspaceID, after string, limit int) ([]mcpconfig.MCPProfileOwner, error) {
	if limit < 1 || limit > 101 {
		return nil, mcpconfig.ErrMCPInvalidSelection
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(profileOwnerQuery+` AND p.id > ? ORDER BY p.id LIMIT ?`), workspaceID, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []mcpconfig.MCPProfileOwner{}
	for rows.Next() {
		owner, scanErr := scanMCPProfileOwner(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *owner)
	}
	return items, rows.Err()
}
func (r *sqliteRepository) GetMCPProfileOwner(ctx context.Context, workspaceID, profileID string) (*mcpconfig.MCPProfileOwner, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(profileOwnerQuery+` AND p.id = ?`), workspaceID, profileID)
	owner, err := scanMCPProfileOwner(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return owner, err
}
func scanMCPProfileOwner(row interface{ Scan(...any) error }) (*mcpconfig.MCPProfileOwner, error) {
	var owner mcpconfig.MCPProfileOwner
	err := row.Scan(&owner.ID, &owner.AgentID, &owner.AgentName, &owner.Name, &owner.WorkspaceID, &owner.Enabled, &owner.SupportsMCP)
	if err != nil {
		return nil, err
	}
	owner.Kind = "concrete"
	if owner.AgentID == agents.DynamicAgentID {
		owner.Kind = dynamicProfileOwnerKind
	}
	return &owner, nil
}
