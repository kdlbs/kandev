package azuredevops

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
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
	existing, err := s.findTaskPR(ctx, taskPR)
	if err != nil {
		return err
	}
	if existing != nil {
		taskPR.ID = existing.ID
		taskPR.CreatedAt = existing.CreatedAt
		return s.updateTaskPR(ctx, taskPR)
	}
	if taskPR.ID == "" {
		taskPR.ID = uuid.NewString()
	}
	if taskPR.CreatedAt.IsZero() {
		taskPR.CreatedAt = now
	}
	_, err = s.db.NamedExecContext(ctx, `
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
		)`, taskPR)
	return err
}

func (s *Store) findTaskPR(ctx context.Context, taskPR *TaskPR) (*TaskPR, error) {
	var existing TaskPR
	err := s.ro.GetContext(ctx, &existing,
		`SELECT `+taskPRSelectColumns+` FROM azure_devops_task_prs
		 WHERE task_id = ? AND repository_id = ? AND azure_repository_id = ?
		 AND pull_request_id = ? LIMIT 1`,
		taskPR.TaskID, taskPR.RepositoryID, taskPR.AzureRepositoryID, taskPR.PullRequestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *Store) updateTaskPR(ctx context.Context, taskPR *TaskPR) error {
	_, err := s.db.NamedExecContext(ctx, `
		UPDATE azure_devops_task_prs SET
			organization_url = :organization_url, project_id = :project_id,
			pull_request_url = :pull_request_url, title = :title,
			source_branch = :source_branch, target_branch = :target_branch,
			author_id = :author_id, author_name = :author_name, status = :status,
			review_state = :review_state, policy_state = :policy_state,
			is_draft = :is_draft, last_synced_at = :last_synced_at,
			updated_at = :updated_at
		WHERE id = :id`, taskPR)
	return err
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
