package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/db/dialect"
)

var _ mcpconfig.SelectionRepository = (*sqliteRepository)(nil)

type mcpSelectionTable struct {
	name        string
	ownerColumn string
}

func selectionTable(scope mcpconfig.SelectionScope) (mcpSelectionTable, error) {
	switch scope {
	case mcpconfig.SelectionScopeProfile:
		return mcpSelectionTable{name: "workspace_agent_profile_mcp_selections", ownerColumn: "profile_id"}, nil
	case mcpconfig.SelectionScopeRepository:
		return mcpSelectionTable{name: "repository_mcp_selections", ownerColumn: "repository_id"}, nil
	case mcpconfig.SelectionScopeTask:
		return mcpSelectionTable{name: "task_mcp_selections", ownerColumn: "task_id"}, nil
	case mcpconfig.SelectionScopeTaskSession:
		return mcpSelectionTable{name: "task_session_mcp_selections", ownerColumn: "task_session_id"}, nil
	default:
		return mcpSelectionTable{}, fmt.Errorf("%w: unsupported scope", mcpconfig.ErrMCPInvalidSelection)
	}
}

func (r *sqliteRepository) ListMCPSelections(
	ctx context.Context,
	scope mcpconfig.SelectionScope,
	workspaceID, ownerID string,
) ([]string, error) {
	table, err := selectionTable(scope)
	if err != nil {
		return nil, err
	}
	query := `SELECT mcp_server_id FROM ` + table.name + ` WHERE ` + table.ownerColumn + ` = ?`
	args := []any{ownerID}
	if scope == mcpconfig.SelectionScopeProfile {
		query += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	query += ` ORDER BY mcp_server_id`
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]string, 0)
	for rows.Next() {
		var definitionID string
		if err := rows.Scan(&definitionID); err != nil {
			return nil, err
		}
		result = append(result, definitionID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) ReplaceMCPSelections(
	ctx context.Context,
	scope mcpconfig.SelectionScope,
	workspaceID, ownerID string,
	definitionIDs []string,
) error {
	table, err := selectionTable(scope)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	deleteQuery := `DELETE FROM ` + table.name + ` WHERE ` + table.ownerColumn + ` = ?`
	deleteArgs := []any{ownerID}
	if scope == mcpconfig.SelectionScopeProfile {
		deleteQuery += ` AND workspace_id = ?`
		deleteArgs = append(deleteArgs, workspaceID)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(deleteQuery), deleteArgs...); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(definitionIDs))
	for _, definitionID := range definitionIDs {
		definitionID = strings.TrimSpace(definitionID)
		if definitionID == "" {
			continue
		}
		if _, exists := seen[definitionID]; exists {
			continue
		}
		seen[definitionID] = struct{}{}
		query := `INSERT INTO ` + table.name + ` (` + table.ownerColumn + `, mcp_server_id`
		args := []any{ownerID, definitionID}
		if scope == mcpconfig.SelectionScopeProfile {
			query = `INSERT INTO ` + table.name + ` (workspace_id, ` + table.ownerColumn + `, mcp_server_id`
			args = []any{workspaceID, ownerID, definitionID}
		}
		query += `) VALUES (` + placeholders(len(args)) + `) ON CONFLICT DO NOTHING`
		if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) SelectionImpact(ctx context.Context, workspaceID, definitionID string) (mcpconfig.SelectionImpact, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(definitionID) == "" {
		return mcpconfig.SelectionImpact{}, mcpconfig.ErrMCPInvalidSelection
	}
	var impact mcpconfig.SelectionImpact
	counts := []struct {
		table string
		out   *int
	}{
		{table: "workspace_agent_profile_mcp_selections", out: &impact.Profile},
		{table: "repository_mcp_selections", out: &impact.Repository},
		{table: "task_mcp_selections", out: &impact.Task},
		{table: "task_session_mcp_selections", out: &impact.TaskSession},
	}
	for _, item := range counts {
		query := `SELECT COUNT(*) FROM ` + item.table + ` s JOIN mcp_server_definitions d ON d.id = s.mcp_server_id WHERE d.workspace_id = ? AND d.id = ?`
		if item.table == "workspace_agent_profile_mcp_selections" {
			query += ` AND s.workspace_id = ?`
			if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), workspaceID, definitionID, workspaceID).Scan(item.out); err != nil {
				return mcpconfig.SelectionImpact{}, err
			}
			continue
		}
		if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), workspaceID, definitionID).Scan(item.out); err != nil {
			return mcpconfig.SelectionImpact{}, err
		}
	}
	return impact, nil
}

