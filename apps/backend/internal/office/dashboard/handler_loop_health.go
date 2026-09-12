package dashboard

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/service"
)

// registerLoopHealthRoutes mounts the two REQ-OFFICE-LOOP-LIVENESS
// reads on the Office route group. Both inherit AgentAuthMiddleware
// and officeWorkspaceScopeMiddleware from the group they're mounted
// on; :wsId is their only id, so neither needs a
// officeParamScopeResolvers entry.
func registerLoopHealthRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/loop-health", h.getLoopHealth)
	api.GET("/workspaces/:wsId/loop-counters", h.getLoopCounters)
}

// getLoopHealth serves GET /workspaces/:wsId/loop-health
// (REQ-OFFICE-LOOP-LIVENESS-004). now is captured once here and
// threaded through every predicate (AC-004.14). Any unreadable input
// fails the whole read with 503 rather than a partial verdict
// (AC-004.10), incrementing office_loop_liveness_degraded_total by
// the failing input's reason.
func (h *Handler) getLoopHealth(c *gin.Context) {
	if h.loopHealth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: "loop health not configured"})
		return
	}
	wsID := c.Param("wsId")
	now := time.Now().UTC()

	resp, err := EvaluateLoopHealth(c.Request.Context(), h.loopHealth, wsID, now)
	if err != nil {
		h.respondLoopHealthError(c, wsID, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) respondLoopHealthError(c *gin.Context, workspaceID string, err error) {
	var degraded *LoopHealthDegradedError
	reason := ReasonRunReadFailed
	if errors.As(err, &degraded) {
		reason = degraded.Reason
	}
	service.IncLoopLivenessDegraded(workspaceID, reason)
	c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: err.Error(), "reason": reason})
}

// getLoopCounters serves GET /workspaces/:wsId/loop-counters
// (REQ-OFFICE-LOOP-LIVENESS-003). Reads process-wide expvar state, so
// it never fails or 503s — an empty workspace simply reads all zeros.
func (h *Handler) getLoopCounters(c *gin.Context) {
	wsID := c.Param("wsId")
	c.JSON(http.StatusOK, ReadLoopCounters(wsID))
}

// LoopHealthRepoImpl returns the wired loop-health repo (may be nil).
// Exposed for tests that need to confirm the wiring.
func (h *Handler) LoopHealthRepoImpl() LoopHealthRepo { return h.loopHealth }
