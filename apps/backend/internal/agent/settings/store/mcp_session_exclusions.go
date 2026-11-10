package store

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func readMCPSessionExclusionsTx(ctx context.Context, tx *sqlx.Tx, workspaceID, sessionID string) ([]string, error) {
	ids := make([]string, 0)
	err := tx.SelectContext(ctx, &ids, tx.Rebind(`SELECT mcp_server_id FROM task_session_mcp_exclusions WHERE workspace_id = ? AND task_session_id = ? ORDER BY mcp_server_id`), workspaceID, sessionID)
	return ids, err
}

// Positive and excluded definitions share one sorted lock order with catalog deletion.
func lockMCPSelectionDefinitions(ctx context.Context, tx *sqlx.Tx, workspaceID string, ids []string, excluded *[]string) error {
	all := append([]string{}, ids...)
	if excluded != nil {
		all = append(all, (*excluded)...)
	}
	for _, id := range normalizedMCPSelectionIDs(all) {
		if err := lockMCPDefinition(ctx, tx, workspaceID, id); err != nil {
			if errors.Is(err, mcpconfig.ErrMCPServerDefinitionNotFound) {
				return mcpconfig.ErrMCPSelectionWorkspaceMismatch
			}
			return err
		}
	}
	return nil
}

func replaceMCPSessionExclusionsTx(ctx context.Context, tx *sqlx.Tx, origin mcpconfig.SelectionOrigin, previous []string, condition mcpconfig.SelectionCondition) error {
	if condition.ExcludedDefinitionIDs == nil {
		return nil
	}
	rc := condition.ExclusionContext
	if rc == nil || rc.WorkspaceID != origin.WorkspaceID || rc.SessionID != origin.OwnerID {
		return mcpconfig.ErrMCPInvalidSelection
	}
	ids := normalizedMCPSelectionIDs(*condition.ExcludedDefinitionIDs)
	inherited, err := readMCPInheritedIDsTx(ctx, tx, *rc)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !slices.Contains(previous, id) && !slices.Contains(inherited, id) {
			return mcpconfig.ErrMCPInvalidSelection
		}
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_session_mcp_exclusions WHERE workspace_id = ? AND task_session_id = ?`), origin.WorkspaceID, origin.OwnerID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_session_mcp_exclusions (workspace_id, task_session_id, mcp_server_id) VALUES (?, ?, ?)`), origin.WorkspaceID, origin.OwnerID, id); err != nil {
			return err
		}
	}
	return nil
}

func readMCPInheritedIDsTx(ctx context.Context, tx *sqlx.Tx, rc mcpconfig.ResolutionContext) ([]string, error) {
	scopes := []mcpconfig.SelectionOrigin{
		{Scope: mcpconfig.SelectionScopeProfile, WorkspaceID: rc.WorkspaceID, OwnerID: rc.ProfileID},
		{Scope: mcpconfig.SelectionScopeTask, WorkspaceID: rc.WorkspaceID, OwnerID: rc.TaskID},
	}
	for _, id := range rc.RepositoryIDs {
		scopes = append(scopes, mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeRepository, WorkspaceID: rc.WorkspaceID, OwnerID: id})
	}
	var result []string
	for _, scope := range scopes {
		if scope.OwnerID == "" {
			continue
		}
		ids, err := readMCPSelectionIDsTx(ctx, tx, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, ids...)
	}
	return result, nil
}

func (r *sqliteRepository) DeleteMCPSessionData(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return mcpconfig.ErrMCPInvalidSelection
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"task_session_mcp_exclusions", "task_session_mcp_selections", "mcp_task_session_apply_state"} {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM `+table+` WHERE task_session_id = ?`), sessionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
