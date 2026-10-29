package utility

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) handlePrepareAttempt(c *gin.Context) {
	var request PrepareAttemptRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validPreparationRequest(request) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "preparation and call identifiers must be UUIDs"})
		return
	}
	if h.attempts == nil {
		c.JSON(http.StatusServiceUnavailable, AttemptStatus{State: AttemptUnknown})
		return
	}
	if h.beginOperation != nil {
		operation, release, err := h.beginOperation(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, AttemptStatus{State: AttemptUnknown})
			return
		}
		defer release()
		if operation.Err() != nil {
			c.JSON(http.StatusRequestTimeout, AttemptStatus{State: AttemptUnknown})
			return
		}
	}
	status, err := h.attempts.prepare(request.PreparationID, request.CallID, request.Retain)
	code := http.StatusOK
	if err != nil {
		code = promptErrorStatus(err)
	}
	c.JSON(code, status)
}

func validPreparationRequest(request PrepareAttemptRequest) bool {
	if _, err := uuid.Parse(request.PreparationID); err != nil {
		return false
	}
	if request.CallID != "" {
		if _, err := uuid.Parse(request.CallID); err != nil {
			return false
		}
	}
	return true
}

func (h *Handler) handleCancelPreparedAttempt(c *gin.Context) {
	h.cancelPrepared(c, c.Param("id"), "")
}

func (h *Handler) handleReleaseRetainedAttempt(c *gin.Context) {
	if h.attempts == nil {
		c.JSON(http.StatusServiceUnavailable, AttemptStatus{State: AttemptUnknown})
		return
	}
	status, err := h.attempts.releaseRetained(c.Param("id"))
	code := http.StatusOK
	if err != nil {
		code = promptErrorStatus(err)
	}
	c.JSON(code, status)
}

func (h *Handler) handleCancelPreparation(c *gin.Context) {
	h.cancelPrepared(c, "", c.Param("id"))
}

func (h *Handler) cancelPrepared(c *gin.Context, id, preparationID string) {
	if h.attempts == nil {
		c.JSON(http.StatusServiceUnavailable, AttemptStatus{State: AttemptUnknown})
		return
	}
	status, err := h.attempts.cancelPrepared(id, preparationID)
	code := http.StatusOK
	if err != nil {
		code = promptErrorStatus(err)
	}
	c.JSON(code, status)
}
