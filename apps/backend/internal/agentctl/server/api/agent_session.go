package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AgentSessionAssociation is authenticated evidence that binds a live native
// conversation to the Kandev session and delivery generation that own it.
type AgentSessionAssociation struct {
	InstanceID        string `json:"instance_id"`
	SessionID         string `json:"session_id"`
	IncarnationID     string `json:"incarnation_id"`
	HarnessGeneration uint64 `json:"harness_generation"`
	NativeSessionID   string `json:"native_session_id"`
	AgentStatus       string `json:"agent_status"`
}

func (s *Server) handleAgentSessionAssociation(c *gin.Context) {
	if s.procMgr == nil || s.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "AGENT_SESSION_UNAVAILABLE"})
		return
	}
	c.JSON(http.StatusOK, AgentSessionAssociation{
		InstanceID:        s.cfg.InstanceID,
		SessionID:         s.cfg.SessionID,
		IncarnationID:     s.cfg.DeliveryIncarnationID,
		HarnessGeneration: s.cfg.DeliveryHarnessGeneration,
		NativeSessionID:   s.procMgr.GetSessionID(),
		AgentStatus:       string(s.procMgr.Status()),
	})
}
