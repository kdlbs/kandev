package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (r *sqliteRepository) GetBuiltinPolicySnapshot(ctx context.Context, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	tx, err := r.ro.BeginTxx(ctx, mcpResolutionReadOptions(r.ro.DriverName()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := readBuiltinPolicySnapshot(ctx, tx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	return snapshot, tx.Commit()
}

func readBuiltinPolicySnapshot(ctx context.Context, tx *sqlx.Tx, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	if err := checkBuiltinPolicyOwner(ctx, tx, workspaceID, taskID); err != nil {
		return nil, err
	}
	policy, err := readBuiltinWorkspacePolicy(ctx, tx, workspaceID)
	if err != nil {
		return nil, err
	}
	snapshot := &mcpconfig.BuiltinPolicySnapshot{Policy: policy}
	if taskID != "" {
		snapshot.Preferences, err = readBuiltinTaskPreferences(ctx, tx, workspaceID, taskID)
	}
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func checkBuiltinPolicyOwner(ctx context.Context, tx *sqlx.Tx, workspaceID, taskID string) error {
	var actual string
	query, id := `SELECT id FROM workspaces WHERE id = ?`, workspaceID
	if taskID != "" {
		query, id = `SELECT workspace_id FROM tasks WHERE id = ?`, taskID
	}
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), id).Scan(&actual)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && actual != workspaceID) {
		return mcpconfig.ErrMCPWorkspaceAccess
	}
	return err
}

func readBuiltinWorkspacePolicy(ctx context.Context, tx *sqlx.Tx, workspaceID string) (*mcpconfig.BuiltinWorkspacePolicy, error) {
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy(workspaceID)
	var packages string
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT schema_version, revision, packages_json, updated_by, updated_at FROM workspace_builtin_mcp_policies WHERE workspace_id = ?`), workspaceID).
		Scan(&policy.Version, &policy.Revision, &packages, &policy.UpdatedBy, &policy.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return policy, nil
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal([]byte(packages), &policy.Packages) != nil || policy.Validate() != nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	return policy, nil
}

func readBuiltinTaskPreferences(ctx context.Context, tx *sqlx.Tx, workspaceID, taskID string) (*mcpconfig.BuiltinTaskPreferences, error) {
	preferences := &mcpconfig.BuiltinTaskPreferences{WorkspaceID: workspaceID, TaskID: taskID, Version: 1, Packages: make(mcpconfig.BuiltinPackageChoices)}
	var packages string
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT workspace_id, schema_version, revision, packages_json, updated_by, updated_at FROM task_builtin_mcp_preferences WHERE task_id = ?`), taskID).
		Scan(&preferences.WorkspaceID, &preferences.Version, &preferences.Revision, &packages, &preferences.UpdatedBy, &preferences.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return preferences, nil
	}
	if err != nil {
		return nil, err
	}
	if preferences.WorkspaceID != workspaceID || json.Unmarshal([]byte(packages), &preferences.Packages) != nil || preferences.Validate(p.SurfaceKanbanTask) != nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	return preferences, nil
}
