package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/db/dialect"
)

// ReadMCPResolutionInputs freezes catalog definitions and all selecting scopes
// in one read transaction. No secret values or provider calls occur here.
func (r *sqliteRepository) ReadMCPResolutionInputs(ctx context.Context, rc mcpconfig.ResolutionContext) (*mcpconfig.ResolutionInputs, error) {

	tx, err := r.ro.BeginTxx(ctx, mcpResolutionReadOptions(r.ro.DriverName()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	inputs := &mcpconfig.ResolutionInputs{Origins: make(map[string][]mcpconfig.SelectionOrigin), Definitions: make(map[string]*mcpconfig.MCPServerDefinition)}
	scopes := []mcpconfig.SelectionOrigin{{Scope: mcpconfig.SelectionScopeProfile, WorkspaceID: rc.WorkspaceID, OwnerID: rc.ProfileID}, {Scope: mcpconfig.SelectionScopeTask, WorkspaceID: rc.WorkspaceID, OwnerID: rc.TaskID}, {Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: rc.WorkspaceID, OwnerID: rc.SessionID}}
	seenRepos := make(map[string]bool)
	for _, id := range rc.RepositoryIDs {
		if seenRepos[id] {
			continue
		}
		seenRepos[id] = true
		scopes = append(scopes, mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeRepository, WorkspaceID: rc.WorkspaceID, OwnerID: id})
	}
	for _, scope := range scopes {
		if scope.OwnerID == "" {
			continue
		}
		ids, err := readMCPSelectionIDsTx(ctx, tx, scope)
		if err != nil {
			return nil, err
		}
		if scope.Scope == mcpconfig.SelectionScopeTaskSession {
			inputs.SessionSelectionIDs = ids
		}
		for _, id := range ids {
			inputs.Origins[id] = append(inputs.Origins[id], scope)
		}
	}
	for id := range inputs.Origins {
		d, err := scanMCPServerDefinition(tx.QueryRowxContext(ctx, tx.Rebind(`SELECT `+mcpDefinitionColumns+` FROM mcp_server_definitions WHERE workspace_id = ? AND id = ?`), rc.WorkspaceID, id))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		inputs.Definitions[id] = d
	}
	if err := readMCPSessionResolutionInputs(ctx, tx, rc, inputs); err != nil {
		return nil, err
	}

	return inputs, tx.Commit()
}

func readMCPSelectionIDsTx(ctx context.Context, tx *sqlx.Tx, origin mcpconfig.SelectionOrigin) ([]string, error) {
	table, err := selectionTable(origin.Scope)
	if err != nil {
		return nil, err
	}
	query := `SELECT mcp_server_id FROM ` + table.name + ` WHERE ` + table.ownerColumn + ` = ?`
	args := []any{origin.OwnerID}
	if origin.Scope == mcpconfig.SelectionScopeProfile {
		query += mcpSelectionWorkspacePredicate
		args = append(args, origin.WorkspaceID)
	}
	query += ` ORDER BY mcp_server_id`
	ids := make([]string, 0)
	err = tx.SelectContext(ctx, &ids, tx.Rebind(query), args...)
	return ids, err
}

func mcpResolutionReadOptions(driver string) *sql.TxOptions {
	options := &sql.TxOptions{ReadOnly: true}
	if dialect.IsPostgres(driver) {
		options.Isolation = sql.LevelRepeatableRead
	}
	return options
}

func readMCPSessionResolutionInputs(ctx context.Context, tx *sqlx.Tx, rc mcpconfig.ResolutionContext, inputs *mcpconfig.ResolutionInputs) error {
	if rc.SessionID == "" {
		return nil
	}
	var err error
	inputs.ExcludedDefinitionIDs, err = readMCPSessionExclusionsTx(ctx, tx, rc.WorkspaceID, rc.SessionID)
	if err != nil {
		return err
	}
	err = tx.QueryRowxContext(ctx, tx.Rebind(`SELECT desired_revision FROM mcp_task_session_apply_state WHERE task_session_id = ?`), rc.SessionID).Scan(&inputs.DesiredRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
