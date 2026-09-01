package dashboard

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getTaskQuorum handles GET /workspaces/:wsId/tasks/:taskId/quorum (AC-24b), a sibling
// of listTaskDecisions carrying the same authorization as that route. Uses the
// :taskId param name (not :id) to match the sibling /workspaces/:wsId/tasks/:taskId/labels
// routes — gin's router requires the same wildcard name at a shared tree position.
// Task-existence is resolved here, before GetTaskQuorum is ever called
// (F37, mirroring getTask) — the dispatcher/engine layer is never asked to
// distinguish "task doesn't exist" from any other case.
func (h *Handler) getTaskQuorum(c *gin.Context) {
	ctx := c.Request.Context()
	workspaceID := c.Param("wsId")
	taskID := c.Param("taskId")
	task, err := h.svc.GetTask(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if task.WorkspaceID != workspaceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	resp, err := h.svc.GetTaskQuorum(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
