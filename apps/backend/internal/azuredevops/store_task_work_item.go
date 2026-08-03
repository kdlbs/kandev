package azuredevops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const taskWorkItemColumns = `id, task_id, workspace_id, project_id, work_item_id, work_item_url, title, state, type, created_at, updated_at`

func (s *Store) UpsertTaskWorkItem(ctx context.Context, row *TaskWorkItem) error {
	if row == nil {
		return errors.New("azure devops store: task work item is required")
	}
	now := time.Now().UTC()
	row.UpdatedAt = now
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	query, args, err := sqlx.Named(`INSERT INTO azure_devops_task_work_items (`+taskWorkItemColumns+`)
		VALUES (:id,:task_id,:workspace_id,:project_id,:work_item_id,:work_item_url,:title,:state,:type,:created_at,:updated_at)
		ON CONFLICT(task_id, workspace_id, project_id, work_item_id) DO UPDATE SET
		work_item_url=excluded.work_item_url,title=excluded.title,state=excluded.state,type=excluded.type,updated_at=excluded.updated_at
		RETURNING id, created_at`, row)
	if err != nil {
		return err
	}
	return s.db.QueryRowxContext(ctx, s.db.Rebind(query), args...).Scan(&row.ID, &row.CreatedAt)
}

func (s *Store) ListTaskWorkItemsByWorkspace(ctx context.Context, workspaceID string) (map[string][]*TaskWorkItem, error) {
	var rows []TaskWorkItem
	if err := s.ro.SelectContext(ctx, &rows, `SELECT `+taskWorkItemColumns+` FROM azure_devops_task_work_items WHERE workspace_id = ? ORDER BY created_at ASC`, workspaceID); err != nil {
		return nil, err
	}
	result := make(map[string][]*TaskWorkItem)
	for i := range rows {
		result[rows[i].TaskID] = append(result[rows[i].TaskID], &rows[i])
	}
	return result, nil
}

func (s *Store) DeleteTaskWorkItemsByTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM azure_devops_task_work_items WHERE task_id = ?`), taskID)
	return err
}

func (s *Store) DeleteTaskWorkItemsByWorkspace(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM azure_devops_task_work_items WHERE workspace_id = ?`), workspaceID)
	return err
}

func (s *Store) TaskBelongsToWorkspace(ctx context.Context, taskID, workspaceID string) (bool, error) {
	var found string
	err := s.ro.GetContext(ctx, &found, `SELECT workspace_id FROM tasks WHERE id = ?`, taskID)
	if err != nil {
		return false, err
	}
	return found == workspaceID, nil
}
