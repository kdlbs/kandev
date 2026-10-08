package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// GetWorkspaceForCloneTx holds source deletion/placement changes until the
// caller's configuration transaction has settled.
func (r *Repository) GetWorkspaceForCloneTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.Workspace, error) {
	query := `SELECT ` + workspaceSelectColumns + ` FROM workspaces WHERE id = ?`
	if dialect.IsPostgres(tx.DriverName()) {
		query += ` FOR SHARE`
	}
	workspace, err := scanWorkspaceRow(tx.QueryRowContext(ctx, tx.Rebind(query), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrWorkspaceNotFound
	}
	return workspace, err
}

// CreateWorkspaceCloneTx creates only the workspace and its new owner. All
// configuration participants use the same supplied transaction.
func (r *Repository) CreateWorkspaceCloneTx(ctx context.Context, tx *sqlx.Tx, workspace *models.Workspace) error {
	r.prepareWorkspace(workspace)
	if err := r.insertWorkspace(ctx, tx, workspace); err != nil {
		return err
	}
	if workspace.OwnerID == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workspace_members (`+workspaceMemberColumns+`) VALUES (?, ?, 'owner', '', ?)`), workspace.ID, workspace.OwnerID, workspace.CreatedAt)
	return err
}

// CopyWorkspaceConfigurationTx copies live source definitions, returning the
// workflow ID map for the workflow owner's step-copy participant.
func (r *Repository) CopyWorkspaceConfigurationTx(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) (map[string]string, error) {
	repositoryIDs, err := r.copyCloneRepositories(ctx, tx, sourceID, targetID)
	if err != nil {
		return nil, err
	}
	if err := r.copyCloneRepositorySets(ctx, tx, sourceID, targetID, repositoryIDs); err != nil {
		return nil, err
	}
	return r.copyCloneWorkflows(ctx, tx, sourceID, targetID)
}

func (r *Repository) copyCloneRepositories(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) (map[string]string, error) {
	var ids []string
	if err := tx.SelectContext(ctx, &ids, tx.Rebind(`SELECT id FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY id`), sourceID); err != nil {
		return nil, err
	}
	mapped := make(map[string]string, len(ids))
	now := time.Now().UTC()
	for _, id := range ids {
		mapped[id] = uuid.NewString()
		_, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO repositories (id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner, provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at)
			SELECT ?, ?, name, source_type, CASE WHEN source_type = 'local' THEN local_path ELSE '' END, provider, provider_repo_id, provider_host, provider_scope, provider_owner, provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, ?, ?
			FROM repositories WHERE id = ? AND workspace_id = ? AND deleted_at IS NULL`), mapped[id], targetID, now, now, id, sourceID)
		if err != nil {
			return nil, err
		}
		if err := r.copyCloneRepositoryChildren(ctx, tx, id, mapped[id], now); err != nil {
			return nil, err
		}
	}
	return mapped, nil
}

func (r *Repository) copyCloneRepositoryChildren(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string, now time.Time) error {
	var policyIDs, scriptIDs []string
	if err := tx.SelectContext(ctx, &policyIDs, tx.Rebind(`SELECT id FROM repository_branch_policies WHERE repository_id = ? ORDER BY id`), sourceID); err != nil {
		return err
	}
	for _, id := range policyIDs {
		_, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
			SELECT ?, ?, name, description, base_branch, branch_template, pull_request_target, ?, ? FROM repository_branch_policies WHERE id = ?`), uuid.NewString(), targetID, now, now, id)
		if err != nil {
			return err
		}
	}
	if err := tx.SelectContext(ctx, &scriptIDs, tx.Rebind(`SELECT id FROM repository_scripts WHERE repository_id = ? ORDER BY position, id`), sourceID); err != nil {
		return err
	}
	for _, id := range scriptIDs {
		_, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO repository_scripts (id, repository_id, name, command, position, created_at, updated_at)
			SELECT ?, ?, name, command, position, ?, ? FROM repository_scripts WHERE id = ?`), uuid.NewString(), targetID, now, now, id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) copyCloneRepositorySets(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string, repositoryIDs map[string]string) error {
	var ids []string
	if err := tx.SelectContext(ctx, &ids, tx.Rebind(`SELECT id FROM repository_sets WHERE workspace_id = ? ORDER BY id`), sourceID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range ids {
		newID := uuid.NewString()
		_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO repository_sets (`+repositorySetColumns+`)
			SELECT ?, ?, name, description, ?, ? FROM repository_sets WHERE id = ?`), newID, targetID, now, now, id)
		if err != nil {
			return err
		}
		if err := r.copyCloneSetItems(ctx, tx, id, newID, repositoryIDs, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) copyCloneSetItems(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string, repositoryIDs map[string]string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT repository_id, base_branch FROM repository_set_items WHERE repository_set_id = ? ORDER BY position, id`), sourceID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var items []models.RepositorySetItem
	for rows.Next() {
		var item models.RepositorySetItem
		if err := rows.Scan(&item.RepositoryID, &item.BaseBranch); err != nil {
			return err
		}
		if mapped, ok := repositoryIDs[item.RepositoryID]; ok {
			item.RepositoryID = mapped
			items = append(items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return r.insertRepositorySetItems(ctx, tx, targetID, items, now)
}

func (r *Repository) copyCloneWorkflows(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT `+workflowSelectColumns+` FROM workflows WHERE workspace_id = ? AND hidden = 0 ORDER BY sort_order, id`), sourceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var workflows []*models.Workflow
	for rows.Next() {
		workflow, err := scanWorkflowRow(rows)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	mapped := make(map[string]string, len(workflows))
	for _, workflow := range workflows {
		sourceWorkflowID := workflow.ID
		workflow.ID, workflow.WorkspaceID = uuid.NewString(), targetID
		workflow.Source, workflow.SourcePath = models.WorkflowSourceManual, ""
		r.prepareWorkflow(workflow)
		if err := r.insertWorkflow(ctx, tx, workflow); err != nil {
			return nil, err
		}
		mapped[sourceWorkflowID] = workflow.ID
	}
	return mapped, nil
}

// BootstrapWorkspaceCloneTx supplies the ordinary Kanban workflow only when
// the source has no eligible user-managed workflow.
func (r *Repository) BootstrapWorkspaceCloneTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) (*models.Workflow, error) {
	template, err := kanbanTemplate()
	if err != nil {
		return nil, err
	}
	workflow := &models.Workflow{WorkspaceID: workspaceID, Name: "Kanban", WorkflowTemplateID: &template.ID}
	r.prepareWorkflow(workflow)
	if err := r.insertWorkflow(ctx, tx, workflow); err != nil {
		return nil, err
	}
	if err := r.insertTemplateSteps(ctx, tx, workflow.ID, template); err != nil {
		return nil, err
	}
	return workflow, nil
}

// ListWorkspaceCloneWorkflowsTx reads definitions before the coordinator commits.
func (r *Repository) ListWorkspaceCloneWorkflowsTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) ([]*models.Workflow, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT `+workflowSelectColumns+` FROM workflows WHERE workspace_id = ? ORDER BY sort_order, id`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var workflows []*models.Workflow
	for rows.Next() {
		workflow, err := scanWorkflowRow(rows)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	return workflows, rows.Err()
}
