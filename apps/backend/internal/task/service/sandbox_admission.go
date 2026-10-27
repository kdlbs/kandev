package service

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

// SetSandboxCreationAdmission installs policy capture before a task becomes visible.
func (s *Service) SetSandboxCreationAdmission(admit func(context.Context, *models.Task, *CreateTaskRequest) error) {
	s.sandboxCreationAdmission = admit
}

// ResolveTaskCreationExecutor uses the same selection inputs as workspace admission.
func (s *Service) ResolveTaskCreationExecutor(ctx context.Context, req *CreateTaskRequest) (string, error) {
	profileID, executorID := requestExecutorSelectionIDs(req)
	if profileID != "" {
		return s.executorIDFromProfile(ctx, profileID)
	}
	if executorID != "" {
		return executorID, nil
	}
	return s.workspaceDefaultExecutorID(ctx, req.WorkspaceID)
}
