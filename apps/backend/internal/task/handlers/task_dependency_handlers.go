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
	dependencyKeyError        = "error"
	dependencyKeyTaskID       = "task_id"
	maxDependencyRequestBytes = 128 << 10
)

// addTaskDependencyBody is the POST /tasks/:id/dependencies payload.
type addTaskDependencyBody struct {
	DependsOnTaskID string `json:"depends_on_task_id" binding:"required"`
}

// replaceTaskDependenciesBody is the PUT /tasks/:id/dependencies payload.
// A pointer distinguishes an explicit empty list, which clears all edges,
// from a request that omitted the complete-set field.
type replaceTaskDependenciesBody struct {
	DependsOnTaskIDs *[]string `json:"depends_on_task_ids"`
}

// httpAddTaskDependency handles POST /api/v1/tasks/:id/dependencies —
// "this task is blocked by depends_on_task_id".
//
// A cycle returns 409 with a `cycle` array so the frontend can render
// "A → B → C → A"; that body shape is what BlockersPicker already parses.
func (h *TaskHandlers) httpAddTaskDependency(c *gin.Context) {
	taskID := c.Param("id")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDependencyRequestBytes)
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

// httpReplaceTaskDependencies handles PUT /api/v1/tasks/:id/dependencies.
// The request is a complete predecessor set, including an empty list to clear
// all direct dependencies.
func (h *TaskHandlers) httpReplaceTaskDependencies(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDependencyRequestBytes)
	var body replaceTaskDependenciesBody
	if err := c.ShouldBindJSON(&body); err != nil || body.DependsOnTaskIDs == nil {
		c.JSON(http.StatusBadRequest, gin.H{dependencyKeyError: "depends_on_task_ids is required"})
		return
	}
	err := h.service.ReplaceDependencies(c.Request.Context(), c.Param("id"), *body.DependsOnTaskIDs)
	if err == nil {
		h.respondWithDependencies(c, c.Param("id"))
		return
	}
	var cycle *service.CycleError
	if errors.As(err, &cycle) {
		c.JSON(http.StatusConflict, gin.H{dependencyKeyError: cycle.Error(), "cycle": cycle.Path})
		return
	}
	if errors.Is(err, service.ErrInvalidDependencySet) {
		c.JSON(http.StatusBadRequest, taskErrorBody(err))
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
	// Serialize through the same DTO adapter the task-list path uses, so a
	// mutation response and a list response are one JSON contract.
	projection := dependencyProjection(views[taskID])
	c.JSON(http.StatusOK, gin.H{
		dependencyKeyTaskID: taskID,
		"blocked":           projection.Blocked,
		"blocked_reason":    projection.BlockedReason,
		"depends_on":        projection.DependsOn,
		"blocks":            projection.Blocks,
	})
}
