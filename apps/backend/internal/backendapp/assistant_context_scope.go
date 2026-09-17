package backendapp

import (
	"context"
	"fmt"
	shared "github.com/kandev/kandev/internal/orchestration/models"
)

// Project scope uses the workspace repository ID, not an Office project.
func (a *taskCreatorAdapter) ValidateAssistantContextScope(ctx context.Context, workspace string, scope shared.ContextScope) error {
	if scope.ProjectID != "" {
		repository, err := a.taskSvc.GetRepository(ctx, scope.ProjectID)
		if err != nil || repository.WorkspaceID != workspace {
			return fmt.Errorf("context repository unavailable")
		}
		if err := a.validateContextRepositoryTask(ctx, scope); err != nil {
			return err
		}
	}
	if scope.EnvironmentID != "" {
		if scope.TaskID == "" {
			return fmt.Errorf("environment context requires a task")
		}
		env, err := a.taskSvc.GetTaskEnvironmentByTaskID(ctx, scope.TaskID)
		if err != nil || env == nil || env.ID != scope.EnvironmentID {
			return fmt.Errorf("context environment unavailable")
		}
	}
	return nil
}

func (a *taskCreatorAdapter) validateContextRepositoryTask(ctx context.Context, scope shared.ContextScope) error {
	if scope.TaskID == "" {
		return nil
	}
	if a.taskRepo == nil {
		return fmt.Errorf("task repository scope unavailable")
	}
	rows, err := a.taskRepo.ListTaskRepositories(ctx, scope.TaskID)
	if err != nil {
		return err
	}
	found := false
	for _, row := range rows {
		if row.RepositoryID == scope.ProjectID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("repository is not attached to the context task")
	}
	return nil
}

// Environment memories are validated through the native environment's task.
func (a *taskCreatorAdapter) ValidateAssistantMemoryEnvironment(ctx context.Context, workspace, id string) error {
	if a.taskRepo == nil {
		return fmt.Errorf("environment scope unavailable")
	}
	env, err := a.taskRepo.GetTaskEnvironment(ctx, id)
	if err != nil || env == nil {
		return fmt.Errorf("environment scope unavailable")
	}
	task, err := a.taskSvc.GetTask(ctx, env.TaskID)
	if err != nil || task == nil || task.WorkspaceID != workspace {
		return fmt.Errorf("environment scope unavailable")
	}
	return nil
}
