package azuredevops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func normalizeWatchIdentity(id string) string {
	if id == "" {
		return uuid.NewString()
	}
	return id
}

func normalizeWorkItemWatch(watch *WorkItemWatch) error {
	if watch == nil || watch.WorkspaceID == "" || watch.ProjectID == "" || watch.WIQL == "" {
		return fmt.Errorf("%w: workspace, project, and WIQL are required", ErrInvalidConfig)
	}
	if !IsValidCleanupPolicy(watch.CleanupPolicy) {
		return fmt.Errorf("%w: invalid cleanup policy", ErrInvalidConfig)
	}
	if err := validateMaxInflightTasks(watch.MaxInflightTasks); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	watch.ID = normalizeWatchIdentity(watch.ID)
	watch.PollIntervalSeconds = normalizePollInterval(watch.PollIntervalSeconds)
	watch.CleanupPolicy = normalizeCleanupPolicy(watch.CleanupPolicy)
	if watch.Generation <= 0 {
		watch.Generation = 1
	}
	return nil
}

func normalizePullRequestWatch(watch *PullRequestWatch) error {
	if watch == nil || watch.WorkspaceID == "" || watch.ProjectID == "" {
		return fmt.Errorf("%w: workspace and project are required", ErrInvalidConfig)
	}
	if !IsValidCleanupPolicy(watch.CleanupPolicy) {
		return fmt.Errorf("%w: invalid cleanup policy", ErrInvalidConfig)
	}
	if err := validateMaxInflightTasks(watch.MaxInflightTasks); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	watch.ID = normalizeWatchIdentity(watch.ID)
	watch.PollIntervalSeconds = normalizePollInterval(watch.PollIntervalSeconds)
	watch.CleanupPolicy = normalizeCleanupPolicy(watch.CleanupPolicy)
	if watch.Status == "" {
		watch.Status = activePullRequestState
	}
	if watch.Generation <= 0 {
		watch.Generation = 1
	}
	return nil
}

func (s *Store) CreateWorkItemWatch(ctx context.Context, watch *WorkItemWatch) error {
	if err := normalizeWorkItemWatch(watch); err != nil {
		return err
	}
	watch.Enabled = true
	now := time.Now().UTC()
	if watch.CreatedAt.IsZero() {
		watch.CreatedAt = now
	}
	watch.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO azure_devops_work_item_watches
		(id, workspace_id, workflow_id, workflow_step_id, project_id, wiql,
		repository_id, base_branch, agent_profile_id, executor_profile_id, prompt,
		enabled, poll_interval_seconds, cleanup_policy, max_inflight_tasks,
		generation, deleting, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		watch.ID, watch.WorkspaceID, watch.WorkflowID, watch.WorkflowStepID,
		watch.ProjectID, watch.WIQL, watch.RepositoryID, watch.BaseBranch,
		watch.AgentProfileID, watch.ExecutorProfileID, watch.Prompt, watch.Enabled,
		watch.PollIntervalSeconds, watch.CleanupPolicy, watch.MaxInflightTasks,
		watch.Generation, watch.Deleting, watch.LastError, watch.CreatedAt, watch.UpdatedAt)
	return err
}