func (r *sqliteRepository) DeleteMCPSelectionsForDefinition(ctx context.Context, workspaceID, definitionID string) error {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(definitionID) == "" {
		return mcpconfig.ErrMCPInvalidSelection
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteMCPSelectionRows(ctx, tx, definitionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *sqliteRepository) GetMCPImportState(ctx context.Context, workspaceID, profileID string) (mcpconfig.LegacyImportState, error) {
	var state mcpconfig.LegacyImportState
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT workspace_id, profile_id, status, failure_code, failure_reason, updated_at
		FROM mcp_legacy_import_state WHERE workspace_id = ? AND profile_id = ?
	`), workspaceID, profileID).Scan(
		&state.WorkspaceID, &state.ProfileID, &state.Status,
		&state.FailureCode, &state.FailureReason, &state.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpconfig.LegacyImportState{}, mcpconfig.ErrMCPLegacyImportStateNotFound
	}
	return state, err
}

func (r *sqliteRepository) SaveMCPImportState(ctx context.Context, state mcpconfig.LegacyImportState) error {
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO mcp_legacy_import_state
		(workspace_id, profile_id, status, failure_code, failure_reason, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, profile_id) DO UPDATE SET
			status = excluded.status,
			failure_code = excluded.failure_code,
			failure_reason = excluded.failure_reason,
			updated_at = excluded.updated_at
	`), state.WorkspaceID, state.ProfileID, state.Status,
		state.FailureCode, state.FailureReason, state.UpdatedAt)
	return err
}

func deleteMCPSelectionRows(ctx context.Context, tx *sqlx.Tx, definitionID string) error {
	for _, table := range []string{
		"workspace_agent_profile_mcp_selections",
		"repository_mcp_selections",
		"task_mcp_selections",
		"task_session_mcp_selections",
	} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE mcp_server_id = ?`, definitionID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMCPServerDefinitionWithSelections is the atomic path used by the
// catalog service when a confirmed deletion has affected selections.
func (r *sqliteRepository) DeleteMCPServerDefinitionWithSelections(ctx context.Context, workspaceID, id string, expectedRevision int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteMCPSelectionRows(ctx, tx, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM mcp_server_definitions WHERE workspace_id = ? AND id = ? AND revision = ?`), workspaceID, id, expectedRevision)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		_ = tx.Rollback()
		return r.mcpRevisionOrNotFound(ctx, workspaceID, id)
	}
	return tx.Commit()
}

// ImportLegacyMCPProfileWorkspace commits imported definitions, the
// workspace-contextual profile selection, and the complete marker together.
// Callers use this only after service-level validation and secret redaction.
func (r *sqliteRepository) ImportLegacyMCPProfileWorkspace(
	ctx context.Context,
	workspaceID, profileID string,
	definitions []*mcpconfig.MCPServerDefinition,
	definitionIDs []string,
	state mcpconfig.LegacyImportState,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, definition := range definitions {
		if err := prepareMCPServerDefinition(definition); err != nil {
			return err
		}
		configuration, bindings, err := marshalMCPDefinition(definition)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO mcp_server_definitions
			(id, workspace_id, runtime_name, normalized_runtime_name, display_name, description,
			 enabled, execution_mode, transport, configuration_json, secret_bindings_json,
			 source, source_identity, revision, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING
		`), definition.ID, definition.WorkspaceID, definition.RuntimeName,
			definition.NormalizedRuntimeName, definition.DisplayName, definition.Description,
			dialect.BoolToInt(definition.Enabled), definition.ExecutionMode, definition.Transport,
			configuration, bindings, definition.Source, definition.SourceIdentity,
			definition.Revision, definition.CreatedAt, definition.UpdatedAt)
		if err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM workspace_agent_profile_mcp_selections
		WHERE workspace_id = ? AND profile_id = ?
	`), workspaceID, profileID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(definitionIDs))
	for _, definitionID := range definitionIDs {
		if _, ok := seen[definitionID]; ok || strings.TrimSpace(definitionID) == "" {
			continue
		}
		seen[definitionID] = struct{}{}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO workspace_agent_profile_mcp_selections
			(workspace_id, profile_id, mcp_server_id) VALUES (?, ?, ?)
			ON CONFLICT DO NOTHING
		`), workspaceID, profileID, definitionID); err != nil {
			return err
		}
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO mcp_legacy_import_state
		(workspace_id, profile_id, status, failure_code, failure_reason, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, profile_id) DO UPDATE SET
			status = excluded.status,
			failure_code = excluded.failure_code,
			failure_reason = excluded.failure_reason,
			updated_at = excluded.updated_at
	`), state.WorkspaceID, state.ProfileID, state.Status,
		state.FailureCode, state.FailureReason, state.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
