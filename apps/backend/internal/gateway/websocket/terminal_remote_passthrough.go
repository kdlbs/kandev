package websocket

import (
	"net/http"

	"github.com/gin-gonic/gin"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/securityutil"
	"go.uber.org/zap"
)

func (h *TerminalHandler) handleRemoteAgentTerminal(c *gin.Context, execution *runtimeapi.AgentExecution) {
	upstream, err := h.lifecycleMgr.StreamRemotePassthrough(c.Request.Context(), execution)
	if err != nil {
		safeError := securityutil.SanitizeError(err)
		h.logger.Warn("failed to acquire remote passthrough stream",
			zap.String("stage", "remote_stream_acquisition"),
			zap.String("session_id", execution.SessionID),
			zap.Error(safeError))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": safeError.Error()})
		return
	}
	defer func() { _ = upstream.Close() }()
	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade remote passthrough WebSocket", zap.String("session_id", execution.SessionID), zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()
	h.bridgeBinaryWebSockets(conn, upstream, execution.SessionID)
}
