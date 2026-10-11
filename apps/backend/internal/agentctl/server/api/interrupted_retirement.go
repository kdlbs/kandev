package api

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"net/http"
)

func (s *Server) validateInterruptedRetirement(c *gin.Context) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var observed struct {
		StreamID   string `json:"stream_id"`
		Generation uint64 `json:"harness_generation"`
	}
	if err := c.ShouldBindJSON(&observed); err != nil || observed.StreamID == "" || observed.Generation == 0 || observed.Generation+1 != s.procMgr.DeliveryHarnessGeneration() {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	retained, err := s.procMgr.DeliveryJournal()
	if err != nil {
		writeDeliveryError(c, err)
		return false
	}
	submission, err := retained.GetSubmission(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeDeliveryError(c, err)
		return false
	}
	if submission.StreamID != observed.StreamID || submission.HarnessGeneration != observed.Generation {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	return true
}
