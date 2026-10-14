package backendapp

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
)

// The host composition owns agent HTTP identity; the task module owns adoption.
func configureWorkspaceAdoptionCaller(h *taskhandlers.TaskHandlers) {
	h.SetWorkspaceAdoptionCallerGuard(func(c *gin.Context) bool {
		id, ok := authn.FromGin(c)
		if !ok || id.Synthetic {
			return false
		}
		return officeagents.CallerFromContext(c) == nil && officeagents.ClaimsFromContext(c) == nil
	})
}