func (s *Store) GetWorkItemWatch(ctx context.Context, id string) (*WorkItemWatch, error) {
	var watch WorkItemWatch
	err := s.ro.GetContext(ctx, &watch, `SELECT id, workspace_id, workflow_id,
		workflow_step_id, project_id, wiql, repository_id, base_branch,
		agent_profile_id, executor_profile_id, prompt, enabled,
		poll_interval_seconds, cleanup_policy, max_inflight_tasks, generation,
		deleting, last_error, last_error_at, last_polled_at, created_at, updated_at
		FROM azure_devops_work_item_watches WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &watch, nil
}

func (s *Store) ListWorkItemWatches(ctx context.Context, workspaceID string) ([]*WorkItemWatch, error) {
	var watches []*WorkItemWatch
	err := s.ro.SelectContext(ctx, &watches, `SELECT id, workspace_id, workflow_id,
		workflow_step_id, project_id, wiql, repository_id, base_branch,
		agent_profile_id, executor_profile_id, prompt, enabled,
		poll_interval_seconds, cleanup_policy, max_inflight_tasks, generation,
		deleting, last_error, last_error_at, last_polled_at, created_at, updated_at
		FROM azure_devops_work_item_watches WHERE workspace_id = ? AND deleting = 0
		ORDER BY created_at, id`, workspaceID)
	return watches, err
}

func (s *Store) ListEnabledWorkItemWatches(ctx context.Context) ([]*WorkItemWatch, error) {
	var watches []*WorkItemWatch
	err := s.ro.SelectContext(ctx, &watches, `SELECT id, workspace_id, workflow_id,
		workflow_step_id, project_id, wiql, repository_id, base_branch,
		agent_profile_id, executor_profile_id, prompt, enabled,
		poll_interval_seconds, cleanup_policy, max_inflight_tasks, generation,
		deleting, last_error, last_error_at, last_polled_at, created_at, updated_at
		FROM azure_devops_work_item_watches WHERE enabled = 1 AND deleting = 0
		ORDER BY last_polled_at IS NOT NULL, last_polled_at, id`)
	return watches, err
}

func (s *Store) UpdateWorkItemWatch(ctx context.Context, watch *WorkItemWatch) error {
	if err := normalizeWorkItemWatch(watch); err != nil {
		return err
	}
	watch.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE azure_devops_work_item_watches SET
		workflow_id = ?, workflow_step_id = ?, project_id = ?, wiql = ?,
		repository_id = ?, base_branch = ?, agent_profile_id = ?,
		executor_profile_id = ?, prompt = ?, enabled = ?, poll_interval_seconds = ?,
		cleanup_policy = ?, max_inflight_tasks = ?, updated_at = ?
		WHERE id = ? AND deleting = 0`,
		watch.WorkflowID, watch.WorkflowStepID, watch.ProjectID, watch.WIQL,
		watch.RepositoryID, watch.BaseBranch, watch.AgentProfileID,
		watch.ExecutorProfileID, watch.Prompt, watch.Enabled, watch.PollIntervalSeconds,
		watch.CleanupPolicy, watch.MaxInflightTasks, watch.UpdatedAt, watch.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWatchNotFound
	}
	return nil
}

func (s *Store) CreatePullRequestWatch(ctx context.Context, watch *PullRequestWatch) error {
	if err := normalizePullRequestWatch(watch); err != nil {
		return err
	}
	watch.Enabled = true
	now := time.Now().UTC()
	if watch.CreatedAt.IsZero() {
		watch.CreatedAt = now
	}
	watch.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO azure_devops_pull_request_watches
		(id, workspace_id, workflow_id, workflow_step_id, project_id,
		azure_repository_id, status, creator_id, reviewer_id, repository_id,
		base_branch, agent_profile_id, executor_profile_id, prompt, enabled,
		poll_interval_seconds, cleanup_policy, max_inflight_tasks, generation,
		deleting, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		watch.ID, watch.WorkspaceID, watch.WorkflowID, watch.WorkflowStepID,
		watch.ProjectID, watch.AzureRepositoryID, watch.Status, watch.CreatorID,
		watch.ReviewerID, watch.RepositoryID, watch.BaseBranch, watch.AgentProfileID,
		watch.ExecutorProfileID, watch.Prompt, watch.Enabled, watch.PollIntervalSeconds,
		watch.CleanupPolicy, watch.MaxInflightTasks, watch.Generation, watch.Deleting,
		watch.LastError, watch.CreatedAt, watch.UpdatedAt)
	return err
}

