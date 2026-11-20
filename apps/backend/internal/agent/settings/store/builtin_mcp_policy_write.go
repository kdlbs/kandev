package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	internaldb "github.com/kandev/kandev/internal/db"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

var _ mcpconfig.BuiltinPolicyRepository = (*sqliteRepository)(nil)

func (r *sqliteRepository) beginBuiltinPolicyWrite(ctx context.Context, workspaceID, taskID string) (*sqlx.Tx, error) {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	err = internaldb.LockTaskHierarchy(ctx, tx, r.db.DriverName(), r.db.Rebind, []string{workspaceID}, []string{taskID})
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (r *sqliteRepository) PutBuiltinWorkspacePolicy(ctx context.Context, candidate *mcpconfig.BuiltinWorkspacePolicy, expectedRevision int64) (*mcpconfig.BuiltinWorkspacePolicy, error) {
	if candidate == nil || candidate.Validate() != nil || candidate.UpdatedBy == "" || expectedRevision < 0 {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	tx, err := r.beginBuiltinPolicyWrite(ctx, candidate.WorkspaceID, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := readBuiltinPolicySnapshot(ctx, tx, candidate.WorkspaceID, "")
	if err != nil {
		return nil, err
	}
	if current.Policy.Revision != expectedRevision {
		return nil, &mcpconfig.BuiltinPolicyConflictError{Current: current}
	}
	if maps.Equal(current.Policy.Packages, candidate.Packages) {
		return current.Policy, tx.Commit()
	}
	next := *candidate
	next.Packages = maps.Clone(candidate.Packages)
	next.Revision = current.Policy.Revision + 1
	packages, err := json.Marshal(next.Packages)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workspace_builtin_mcp_policies
		(workspace_id, schema_version, revision, packages_json, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id) DO UPDATE SET schema_version = excluded.schema_version, revision = excluded.revision,
		packages_json = excluded.packages_json, updated_by = excluded.updated_by, updated_at = excluded.updated_at`),
		next.WorkspaceID, next.Version, next.Revision, string(packages), next.UpdatedBy, next.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &next, tx.Commit()
}

func (r *sqliteRepository) PutBuiltinTaskPreferences(ctx context.Context, candidate *mcpconfig.BuiltinTaskPreferences, expectedRevision, expectedPolicyRevision int64, surface p.Surface) (*mcpconfig.BuiltinPolicySnapshot, error) {
	if candidate == nil || candidate.Validate(surface) != nil || candidate.UpdatedBy == "" || expectedRevision < 0 || expectedPolicyRevision < 0 {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	tx, err := r.beginBuiltinPolicyWrite(ctx, candidate.WorkspaceID, candidate.TaskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := readBuiltinPolicySnapshot(ctx, tx, candidate.WorkspaceID, candidate.TaskID)
	if err != nil {
		return nil, err
	}
	if current.Preferences.Revision != expectedRevision || current.Policy.Revision != expectedPolicyRevision {
		return nil, &mcpconfig.BuiltinPolicyConflictError{Current: current}
	}
	next, err := prepareBuiltinTaskWrite(candidate, current.Policy)
	if err != nil {
		return nil, err
	}
	if maps.Equal(current.Preferences.Packages, next.Packages) {
		return current, tx.Commit()
	}
	next.Revision = current.Preferences.Revision + 1
	if err := writeBuiltinTaskPreferences(ctx, tx, next); err != nil {
		return nil, err
	}
	current.Preferences = next
	return current, tx.Commit()
}

func writeBuiltinTaskPreferences(ctx context.Context, tx *sqlx.Tx, next *mcpconfig.BuiltinTaskPreferences) error {
	packages, err := json.Marshal(next.Packages)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_builtin_mcp_preferences
		(task_id, workspace_id, schema_version, revision, packages_json, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (task_id) DO UPDATE SET workspace_id = excluded.workspace_id, schema_version = excluded.schema_version,
		revision = excluded.revision, packages_json = excluded.packages_json, updated_by = excluded.updated_by, updated_at = excluded.updated_at`),
		next.TaskID, next.WorkspaceID, next.Version, next.Revision, string(packages), next.UpdatedBy, next.UpdatedAt)
	return err
}

func prepareBuiltinTaskWrite(candidate *mcpconfig.BuiltinTaskPreferences, policy *mcpconfig.BuiltinWorkspacePolicy) (*mcpconfig.BuiltinTaskPreferences, error) {
	next := *candidate
	next.Packages = maps.Clone(candidate.Packages)
	for id, value := range next.Packages {
		if value == mcpconfig.BuiltinPreferenceOn && policy.Packages[id] == mcpconfig.BuiltinPolicyBlocked {
			return nil, mcpconfig.ErrBuiltinPackageBlocked
		}
		if value == mcpconfig.BuiltinPreferenceInherit {
			delete(next.Packages, id)
		}
	}
	return &next, nil
}
