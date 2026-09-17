package runtime

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type ContextScopeValidator interface {
	ValidateAssistantContextScope(context.Context, string, models.ContextScope) error
}

func (s *Service) validateContextScope(ctx context.Context, workspace string, scope models.ContextScope) error {
	if scope.TaskID != "" {
		task, err := s.Tasks.GetTask(ctx, scope.TaskID)
		if err != nil || task == nil || task.WorkspaceID != workspace {
			return fmt.Errorf("context task unavailable")
		}
	}
	if scope.ProjectID == "" && scope.EnvironmentID == "" {
		return nil
	}
	validator, ok := s.Manager.(ContextScopeValidator)
	if !ok {
		return fmt.Errorf("project/environment scope validation unavailable")
	}
	return validator.ValidateAssistantContextScope(ctx, workspace, scope)
}
