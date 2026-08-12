package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// dependencyProjection adapts the service's derived DependencyView to the DTO
// package's projection type. The two are kept separate so dto stays importable
// from service without a cycle.
func dependencyProjection(view service.DependencyView) dto.TaskDependencyProjection {
	return dto.TaskDependencyProjection{
		Blocked:       view.Blocked,
		BlockedReason: view.BlockedReason,
		DependsOn:     dependencyRefs(view.DependsOn),
		Blocks:        dependencyRefs(view.Blocks),
	}
}

func dependencyRefs(refs []service.DependencyRef) []dto.TaskDependencyRefDTO {
	if len(refs) == 0 {
		return nil
	}
	out := make([]dto.TaskDependencyRefDTO, 0, len(refs))
	for _, r := range refs {
		out = append(out, dto.TaskDependencyRefDTO{
			ID: r.ID, Title: r.Title, State: r.State, Status: r.Status,
		})
	}
	return out
}

// Response keys repeated across the dependency handlers.
const (
	dependencyKeyError  = "error"
	dependencyKeyTaskID = "task_id"
)

// addTaskDependencyBody is the POST /tasks/:id/dependencies payload.
type addTaskDependencyBody struct {
	DependsOnTaskID string `json:"depends_on_task_id" binding:"required"`
}

// httpAddTaskDependency handles POST /api/v1/tasks/:id/dependencies —
// "this task is blocked by depends_on_task_id".
//
// A cycle returns 409 with a `cycle` array so the frontend can render
// "A → B → C → A"; that body shape is what BlockersPicker already parses.
func (h *TaskHandlers) httpAddTaskDependency(c *gin.Context) {
	taskID := c.Param("id")
	var body addTaskDependencyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{dependencyKeyError: "depends_on_task_id is required"})
		return
	}
	err := h.service.AddDependency(c.Request.Context(), taskID, body.DependsOnTaskID)
	if err == nil {
		h.respondWithDependencies(c, taskID)
		return
	}
	var cycle *service.CycleError
	if errors.As(err, &cycle) {
		c.JSON(http.StatusConflict, gin.H{dependencyKeyError: cycle.Error(), "cycle": cycle.Path})
		return
	}
	if errors.Is(err, service.ErrDependencyRepositoryUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{dependencyKeyError: err.Error()})
		return
	}
	handleNotFound(c, h.logger, err, "task not found")
}

// httpRemoveTaskDependency handles DELETE /api/v1/tasks/:id/dependencies/:depId.
// Removing an absent edge is a success no-op.
func (h *TaskHandlers) httpRemoveTaskDependency(c *gin.Context) {
	taskID := c.Param("id")
	if err := h.service.RemoveDependency(c.Request.Context(), taskID, c.Param("depId")); err != nil {
		if errors.Is(err, service.ErrDependencyRepositoryUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{dependencyKeyError: err.Error()})
			return
		}
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	h.respondWithDependencies(c, taskID)
}

// respondWithDependencies returns the task's dependency projection after a
// mutation so the caller does not have to re-fetch the task to learn the result.
func (h *TaskHandlers) respondWithDependencies(c *gin.Context, taskID string) {
	task, err := h.service.GetTask(c.Request.Context(), taskID)
	if err != nil || task == nil {
		c.JSON(http.StatusOK, gin.H{dependencyKeyTaskID: taskID})
		return
	}
	views := h.service.BuildDependencyViews(c.Request.Context(), []*models.Task{task})
	v := views[taskID]
	c.JSON(http.StatusOK, gin.H{
		dependencyKeyTaskID: taskID,
		"blocked":           v.Blocked,
		"blocked_reason":    v.BlockedReason,
		"depends_on":        v.DependsOn,
		"blocks":            v.Blocks,
	})
}
