package github

import (
	"context"
	"errors"
	"strings"
)

// authorizeTaskPRMutation checks the workspace that owns a task-PR
// association before a mutation can proceed. Current rows carry that
// workspace directly; legacy rows derive it from the owning task instead.
func (s *Service) authorizeTaskPRMutation(ctx context.Context, tp *TaskPR, workspaceID string) (string, error) {
	if tp == nil {
		return "", ErrTaskPRNotFound
	}
	if strings.TrimSpace(tp.WorkspaceID) != "" {
		if tp.WorkspaceID != workspaceID {
			return "", ErrTaskPRNotFound
		}
		if err := s.authorizeWorkspaceAccess(ctx, tp.WorkspaceID); err != nil {
			return "", err
		}
		return tp.WorkspaceID, nil
	}
	if strings.TrimSpace(tp.TaskID) == "" {
		return "", ErrTaskPRNotFound
	}

	taskStore := s.getTaskIssueStore()
	if taskStore == nil {
		return "", ErrTaskPRNotFound
	}
	task, err := taskStore.GetTask(ctx, tp.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return "", ErrTaskPRNotFound
		}
		return "", err
	}
	if task == nil || strings.TrimSpace(task.WorkspaceID) == "" || task.WorkspaceID != workspaceID {
		return "", ErrTaskPRNotFound
	}
	if err := s.authorizeWorkspaceAccess(ctx, task.WorkspaceID); err != nil {
		return "", err
	}
	return task.WorkspaceID, nil
}
