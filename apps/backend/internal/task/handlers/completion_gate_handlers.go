package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

func (h *TaskHandlers) httpGetTaskCompletionGate(c *gin.Context) {
	snapshot, err := h.service.GetTaskCompletionGate(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleCompletionGateError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *TaskHandlers) httpListTaskCompletionGateHistory(c *gin.Context) {
	history, err := h.service.ListTaskCompletionGateHistory(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleCompletionGateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": history})
}

func (h *TaskHandlers) httpSetTaskCompletionCriteria(c *gin.Context) {
	var request service.SetTaskCompletionCriteriaRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_completion_criteria"})
		return
	}
	snapshot, err := h.service.SetTaskCompletionCriteria(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		h.handleCompletionGateError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *TaskHandlers) httpVerifyTaskCompletionCriterion(c *gin.Context) {
	var request service.VerifyTaskCompletionCriterionRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_completion_evidence"})
		return
	}
	request.CriterionID = c.Param("criterion_id")
	snapshot, err := h.service.VerifyTaskCompletionCriterion(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		h.handleCompletionGateError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *TaskHandlers) handleCompletionGateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrTaskCompletionGateBlocked):
		snapshot, readErr := h.service.GetTaskCompletionGate(c.Request.Context(), c.Param("id"))
		body := gin.H{"code": "task_completion_gate_blocked"}
		if readErr == nil {
			body["gate"] = snapshot
		}
		c.JSON(http.StatusConflict, body)
	case errors.Is(err, repository.ErrTaskCompletionCriteriaConflict):
		c.JSON(http.StatusConflict, gin.H{"code": "task_completion_criteria_conflict"})
	case errors.Is(err, repository.ErrTaskCompletionEvidenceChanged):
		c.JSON(http.StatusConflict, gin.H{"code": "task_completion_evidence_changed"})
	case errors.Is(err, repository.ErrTaskCompletionHumanConfirmationRequired):
		c.JSON(http.StatusConflict, gin.H{"code": "task_completion_human_confirmation_required"})
	case errors.Is(err, repository.ErrTaskVersionConflict):
		c.JSON(http.StatusConflict, gin.H{"code": "task_version_conflict"})
	case errors.Is(err, repository.ErrTaskNotFound), service.IsForbidden(err):
		handleNotFound(c, h.logger, err, "task not found")
	case isValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_completion_gate"})
	default:
		h.logger.Error("task completion gate request failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task completion gate request failed"})
	}
}
