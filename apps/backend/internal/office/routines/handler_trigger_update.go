package routines

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) updateTrigger(c *gin.Context) {
	var request UpdateTriggerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{jsonErrorKey: err.Error()})
		return
	}
	trigger, err := h.svc.UpdateRoutineTrigger(c.Request.Context(), c.Param("triggerId"), request)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, sql.ErrNoRows):
			status = http.StatusNotFound
		case errors.Is(err, ErrInvalidTrigger):
			status = http.StatusBadRequest
		case errors.Is(err, ErrTriggerUpdateConflict):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{jsonErrorKey: err.Error()})
		return
	}
	trigger.Secret = ""
	c.JSON(http.StatusOK, TriggerResponse{Trigger: trigger})
}
