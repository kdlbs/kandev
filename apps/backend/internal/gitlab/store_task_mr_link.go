package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ValidateTaskMRScope verifies that the task and optional repository belong to
// the requested workspace. Unknown and cross-workspace resources deliberately
// share one error so callers do not disclose resource existence.
func (s *Store) ValidateTaskMRScope(ctx context.Context, workspaceID, taskID, repositoryID string) error {
	var taskExists int
	err := s.ro.GetContext(ctx, &taskExists,
		`SELECT 1 FROM tasks WHERE id = ? AND workspace_id = ? LIMIT 1`, taskID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("validate task workspace: %w", err)
	}
	if repositoryID == "" {
		return nil
	}
	var repositoryExists int
	err = s.ro.GetContext(ctx, &repositoryExists, `
		SELECT 1
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = ? AND tr.repository_id = ? AND r.workspace_id = ?
		LIMIT 1`, taskID, repositoryID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("validate task repository: %w", err)
	}
	return nil
}

// ValidateTaskMRRepositoryIdentity verifies the durable provider identity for
// the repository selected by the task. Legacy rows without a provider host
// fail closed because owner/name alone cannot distinguish GitLab instances.
func (s *Store) ValidateTaskMRRepositoryIdentity(
	ctx context.Context,
	workspaceID, taskID, repositoryID, configuredHost, projectPath string,
) error {
	if repositoryID == "" {
		return ErrTaskMRRepositoryMismatch
	}
	var identity struct {
		Provider string `db:"provider"`
		Host     string `db:"provider_host"`
		Owner    string `db:"provider_owner"`
		Name     string `db:"provider_name"`
	}
	err := s.ro.GetContext(ctx, &identity, `
		SELECT r.provider, r.provider_host, r.provider_owner, r.provider_name
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		JOIN tasks t ON t.id = tr.task_id
		WHERE tr.task_id = ? AND tr.repository_id = ?
			AND t.workspace_id = ? AND r.workspace_id = ?
		LIMIT 1`, taskID, repositoryID, workspaceID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("load task repository identity: %w", err)
	}
	wantHost, err := normalizeHostOrigin(configuredHost)
	if err != nil {
		return ErrTaskMRRepositoryMismatch
	}
	gotHost, err := normalizeHostOrigin(identity.Host)
	if err != nil {
		return ErrTaskMRRepositoryMismatch
	}
	gotProject := strings.Trim(strings.TrimSpace(identity.Owner+"/"+identity.Name), "/")
	wantProject := strings.Trim(strings.TrimSpace(projectPath), "/")
	if !strings.EqualFold(identity.Provider, "gitlab") ||
		!strings.EqualFold(gotHost, wantHost) ||
		!strings.EqualFold(gotProject, wantProject) {
		return ErrTaskMRRepositoryMismatch
	}
	return nil
}

// ResolveTaskMRRepository validates an explicit repository or infers the sole
// task repository. Scratch tasks retain an empty repository association;
// multi-repository tasks must make the choice explicit.
func (s *Store) ResolveTaskMRRepository(
	ctx context.Context,
	workspaceID, taskID, repositoryID string,
) (string, error) {
	if repositoryID != "" {
		if err := s.ValidateTaskMRScope(ctx, workspaceID, taskID, repositoryID); err != nil {
			return "", err
		}
		return repositoryID, nil
	}
	if err := s.ValidateTaskMRScope(ctx, workspaceID, taskID, ""); err != nil {
		return "", err
	}
	var repositoryIDs []string
	if err := s.ro.SelectContext(ctx, &repositoryIDs, `
		SELECT tr.repository_id
		FROM task_repositories tr
		JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = ? AND r.workspace_id = ?
		ORDER BY tr.id`, taskID, workspaceID); err != nil {
		return "", fmt.Errorf("list task repositories: %w", err)
	}
	switch len(repositoryIDs) {
	case 0:
		return "", nil
	case 1:
		return repositoryIDs[0], nil
	default:
		return "", ErrTaskMRRepositoryRequired
	}
}

// DeleteTaskMRForWorkspace atomically removes one association and only the
// refresh watch that identifies the same task, repository, project and MR.
func (s *Store) DeleteTaskMRForWorkspace(ctx context.Context, workspaceID, associationID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var association TaskMR
	err = tx.GetContext(ctx, &association, `
		SELECT `+taskMRSelectColsQualified+`
		FROM gitlab_task_mrs gtm
		JOIN tasks t ON t.id = gtm.task_id
		WHERE gtm.id = ? AND t.workspace_id = ?
		LIMIT 1`, associationID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskMRNotFound
	}
	if err != nil {
		return fmt.Errorf("find task MR association: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM gitlab_mr_watches
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		association.TaskID, association.RepositoryID, association.ProjectPath, association.MRIID,
	); err != nil {
		return fmt.Errorf("delete task MR refresh watch: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM gitlab_task_mrs WHERE id = ?`, association.ID); err != nil {
		return fmt.Errorf("delete task MR association: %w", err)
	}
	return tx.Commit()
}
