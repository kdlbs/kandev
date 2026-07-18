package azuredevops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const taskPRSelectColumns = `id, task_id, repository_id, organization_url,
	project_id, azure_repository_id, pull_request_id, pull_request_url, title,
	source_branch, target_branch, author_id, author_name, status, review_state,
	policy_state, is_draft, last_synced_at, created_at, updated_at`

const qualifiedTaskPRSelectColumns = `atp.id, atp.task_id, atp.repository_id,
	atp.organization_url, atp.project_id, atp.azure_repository_id,
	atp.pull_request_id, atp.pull_request_url, atp.title, atp.source_branch,
	atp.target_branch, atp.author_id, atp.author_name, atp.status,
	atp.review_state, atp.policy_state, atp.is_draft, atp.last_synced_at,
	atp.created_at, atp.updated_at`

// UpsertTaskPR persists one task-to-pull-request association while retaining
// its stable ID and creation timestamp across refreshes.
func (s *Store) UpsertTaskPR(ctx context.Context, taskPR *TaskPR) error {
	if taskPR == nil {
		return errors.New("azure devops store: task PR is required")
	}
	now := time.Now().UTC()
	taskPR.UpdatedAt = now
	if taskPR.ID == "" {
		taskPR.ID = uuid.NewString()
	}
	if taskPR.CreatedAt.IsZero() {
		taskPR.CreatedAt = now
	}
	query, args, err := sqlx.Named(`
		INSERT INTO azure_devops_task_prs (
			id, task_id, repository_id, organization_url, project_id,
			azure_repository_id, pull_request_id, pull_request_url, title,
			source_branch, target_branch, author_id, author_name, status,
			review_state, policy_state, is_draft, last_synced_at, created_at, updated_at
		) VALUES (
			:id, :task_id, :repository_id, :organization_url, :project_id,
			:azure_repository_id, :pull_request_id, :pull_request_url, :title,
			:source_branch, :target_branch, :author_id, :author_name, :status,
			:review_state, :policy_state, :is_draft, :last_synced_at, :created_at, :updated_at
		)
		ON CONFLICT(task_id, repository_id, azure_repository_id, pull_request_id)
		DO UPDATE SET
			organization_url = excluded.organization_url,
			project_id = excluded.project_id,
			pull_request_url = excluded.pull_request_url,
			title = excluded.title,
			source_branch = excluded.source_branch,
			target_branch = excluded.target_branch,
			author_id = excluded.author_id,
			author_name = excluded.author_name,
			status = excluded.status,
			review_state = excluded.review_state,
			policy_state = excluded.policy_state,
			is_draft = excluded.is_draft,
			last_synced_at = excluded.last_synced_at,
			updated_at = excluded.updated_at
		RETURNING id, created_at`, taskPR)
	if err != nil {
		return err
	}
	query = s.db.Rebind(query)
	return s.db.QueryRowxContext(ctx, query, args...).Scan(&taskPR.ID, &taskPR.CreatedAt)
}

// ListTaskPRsByTask returns all associations for one task in creation order.
func (s *Store) ListTaskPRsByTask(ctx context.Context, taskID string) ([]*TaskPR, error) {
	var rows []TaskPR
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+taskPRSelectColumns+` FROM azure_devops_task_prs
		 WHERE task_id = ? ORDER BY created_at ASC`, taskID); err != nil {
		return nil, err
	}
	return taskPRPointers(rows), nil
}

// DeleteTaskPRsByTask removes every Azure pull request association owned by a task.
func (s *Store) DeleteTaskPRsByTask(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("azure devops store: task id is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM azure_devops_task_prs WHERE task_id = ?`, taskID)
	return err
}

// ListTaskPRsByWorkspace groups associations for tasks owned by one workspace.
func (s *Store) ListTaskPRsByWorkspace(ctx context.Context, workspaceID string) (map[string][]*TaskPR, error) {
	var rows []TaskPR
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+qualifiedTaskPRSelectColumns+` FROM azure_devops_task_prs atp
		 INNER JOIN tasks t ON atp.task_id = t.id
		 WHERE t.workspace_id = ? ORDER BY atp.created_at ASC`, workspaceID); err != nil {
		return nil, err
	}
	grouped := make(map[string][]*TaskPR)
	for i := range rows {
		grouped[rows[i].TaskID] = append(grouped[rows[i].TaskID], &rows[i])
	}
	return grouped, nil
}

func taskPRPointers(rows []TaskPR) []*TaskPR {
	result := make([]*TaskPR, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}
	return result
}
