package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrator"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

// httpMoveTaskPreview returns an advisory move prediction. The task and
// workflow reads here provide the same access checks as the move endpoint;
// the orchestrator then performs the read-only routing calculation.
func (h *TaskHandlers) httpMoveTaskPreview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.movePreviewer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workflow move preview is unavailable"})
		return
	}

	var body httpMoveTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.WorkflowID == "" || body.WorkflowStepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow_id and workflow_step_id are required"})
		return
	}

	taskID := c.Param("id")
	task, err := h.service.GetTask(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	workflow, err := h.service.GetWorkflow(c.Request.Context(), body.WorkflowID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	if workflow == nil || workflow.WorkspaceID != task.WorkspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target workflow is in a different workspace"})
		return
	}

	entryOptions, err := workflowmove.NormalizeEntryOptions(body.EntryOptions, "")
	if err != nil {
		handleSelectedMoveError(c, h.logger, err)
		return
	}
	change := workflowmove.MoveChangeStep
	if task.WorkflowID == body.WorkflowID && task.WorkflowStepID == body.WorkflowStepID {
		change = workflowmove.MoveChangePositionOnly
	}
	if err := workflowmove.ValidateEntryOptions(entryOptions, change); err != nil {
		handleSelectedMoveError(c, h.logger, err)
		return
	}

	preview, err := h.movePreviewer.PreviewWorkflowMove(c.Request.Context(), orchestrator.WorkflowMovePreviewRequest{
		TaskID:         taskID,
		WorkflowID:     body.WorkflowID,
		WorkflowStepID: body.WorkflowStepID,
		EntryOptions:   entryOptions,
	})
	if err != nil {
		handleSelectedMoveError(c, h.logger, err)
		return
	}
	c.JSON(http.StatusOK, preview)
}
