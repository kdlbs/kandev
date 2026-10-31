package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

func (h *Handlers) httpGetTurnAllowance(c *gin.Context) {
	result, err := h.controller.GetProfileTurnAllowance(c.Request.Context(), c.Param("id"))
	turnAllowanceResponse(c, result, err)
}

func (h *Handlers) httpPatchTurnAllowance(c *gin.Context) {
	if !h.controller.ProviderTurnAllowancesEnabled() {
		turnAllowanceResponse(c, nil, controller.ErrProviderUsageDisabled)
		return
	}
	var patch json.RawMessage
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid turn allowance"})
		return
	}
	result, err := h.controller.PatchProfileTurnAllowance(c.Request.Context(), c.Param("id"), patch)
	turnAllowanceResponse(c, result, err)
}

func turnAllowanceResponse(c *gin.Context, result interface{}, err error) {
	switch {
	case errors.Is(err, controller.ErrProviderUsageDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": "provider usage limits are disabled"})
	case errors.Is(err, controller.ErrAgentProfileNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "agent profile not found"})
	case errors.Is(err, agentusage.ErrTurnAllowanceInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid turn allowance"})
	case errors.Is(err, agentusage.ErrTurnAllowanceUnknown):
		c.JSON(http.StatusConflict, gin.H{"error": "local execution account binding is unknown"})
	case err != nil:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "turn allowance is unavailable"})
	default:
		c.JSON(http.StatusOK, result)
	}
}
