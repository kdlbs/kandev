package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/db/dialect"
)

const mcpForUpdateClause = ` FOR UPDATE`

var _ mcpconfig.ConditionalSelectionRepository = (*sqliteRepository)(nil)

func lockMCPSelectionOwner(ctx context.Context, tx *sqlx.Tx, origin mcpconfig.SelectionOrigin) error {
	if dialect.IsPostgres(tx.DriverName()) {
		key := "mcp-selection:" + origin.WorkspaceID + ":" + string(origin.Scope) + ":" + origin.OwnerID
		_, err := tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`), key)
		return err
	}
	// Acquire SQLite writer admission even when the owner has no selections yet.
	_, err := tx.ExecContext(ctx, `UPDATE mcp_task_session_apply_state SET desired_revision = desired_revision WHERE 1 = 0`)
	return err
}

func readMCPSelectionSnapshotTx(ctx context.Context, tx *sqlx.Tx, origin mcpconfig.SelectionOrigin) (mcpconfig.SelectionSnapshot, error) {
	ids, err := readMCPSelectionIDsTx(ctx, tx, origin)
	snapshot := mcpconfig.SelectionSnapshot{DefinitionIDs: ids}
	if err != nil || origin.Scope != mcpconfig.SelectionScopeTaskSession {
		return snapshot, err
	}
	state, err := readMCPSelectionState(ctx, tx, origin.OwnerID)
	if errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		return snapshot, nil
	}
	snapshot.State = &state
	return snapshot, err
}

func (r *sqliteRepository) ReadMCPSelectionSnapshot(ctx context.Context, origin mcpconfig.SelectionOrigin) (mcpconfig.SelectionSnapshot, error) {
	tx, err := r.ro.BeginTxx(ctx, mcpResolutionReadOptions(r.ro.DriverName()))
	if err != nil {
		return mcpconfig.SelectionSnapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := readMCPSelectionSnapshotTx(ctx, tx, origin)
	if err != nil {
		return snapshot, err
	}
	return snapshot, tx.Commit()
}

func normalizedMCPSelectionIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			result = append(result, id)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func checkMCPSelectionCondition(snapshot mcpconfig.SelectionSnapshot, condition mcpconfig.SelectionCondition) error {
	if condition.ExpectedDefinitionIDs != nil {
		savedIDs := slices.Clone(snapshot.DefinitionIDs)
		slices.Sort(savedIDs)
		if !slices.Equal(savedIDs, normalizedMCPSelectionIDs(*condition.ExpectedDefinitionIDs)) {
			return mcpconfig.ErrMCPSelectionConflict
		}
	}
	revision := int64(0)
	if snapshot.State != nil {
		revision = snapshot.State.DesiredRevision
	}
	if condition.ExpectedRevision != nil && revision != *condition.ExpectedRevision {
		return mcpconfig.ErrMCPSelectionConflict
	}
	return nil
}

func validateMCPSelectionDefinitions(ctx context.Context, tx *sqlx.Tx, workspaceID string, ids, previous []string) error {
	for _, id := range ids {
		query := `SELECT enabled FROM mcp_server_definitions WHERE workspace_id = ? AND id = ?`
		if dialect.IsPostgres(tx.DriverName()) {
			query += mcpForUpdateClause
		}
		var enabled bool
		err := tx.QueryRowxContext(ctx, tx.Rebind(query), workspaceID, id).Scan(&enabled)
		if errors.Is(err, sql.ErrNoRows) {
			return mcpconfig.ErrMCPSelectionWorkspaceMismatch
		}
		if err != nil {
			return err
		}
		if !enabled && !slices.Contains(previous, id) {
			return mcpconfig.ErrMCPDefinitionDisabled
		}
	}
	return nil
}

func (r *sqliteRepository) ReplaceMCPSelectionsConditional(ctx context.Context, origin mcpconfig.SelectionOrigin, ids []string, condition mcpconfig.SelectionCondition) (mcpconfig.SelectionSnapshot, error) {
	var result mcpconfig.SelectionSnapshot
	if err := condition.Validate(origin.Scope); err != nil {
		return result, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockMCPSelectionOwner(ctx, tx, origin); err != nil {
		return result, err
	}
	previous, err := readMCPSelectionSnapshotTx(ctx, tx, origin)
	if err != nil {
		return result, err
	}
	if err := checkMCPSelectionCondition(previous, condition); err != nil {
		return result, err
	}
	ids = normalizedMCPSelectionIDs(ids)
	if slices.ContainsFunc(ids, func(id string) bool { return !slices.Contains(previous.DefinitionIDs, id) }) {
		if err := validateMCPSelectionAdditionOwner(ctx, tx, origin, condition.ValidateAdditions); err != nil {
			return result, err
		}
	}
	if err := validateMCPSelectionDefinitions(ctx, tx, origin.WorkspaceID, ids, previous.DefinitionIDs); err != nil {
		return result, err
	}
	if err := replaceMCPSelectionsTx(ctx, tx, origin.Scope, origin.WorkspaceID, origin.OwnerID, ids); err != nil {
		return result, err
	}
	if origin.Scope == mcpconfig.SelectionScopeTaskSession {
		if err := advanceMCPSelectionRevision(ctx, tx, origin.OwnerID); err != nil {
			return result, err
		}
	}
	result, err = readMCPSelectionSnapshotTx(ctx, tx, origin)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func validateMCPSelectionAdditionOwner(ctx context.Context, tx *sqlx.Tx, origin mcpconfig.SelectionOrigin, validate func(context.Context) error) error {
	if validate != nil {
		if err := validate(ctx); err != nil {
			return err
		}
	}
	if origin.Scope != mcpconfig.SelectionScopeProfile {
		return nil
	}
	query := `SELECT enabled FROM agent_profiles WHERE id = ? AND deleted_at IS NULL AND (workspace_id IS NULL OR workspace_id = '' OR workspace_id = ?)`
	if dialect.IsPostgres(tx.DriverName()) {
		query += mcpForUpdateClause
	}
	var enabled bool
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), origin.OwnerID, origin.WorkspaceID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !enabled) {
		return mcpconfig.ErrMCPSelectionOwnerAccess
	}
	return err
}

func advanceMCPSelectionRevision(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
 INSERT INTO mcp_task_session_apply_state (task_session_id, desired_revision, applied_revision, apply_state, updated_at)
 VALUES (?, 1, 0, 'pending_idle', ?)
 ON CONFLICT(task_session_id) DO UPDATE SET
 desired_revision = mcp_task_session_apply_state.desired_revision + 1,
 apply_state = 'pending_idle', failure_code = '', failure_summary = '', updated_at = excluded.updated_at
 `), sessionID, time.Now().UTC())
	return err
}

// Catalog deletion locks the definition before cleaning references. Selection
// writers take their owner lock, then definition locks in sorted ID order.
func lockMCPDefinition(ctx context.Context, tx *sqlx.Tx, workspaceID, id string) error {
	query := `SELECT id FROM mcp_server_definitions WHERE workspace_id = ? AND id = ?`
	if dialect.IsPostgres(tx.DriverName()) {
		query += mcpForUpdateClause
	}
	var found string
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), workspaceID, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpconfig.ErrMCPServerDefinitionNotFound
	}
	return err
}
