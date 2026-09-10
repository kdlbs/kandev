package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/service"
)

type httpTaskDeletePreflightRequest struct {
	TaskIDs []string `json:"task_ids"`
	Cascade bool     `json:"cascade"`
}

// httpTaskDeletePreflight returns a no-store, read-only cleanup consent
// decision for the requested delete scope. The delete route still performs
// its own authoritative inspection immediately before mutation.
func (h *TaskHandlers) httpTaskDeletePreflight(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var body httpTaskDeletePreflightRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task delete preflight request"})
		return
	}
	result, err := h.service.TaskDeletePreflight(c.Request.Context(), body.TaskIDs, body.Cascade)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTaskDeletePreflightInvalid):
			c.JSON(http.StatusBadRequest, taskErrorBody(err))
		case errors.Is(err, service.ErrTaskDeletePreflightUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task delete preflight unavailable"})
		default:
			handleNotFound(c, h.logger, err, "task not found")
		}
		return
	}
	c.JSON(http.StatusOK, result)
}
