package service

import "context"

// workspaceOwnerUserIDForTask hydrates the durable workspace owner instead of
// copying the caller identity into workspace-oriented metadata. Callers apply
// their own actor precedence before this value.
func (s *Service) workspaceOwnerUserIDForTask(ctx context.Context, taskID string) string {
	if taskID == "" || s.tasks == nil || s.workspaces == nil {
		return ""
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return ""
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil {
		return ""
	}
	return workspace.OwnerID
}