func (s *Store) GetPullRequestWatch(ctx context.Context, id string) (*PullRequestWatch, error) {
	var watch PullRequestWatch
	err := s.ro.GetContext(ctx, &watch, `SELECT id, workspace_id, workflow_id,
		workflow_step_id, project_id, azure_repository_id, status, creator_id,
		reviewer_id, repository_id, base_branch, agent_profile_id,
		executor_profile_id, prompt, enabled, poll_interval_seconds, cleanup_policy,
		max_inflight_tasks, generation, deleting, last_error, last_error_at,
		last_polled_at, created_at, updated_at
		FROM azure_devops_pull_request_watches WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &watch, nil
}

func (s *Store) ListPullRequestWatches(ctx context.Context, workspaceID string) ([]*PullRequestWatch, error) {
	var watches []*PullRequestWatch
	err := s.ro.SelectContext(ctx, &watches, `SELECT id, workspace_id, workflow_id,
		workflow_step_id, project_id, azure_repository_id, status, creator_id,
		reviewer_id, repository_id, base_branch, agent_profile_id,
		executor_profile_id, prompt, enabled, poll_interval_seconds, cleanup_policy,
		max_inflight_tasks, generation, deleting, last_error, last_error_at,
		last_polled_at, created_at, updated_at
		FROM azure_devops_pull_request_watches WHERE workspace_id = ? AND deleting = 0
		ORDER BY created_at, id`, workspaceID)
	return watches, err
}

func (s *Store) ListEnabledPullRequestWatches(ctx context.Context) ([]*PullRequestWatch, error) {
	var watches []*PullRequestWatch
	err := s.ro.SelectContext(ctx, &watches, `SELECT id, workspace_id, workflow_id,
		workflow_step_id, project_id, azure_repository_id, status, creator_id,
		reviewer_id, repository_id, base_branch, agent_profile_id,
		executor_profile_id, prompt, enabled, poll_interval_seconds, cleanup_policy,
		max_inflight_tasks, generation, deleting, last_error, last_error_at,
		last_polled_at, created_at, updated_at
		FROM azure_devops_pull_request_watches WHERE enabled = 1 AND deleting = 0
		ORDER BY last_polled_at IS NOT NULL, last_polled_at, id`)
	return watches, err
}

func (s *Store) UpdatePullRequestWatch(ctx context.Context, watch *PullRequestWatch) error {
	if err := normalizePullRequestWatch(watch); err != nil {
		return err
	}
	watch.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE azure_devops_pull_request_watches SET
		workflow_id = ?, workflow_step_id = ?, project_id = ?, azure_repository_id = ?,
		status = ?, creator_id = ?, reviewer_id = ?, repository_id = ?, base_branch = ?,
		agent_profile_id = ?, executor_profile_id = ?, prompt = ?, enabled = ?,
		poll_interval_seconds = ?, cleanup_policy = ?, max_inflight_tasks = ?,
		updated_at = ? WHERE id = ? AND deleting = 0`,
		watch.WorkflowID, watch.WorkflowStepID, watch.ProjectID, watch.AzureRepositoryID,
		watch.Status, watch.CreatorID, watch.ReviewerID, watch.RepositoryID,
		watch.BaseBranch, watch.AgentProfileID, watch.ExecutorProfileID, watch.Prompt,
		watch.Enabled, watch.PollIntervalSeconds, watch.CleanupPolicy,
		watch.MaxInflightTasks, watch.UpdatedAt, watch.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWatchNotFound
	}
	return nil
}

func (s *Store) SetWorkItemWatchError(ctx context.Context, id, message string, checkedAt time.Time) error {
	return s.setWatchError(ctx, "azure_devops_work_item_watches", id, message, checkedAt)
}

func (s *Store) SetPullRequestWatchError(ctx context.Context, id, message string, checkedAt time.Time) error {
	return s.setWatchError(ctx, "azure_devops_pull_request_watches", id, message, checkedAt)
}

func (s *Store) setWatchError(ctx context.Context, table, id, message string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET last_error = ?, last_error_at = ?, last_polled_at = ?, updated_at = ? WHERE id = ? AND deleting = 0`, table), message, checkedAt, checkedAt, checkedAt, id)
	return err
}

func (s *Store) ClearWorkItemWatchError(ctx context.Context, id string, polledAt time.Time) error {
	return s.clearWatchError(ctx, "azure_devops_work_item_watches", id, polledAt)
}

func (s *Store) ClearPullRequestWatchError(ctx context.Context, id string, polledAt time.Time) error {
	return s.clearWatchError(ctx, "azure_devops_pull_request_watches", id, polledAt)
}

func (s *Store) clearWatchError(ctx context.Context, table, id string, polledAt time.Time) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET last_error = '', last_error_at = NULL, last_polled_at = ?, updated_at = ? WHERE id = ? AND deleting = 0`, table), polledAt, polledAt, id)
	return err
}

func (s *Store) DisableWorkItemWatchWithError(ctx context.Context, id, cause string) error {
	return s.disableWatchWithError(ctx, "azure_devops_work_item_watches", id, cause)
}

func (s *Store) DisablePullRequestWatchWithError(ctx context.Context, id, cause string) error {
	return s.disableWatchWithError(ctx, "azure_devops_pull_request_watches", id, cause)
}

