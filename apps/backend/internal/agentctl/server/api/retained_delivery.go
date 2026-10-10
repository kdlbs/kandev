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

func (m *ControlServer) handleRetainedReconstructionEvidence(c *gin.Context) {
	var request journal.RetainedReconstructionEvidenceRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.ShouldBindJSON(&request); err != nil || request.SessionID == "" || request.IncarnationID == "" || request.HarnessGeneration == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RECOVERY_IDENTITY"})
		return
	}
	if !m.retainedSessionIsUnowned(c, request.SessionID) {
		return
	}
	result, err := journal.ReadRetainedReconstructionEvidence(c.Request.Context(), request)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (m *ControlServer) handleRetainedReconstructionSubmission(c *gin.Context) {
	var request journal.RetainedReconstructionSubmissionRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if err := c.ShouldBindJSON(&request); err != nil || request.SessionID == "" || request.IncarnationID == "" || request.HarnessGeneration == 0 || request.Candidate.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RECOVERY_IDENTITY"})
		return
	}
	if !m.retainedSessionIsUnowned(c, request.SessionID) {
		return
	}
	result, err := journal.ReadRetainedReconstructionSubmission(c.Request.Context(), request)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (m *ControlServer) retainedSessionIsUnowned(c *gin.Context, sessionID string) bool {
	if m.instMgr == nil {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	for _, instance := range m.instMgr.ListInstances() {
		if instance.SessionID == sessionID {
			writeDeliveryError(c, journal.ErrOwnerMismatch)
			return false
		}
	}
	return true
}
