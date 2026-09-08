package handlers

import "github.com/kandev/kandev/internal/task/models"

// canDirectParentAccess is deliberately narrower than workspace membership:
// only a direct parent may mutate its child through task-bound MCP.
func canDirectParentAccess(caller, target *models.Task) bool {
	return caller != nil && target != nil && caller.WorkspaceID != "" &&
		target.WorkspaceID == caller.WorkspaceID && target.ParentID == caller.ID
}
