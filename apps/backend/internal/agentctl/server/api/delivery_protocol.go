package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func (s *Server) deliveryProtocolGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.cfg == nil || s.cfg.DurableJournalPath == "" || !deliveryProtocolRequired(c.Request.URL.Path) {
			c.Next()
			return
		}
		if c.GetHeader(journal.DeliveryProtocolHeader) != "2" {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"code": "INCOMPATIBLE_DELIVERY_VERSION", "supported_version": journal.CurrentProtocolVersion})
			return
		}
		c.Next()
	}
}

func deliveryProtocolRequired(path string) bool {
	if strings.HasSuffix(path, "/cancel") {
		return false
	}
	return path == "/api/v1/agent/stream" || strings.HasPrefix(path, "/api/v1/agent/submissions") || strings.HasPrefix(path, "/api/v1/agent/delivery/stream")
}
