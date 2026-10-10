package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func (m *ControlServer) handleRetainedDelivery(c *gin.Context) {
	var request journal.RetainedRecoveryRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.ShouldBindJSON(&request); err != nil || request.SessionID == "" || request.ExecutionID == "" || request.SubmissionID == "" || request.StreamID == "" || request.IncarnationID == "" || request.HarnessGeneration == 0 || request.Limit < 1 || request.Limit > 4 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RECOVERY_IDENTITY"})
		return
	}
	if m.instMgr == nil {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return
	}
	for _, instance := range m.instMgr.ListInstances() {
		if instance.ID == request.ExecutionID || instance.SessionID == request.SessionID {
			writeDeliveryError(c, journal.ErrOwnerMismatch)
			return
		}
	}
	result, err := journal.ReadRetainedRecovery(c.Request.Context(), request)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
