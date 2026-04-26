package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

func (h *Handlers) listRoutines(c *gin.Context) {
	routines, err := h.ctrl.Svc.ListRoutines(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RoutineListResponse{Routines: routines})
}

func (h *Handlers) createRoutine(c *gin.Context) {
	var req dto.CreateRoutineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	routine := &models.Routine{
		WorkspaceID:             c.Param("wsId"),
		Name:                    req.Name,
		Description:             req.Description,
		TaskTemplate:            req.TaskTemplate,
		AssigneeAgentInstanceID: req.AssigneeAgentInstanceID,
		Status:                  "active",
		ConcurrencyPolicy:       req.ConcurrencyPolicy,
		Variables:               req.Variables,
	}
	if err := h.ctrl.Svc.CreateRoutine(c.Request.Context(), routine); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.RoutineResponse{Routine: routine})
}

func (h *Handlers) getRoutine(c *gin.Context) {
	routine, err := h.ctrl.Svc.GetRoutine(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RoutineResponse{Routine: routine})
}

func (h *Handlers) updateRoutine(c *gin.Context) {
	ctx := c.Request.Context()
	routineID := c.Param("id")
	routine, err := h.ctrl.Svc.GetRoutine(ctx, routineID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var req dto.UpdateRoutineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	applyRoutineUpdates(routine, &req)
	if err := h.ctrl.Svc.UpdateRoutine(ctx, routine); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RoutineResponse{Routine: routine})
}

func (h *Handlers) deleteRoutine(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteRoutine(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) runRoutine(c *gin.Context) {
	var req dto.RunRoutineRequest
	// Body is optional for manual trigger.
	_ = c.ShouldBindJSON(&req)
	run, err := h.ctrl.Svc.FireManual(c.Request.Context(), c.Param("id"), req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RoutineRunResponse{Run: run})
}

func (h *Handlers) listTriggers(c *gin.Context) {
	triggers, err := h.ctrl.Svc.ListRoutineTriggers(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.TriggerListResponse{Triggers: triggers})
}

func (h *Handlers) createTrigger(c *gin.Context) {
	var req dto.CreateTriggerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	trigger := &models.RoutineTrigger{
		RoutineID:      c.Param("id"),
		Kind:           req.Kind,
		CronExpression: req.CronExpression,
		Timezone:       req.Timezone,
		PublicID:       req.PublicID,
		SigningMode:    req.SigningMode,
		Secret:         req.Secret,
		Enabled:        true,
	}
	if err := h.ctrl.Svc.CreateRoutineTrigger(c.Request.Context(), trigger); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.TriggerResponse{Trigger: trigger})
}

func (h *Handlers) deleteTrigger(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteRoutineTrigger(c.Request.Context(), c.Param("triggerId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) listRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	runs, err := h.ctrl.Svc.ListRoutineRuns(c.Request.Context(), c.Param("id"), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RunListResponse{Runs: runs})
}

func (h *Handlers) listAllRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	runs, err := h.ctrl.Svc.ListAllRoutineRuns(c.Request.Context(), c.Param("wsId"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RunListResponse{Runs: runs})
}

func applyRoutineUpdates(routine *models.Routine, req *dto.UpdateRoutineRequest) {
	if req.Name != nil {
		routine.Name = *req.Name
	}
	if req.Description != nil {
		routine.Description = *req.Description
	}
	if req.TaskTemplate != nil {
		routine.TaskTemplate = *req.TaskTemplate
	}
	if req.AssigneeAgentInstanceID != nil {
		routine.AssigneeAgentInstanceID = *req.AssigneeAgentInstanceID
	}
	if req.Status != nil {
		routine.Status = *req.Status
	}
	if req.ConcurrencyPolicy != nil {
		routine.ConcurrencyPolicy = *req.ConcurrencyPolicy
	}
	if req.Variables != nil {
		routine.Variables = *req.Variables
	}
}
