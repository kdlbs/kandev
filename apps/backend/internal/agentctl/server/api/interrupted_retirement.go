package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func (s *Server) prepareDeliverySubmissionRetirement(c *gin.Context) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST"})
		return false
	}
	var missingPeer struct {
		SessionID         string `json:"session_id"`
		IncarnationID     string `json:"incarnation_id"`
		HarnessGeneration uint64 `json:"harness_generation"`
		StreamID          string `json:"stream_id"`
		Hash              string `json:"hash"`
	}
	if err := json.Unmarshal(body, &missingPeer); err == nil && missingPeer.Hash != "" {
		submission := journal.Submission{
			ID: c.Param("id"), SessionID: missingPeer.SessionID, IncarnationID: missingPeer.IncarnationID,
			HarnessGeneration: missingPeer.HarnessGeneration, StreamID: missingPeer.StreamID,
			Hash: missingPeer.Hash, State: journal.SubmissionInterruptedUnknown,
		}
		if err := s.procMgr.RetireMissingUnknownDeliverySubmission(c.Request.Context(), submission); err != nil {
			writeDeliveryError(c, err)
			return false
		}
		c.Status(http.StatusNoContent)
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return s.validateInterruptedRetirement(c)
}

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
