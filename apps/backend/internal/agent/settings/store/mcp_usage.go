package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"strings"
)

var _ mcpconfig.CatalogUsageRepository = (*sqliteRepository)(nil)

// ListMCPUsageCandidates reads only direct references. Task ownership and access
// are checked by the task service after these database rows have been closed.
func (r *sqliteRepository) ListMCPUsageCandidates(ctx context.Context, workspaceID, definitionID string, after mcpconfig.UsageReference, limit int) ([]mcpconfig.UsageReference, error) {
	if limit < 1 || limit > 201 {
		return nil, mcpconfig.ErrMCPInvalidSelection
	}
	scopes := []mcpconfig.SelectionScope{mcpconfig.SelectionScopeProfile, mcpconfig.SelectionScopeRepository, mcpconfig.SelectionScopeTask, mcpconfig.SelectionScopeTaskSession}
	queries := make([]string, 0, len(scopes))
	args := []any{}
	for _, scope := range scopes {
		table, err := selectionTable(scope)
		if err != nil {
			return nil, err
		}
		query := `SELECT ? AS scope, s.` + table.ownerColumn + ` AS owner_id FROM ` + table.name + ` s JOIN mcp_server_definitions d ON d.id = s.mcp_server_id WHERE d.workspace_id = ? AND d.id = ?`
		args = append(args, string(scope), workspaceID, definitionID)
		if scope == mcpconfig.SelectionScopeProfile {
			query += ` AND s.workspace_id = ?`
			args = append(args, workspaceID)
		}
		queries = append(queries, query)
	}
	query := `SELECT scope, owner_id FROM (` + strings.Join(queries, ` UNION ALL `) + `) usage_refs WHERE (scope > ? OR (scope = ? AND owner_id > ?)) ORDER BY scope, owner_id LIMIT ?`
	args = append(args, string(after.Scope), string(after.Scope), after.OwnerID, limit)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	refs := []mcpconfig.UsageReference{}
	for rows.Next() {
		var ref mcpconfig.UsageReference
		if err := rows.Scan(&ref.Scope, &ref.OwnerID); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// GetMCPUsageProfile reads navigation metadata without loading configuration,
// secret bindings, or running the ordinary profile's legacy backfill.
func (r *sqliteRepository) GetMCPUsageProfile(ctx context.Context, workspaceID, profileID string) (*mcpconfig.UsageOwner, error) {
	owner := &mcpconfig.UsageOwner{UsageReference: mcpconfig.UsageReference{Scope: mcpconfig.SelectionScopeProfile, OwnerID: profileID}, WorkspaceID: workspaceID, OwnerState: mcpconfig.UsageOwnerAvailable}
	var enabled int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT name, agent_id, enabled FROM agent_profiles WHERE id = ? AND deleted_at IS NULL AND (workspace_id = '' OR workspace_id = ?)`), profileID, workspaceID).Scan(&owner.DisplayName, &owner.AgentID, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if enabled == 0 {
		owner.OwnerState = mcpconfig.UsageOwnerDisabled
	}
	return owner, nil
}