func (s *Store) disableWatchWithError(ctx context.Context, table, id, cause string) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET enabled = 0, last_error = ?, last_error_at = ?, updated_at = ? WHERE id = ? AND deleting = 0`, table), cause, time.Now().UTC(), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWatchNotFound
	}
	return nil
}

func (s *Store) DeleteWorkItemWatch(ctx context.Context, id string) error {
	return s.deleteWatch(ctx, "azure_devops_work_item_watches", "azure_devops_work_item_watch_tasks", id)
}

func (s *Store) DeletePullRequestWatch(ctx context.Context, id string) error {
	return s.deleteWatch(ctx, "azure_devops_pull_request_watches", "azure_devops_pull_request_watch_tasks", id)
}

func (s *Store) deleteWatch(ctx context.Context, watchTable, taskTable, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE watch_id = ?`, taskTable), id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, watchTable), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWatchNotFound
	}
	return tx.Commit()
}

func (s *Store) DeleteWorkItemWatchTasksByTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM azure_devops_work_item_watch_tasks WHERE task_id = ?`, taskID)
	return err
}

func (s *Store) DeletePullRequestWatchTasksByTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM azure_devops_pull_request_watch_tasks WHERE task_id = ?`, taskID)
	return err
}

func (s *Store) DeleteWatchesByWorkspace(ctx context.Context, workspaceID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// These tables intentionally do not rely on SQLite foreign-key pragma
	// state. Delete reservations first so a workspace removal cannot leave
	// orphaned provider matches that continue to block future imports.
	for _, query := range []string{
		`DELETE FROM azure_devops_work_item_watch_tasks WHERE watch_id IN (SELECT id FROM azure_devops_work_item_watches WHERE workspace_id = ?)`,
		`DELETE FROM azure_devops_pull_request_watch_tasks WHERE watch_id IN (SELECT id FROM azure_devops_pull_request_watches WHERE workspace_id = ?)`,
		`DELETE FROM azure_devops_work_item_watches WHERE workspace_id = ?`,
		`DELETE FROM azure_devops_pull_request_watches WHERE workspace_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListWorkItemWatchTasks(ctx context.Context, watchID string, generation int64) ([]*WorkItemWatchTask, error) {
	var rows []WorkItemWatchTask
	err := s.ro.SelectContext(ctx, &rows, `SELECT id, watch_id, project_id, work_item_id,
		work_item_url, task_id, generation, created_at
		FROM azure_devops_work_item_watch_tasks
		WHERE watch_id = ? AND generation = ? ORDER BY created_at`, watchID, generation)
	if err != nil {
		return nil, err
	}
	result := make([]*WorkItemWatchTask, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}
	return result, nil
}

func (s *Store) ListPullRequestWatchTasks(ctx context.Context, watchID string, generation int64) ([]*PullRequestWatchTask, error) {
	var rows []PullRequestWatchTask
	err := s.ro.SelectContext(ctx, &rows, `SELECT id, watch_id, project_id,
		azure_repository_id, pull_request_id, pull_request_url, task_id, generation,
		created_at FROM azure_devops_pull_request_watch_tasks
		WHERE watch_id = ? AND generation = ? ORDER BY created_at`, watchID, generation)
	if err != nil {
		return nil, err
	}
	result := make([]*PullRequestWatchTask, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}
	return result, nil
}

func (s *Store) DeleteWorkItemWatchTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM azure_devops_work_item_watch_tasks WHERE id = ?`, id)
	return err
}

func (s *Store) DeletePullRequestWatchTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM azure_devops_pull_request_watch_tasks WHERE id = ?`, id)
	return err
}

func (s *Store) ReserveWorkItemWatchTask(ctx context.Context, watchID string, generation int64, projectID string, workItemID int, workItemURL string) (bool, error) {
	return s.reserveWatchTask(ctx, `azure_devops_work_item_watch_tasks`, `
		INSERT INTO azure_devops_work_item_watch_tasks
		(id, watch_id, project_id, work_item_id, work_item_url, task_id, generation, created_at)
		SELECT ?, ?, ?, ?, ?, '', ?, ? WHERE EXISTS (
		SELECT 1 FROM azure_devops_work_item_watches WHERE id = ? AND generation = ? AND enabled = 1 AND deleting = 0
		) ON CONFLICT(watch_id, generation, project_id, work_item_id) DO NOTHING`,
		[]any{uuid.NewString(), watchID, projectID, workItemID, workItemURL, generation, time.Now().UTC(), watchID, generation})
}

func (s *Store) AssignWorkItemWatchTaskID(ctx context.Context, watchID string, generation int64, projectID string, workItemID int, taskID string) error {
	return s.assignWatchTaskID(ctx, `azure_devops_work_item_watch_tasks`, watchID, generation, taskID, "project_id = ? AND work_item_id = ?", projectID, workItemID)
}

func (s *Store) ReleaseWorkItemWatchTask(ctx context.Context, watchID string, generation int64, projectID string, workItemID int) error {
	return s.releaseWatchTask(ctx, `azure_devops_work_item_watch_tasks`, watchID, generation, "project_id = ? AND work_item_id = ?", projectID, workItemID)
}

func (s *Store) ReservePullRequestWatchTask(ctx context.Context, watchID string, generation int64, projectID, azureRepositoryID string, pullRequestID int, pullRequestURL string) (bool, error) {
	return s.reserveWatchTask(ctx, `azure_devops_pull_request_watch_tasks`, `
		INSERT INTO azure_devops_pull_request_watch_tasks
		(id, watch_id, project_id, azure_repository_id, pull_request_id, pull_request_url, task_id, generation, created_at)
		SELECT ?, ?, ?, ?, ?, ?, '', ?, ? WHERE EXISTS (
		SELECT 1 FROM azure_devops_pull_request_watches WHERE id = ? AND generation = ? AND enabled = 1 AND deleting = 0
		) ON CONFLICT(watch_id, generation, project_id, azure_repository_id, pull_request_id) DO NOTHING`,
		[]any{uuid.NewString(), watchID, projectID, azureRepositoryID, pullRequestID, pullRequestURL, generation, time.Now().UTC(), watchID, generation})
}

func (s *Store) AssignPullRequestWatchTaskID(ctx context.Context, watchID string, generation int64, projectID, azureRepositoryID string, pullRequestID int, taskID string) error {
	return s.assignWatchTaskID(ctx, `azure_devops_pull_request_watch_tasks`, watchID, generation, taskID, "project_id = ? AND azure_repository_id = ? AND pull_request_id = ?", projectID, azureRepositoryID, pullRequestID)
}

func (s *Store) ReleasePullRequestWatchTask(ctx context.Context, watchID string, generation int64, projectID, azureRepositoryID string, pullRequestID int) error {
	return s.releaseWatchTask(ctx, `azure_devops_pull_request_watch_tasks`, watchID, generation, "project_id = ? AND azure_repository_id = ? AND pull_request_id = ?", projectID, azureRepositoryID, pullRequestID)
}

func (s *Store) reserveWatchTask(ctx context.Context, table, query string, args []any) (bool, error) {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Store) assignWatchTaskID(ctx context.Context, table, watchID string, generation int64, taskID, identity string, identityArgs ...any) error {
	args := []any{taskID, watchID, generation}
	args = append(args, identityArgs...)
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET task_id = ? WHERE watch_id = ? AND generation = ? AND %s`, table, identity), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		watch, watchErr := s.watchGeneration(ctx, watchID)
		if watchErr != nil || watch == nil || watch.Generation != generation || watch.Deleting || !watch.Enabled {
			return ErrWatchOwnershipLost
		}
		return ErrReservationNotFound
	}
	return nil
}

func (s *Store) releaseWatchTask(ctx context.Context, table, watchID string, generation int64, identity string, identityArgs ...any) error {
	args := []any{watchID, generation}
	args = append(args, identityArgs...)
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE watch_id = ? AND generation = ? AND %s`, table, identity), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrReservationNotFound
	}
	return nil
}

type watchGenerationRow struct {
	Generation int64 `db:"generation"`
	Deleting   bool  `db:"deleting"`
	Enabled    bool  `db:"enabled"`
}

func (s *Store) watchGeneration(ctx context.Context, watchID string) (*watchGenerationRow, error) {
	var row watchGenerationRow
	queries := []string{
		`SELECT generation, deleting, enabled FROM azure_devops_work_item_watches WHERE id = ?`,
		`SELECT generation, deleting, enabled FROM azure_devops_pull_request_watches WHERE id = ?`,
	}
	for _, query := range queries {
		err := s.ro.GetContext(ctx, &row, query, watchID)
		if err == nil {
			return &row, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}
