package websocket

import (
	"net/http"

	"github.com/gin-gonic/gin"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
)

func (h *TerminalHandler) handleRemoteAgentTerminal(c *gin.Context, execution *runtimeapi.AgentExecution) {
	upstream, err := h.lifecycleMgr.StreamRemotePassthrough(c.Request.Context(), execution)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer func() { _ = upstream.Close() }()
	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	h.bridgeBinaryWebSockets(conn, upstream, execution.SessionID)
}
