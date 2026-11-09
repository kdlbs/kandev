package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// handleRuntimeStorage serves the advisory capacity of this executor's
// filesystem. It reads the shared source cache and never invokes
// DeliveryHealth, journal pressure refresh, or a write-admission check.
func (s *Server) handleRuntimeStorage(c *gin.Context) {
	if s.procMgr == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "RUNTIME_STORAGE_UNAVAILABLE"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	snapshot := s.procMgr.RuntimeStorage(ctx)
	c.JSON(http.StatusOK, snapshot)
}

// promptErrorDetails forwards only validated storage evidence through the
// authenticated prompt error frame.
func promptErrorDetails(err error) map[string]interface{} {
	diagnostic := streams.DeliveryStorageDiagnosticFromError(err)
	if diagnostic == nil {
		return nil
	}
	return map[string]interface{}{"storage": diagnostic}
}
