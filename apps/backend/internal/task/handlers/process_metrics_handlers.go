package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	agentctlmetrics "github.com/kandev/kandev/internal/system/metrics"
)

var sessionAgentctlDiagnosticMetricIDs = []string{
	agentctlmetrics.MetricAgentctlGoroutines,
	agentctlmetrics.MetricAgentctlGitPollMillis,
	agentctlmetrics.MetricAgentctlMonitorMillis,
	agentctlmetrics.MetricAgentctlCreateReadyMs,
}

// httpGetAgentctlMetrics returns the fixed set of diagnostics for one live
// session. The route is session-scoped and uses the existing backend auth
// middleware, so callers do not need the agentctl instance token. Keeping the
// metric list server-owned also prevents this endpoint from becoming a proxy
// for arbitrary agentctl requests.
func (h *ProcessHandlers) httpGetAgentctlMetrics(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	if h.denySessionAccess(c, sessionID) {
		return
	}

	execution, found := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !found || execution == nil || execution.GetAgentCtlClient() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agentctl not ready"})
		return
	}

	snapshot, err := execution.GetAgentCtlClient().SystemMetrics(
		c.Request.Context(), sessionAgentctlDiagnosticMetricIDs, "",
	)
	if err != nil {
		h.logger.Warn("failed to collect agentctl diagnostics",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{"error": "agentctl metrics unavailable"})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
