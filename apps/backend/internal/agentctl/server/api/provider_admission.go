package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const agentActionStderr = "agent.stderr"

func (s *Server) providerAdmissionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		release, err := s.procMgr.BeginProviderOperation("")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{errKey: "provider replacement is in progress", "reason": "provider_replacement_pending"})
			return
		}
		defer release()
		c.Next()
	}
}

func providerActionNeedsAdmission(action string) bool {
	switch action {
	case agentActionStderr, "agent.permissions.list", "agent.permissions.cancel", "agent.cancel", "agent.background.probe", "agent.auth.status":
		return false
	default:
		return true
	}
}

func (s *Server) providerAdmissionError(msg *ws.Message) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "provider replacement is in progress", map[string]any{"reason": "provider_replacement_pending"})
	return response
}
