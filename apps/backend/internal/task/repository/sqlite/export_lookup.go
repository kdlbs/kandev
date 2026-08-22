package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// GetWorkflowTx, GetRepositoryTx, and GetExecutorProfileTx are the
// transaction-accepting counterparts to GetWorkflow/GetRepository/
// GetExecutorProfile that the automations YAML export (AC-29) needs: the
// export service opens one read transaction on the shared reader pool and
// passes it through every store's lookup, so all reads observe the same
// snapshot. Unlike the plain methods, a missing row is not an error - the
// export's own AC-19 partial-resolution rule decides what a missing
// reference means, so these report a three-outcome (value, found, err)
// result instead.

// GetWorkflowTx retrieves a workflow by ID within tx.
func (r *Repository) GetWorkflowTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.Workflow, bool, error) {
	workflow, err := scanWorkflowRow(tx.QueryRowContext(ctx, tx.Rebind(fmt.Sprintf(`
		SELECT %s
		FROM workflows WHERE id = ?
	`, workflowSelectColumns)), id))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return workflow, true, nil
}

// GetRepositoryTx retrieves a repository by ID within tx. It does not attach
// secret bindings - the export never renders them (AC-19's secrets carve-out).
func (r *Repository) GetRepositoryTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.Repository, bool, error) {
	repository := &models.Repository{}

	err := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner,
		       provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at, deleted_at
		FROM repositories WHERE id = ? AND deleted_at IS NULL
	`), id).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderHost, &repository.ProviderScope, &repository.ProviderOwner, &repository.ProviderName, &repository.RemoteURL,
		&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.WorktreeBranchTemplate, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CopyFiles, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return repository, true, nil
}

// GetExecutorProfileTx retrieves an executor profile by ID within tx.
func (r *Repository) GetExecutorProfileTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.ExecutorProfile, bool, error) {
	profile := &models.ExecutorProfile{}
	var configJSON string
	var envVarsJSON sql.NullString

	err := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT id, executor_id, name, mcp_policy, config, prepare_script, cleanup_script, env_vars, created_at, updated_at
		FROM executor_profiles WHERE id = ?
	`), id).Scan(
		&profile.ID, &profile.ExecutorID, &profile.Name, &profile.McpPolicy,
		&configJSON, &profile.PrepareScript, &profile.CleanupScript, &envVarsJSON, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &profile.Config); err != nil {
			return nil, false, fmt.Errorf("failed to deserialize profile config: %w", err)
		}
	}
	if envVarsJSON.Valid && envVarsJSON.String != "" && envVarsJSON.String != jsonNull {
		if err := json.Unmarshal([]byte(envVarsJSON.String), &profile.EnvVars); err != nil {
			return nil, false, fmt.Errorf("failed to deserialize profile env_vars: %w", err)
		}
	}
	return profile, true, nil
}
