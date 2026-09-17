package runtime

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/shared"
	"net/http"
)

const (
	errorResponseKey = "error"
)

type workspaceManager interface {
	WorkspaceCatalog(context.Context, string) (any, error)
	ManageWorkspaceTask(context.Context, shared.WorkspaceTaskCommand) error
}

func (h *Handler) workspaceCatalog(c *gin.Context) {
	run, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	manager, ok := h.actions.deps.Tasks.(workspaceManager)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "workspace manager unavailable"})
		return
	}
	result, err := manager.WorkspaceCatalog(c.Request.Context(), run.WorkspaceID)
	if err != nil {
		h.respondRuntimeError(c, run, "workspace_catalog", "workspace", run.WorkspaceID, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) manageWorkspaceTask(c *gin.Context) {
	run, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	taskID := c.Param("id")
	if !run.Capabilities.Allows(CapabilitySpawnAgentRun) || !run.CanMutateTask(taskID) {
		h.respondRuntimeError(c, run, "manage_task", "task", taskID, ErrCapabilityDenied)
		return
	}
	var command shared.WorkspaceTaskCommand
	if !bindJSON(c, &command) {
		return
	}
	command.WorkspaceID, command.ChiefID, command.TaskID = run.WorkspaceID, run.AgentID, taskID
	manager, ok := h.actions.deps.Tasks.(workspaceManager)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "workspace manager unavailable"})
		return
	}
	if err := manager.ManageWorkspaceTask(c.Request.Context(), command); err != nil {
		h.respondRuntimeError(c, run, "manage_task", "task", taskID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), run, command.Action, "task", taskID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) workspaceTaskDetails(c *gin.Context) {
	run, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	taskID := c.Param("id")
	if !run.CanMutateTask(taskID) {
		h.respondRuntimeError(c, run, "read_workspace_task", "task", taskID, ErrTaskOutOfScope)
		return
	}
	reader, ok := h.actions.deps.Tasks.(interface {
		WorkspaceTaskDetails(context.Context, string, string) (any, error)
	})
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "workspace task reader unavailable"})
		return
	}
	result, err := reader.WorkspaceTaskDetails(c.Request.Context(), run.WorkspaceID, taskID)
	if err != nil {
		h.respondRuntimeError(c, run, "read_workspace_task", "task", taskID, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
