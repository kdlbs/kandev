package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

type deliverySubmissionRequest struct {
	ID                string `json:"id"`
	SessionID         string `json:"session_id"`
	IncarnationID     string `json:"incarnation_id"`
	HarnessGeneration uint64 `json:"harness_generation"`
	Hash              string `json:"hash"`
	Payload           []byte `json:"payload"`
}

type deliveryAcknowledgementRequest struct {
	StreamID string `json:"stream_id"`
	Sequence uint64 `json:"sequence"`
}

const deliverySubmissionRequestOverhead int64 = 64 << 10

func (s *Server) getDeliveryJournal(c *gin.Context) (*journal.Journal, bool) {
	if s.procMgr == nil {
		c.JSON(http.StatusConflict, gin.H{
			"code":    "DURABLE_DELIVERY_UNAVAILABLE",
			"message": "durable delivery process manager is unavailable",
		})
		return nil, false
	}
	deliveryJournal, err := s.procMgr.DeliveryJournal()
	if err == nil {
		return deliveryJournal, true
	}
	c.JSON(http.StatusConflict, gin.H{
		"code":    "DURABLE_DELIVERY_UNAVAILABLE",
		"message": "durable delivery storage is unavailable",
	})
	return nil, false
}

func (s *Server) handleDeliveryStatus(c *gin.Context) {
	if s.procMgr != nil {
		capability := s.procMgr.DeliveryCapability()
		if !capability.Durable && capability.Reason == "storage_not_durable" {
			c.JSON(http.StatusOK, capability)
			return
		}
	}
	deliveryJournal, ok := s.getDeliveryJournal(c)
	if !ok {
		return
	}
	streamID := c.Query("stream_id")
	response := gin.H{"version": journal.CurrentVersion, "durable": true}
	if streamID != "" {
		if !s.validateDeliveryStreamID(c, streamID) {
			return
		}
		stream, err := deliveryJournal.GetStream(c.Request.Context(), streamID)
		if err != nil {
			writeDeliveryError(c, err)
			return
		}
		if !s.validateDeliveryStream(c, stream) {
			return
		}
		response["stream"] = stream
	}
	c.JSON(http.StatusOK, response)
}

func (s *Server) handleDeliverySubmission(c *gin.Context) {
	deliveryJournal, ok := s.getDeliveryJournal(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDeliverySubmissionRequestBytes(deliveryJournal.MaxEventBytes()))
	var request deliverySubmissionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "SUBMISSION_TOO_LARGE"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_SUBMISSION", "message": err.Error()})
		return
	}
	submission := journal.Submission{
		ID: request.ID, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: request.HarnessGeneration, Hash: request.Hash, Payload: request.Payload,
	}
	if err := s.bindDeliverySubmissionOwner(&submission); err != nil {
		writeDeliveryError(c, err)
		return
	}
	submission, err := s.procMgr.AdmitDeliverySubmission(c.Request.Context(), submission)
	if err != nil {
		status := http.StatusInternalServerError
		code := "DURABLE_DELIVERY_ERROR"
		switch {
		case errors.Is(err, journal.ErrSubmissionConflict):
			status = http.StatusConflict
			code = "SUBMISSION_CONFLICT"
		case errors.Is(err, journal.ErrStreamFull):
			status = http.StatusRequestEntityTooLarge
			code = "SUBMISSION_TOO_LARGE"
		}
		c.JSON(status, gin.H{"code": code, "message": code})
		return
	}
	c.JSON(http.StatusOK, submission)
}

func (s *Server) handleDeliverySubmissionByID(c *gin.Context) {
	deliveryJournal, ok := s.getDeliveryJournal(c)
	if !ok {
		return
	}
	submission, err := deliveryJournal.GetSubmission(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, journal.ErrSubmissionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "SUBMISSION_NOT_FOUND"})
			return
		}
		writeDeliveryError(c, err)
		return
	}
	if !s.validateDeliverySubmission(c, submission) {
		return
	}
	c.JSON(http.StatusOK, submission)
}

func (s *Server) handleDeliverySubmissions(c *gin.Context) {
	deliveryJournal, ok := s.getDeliveryJournal(c)
	if !ok {
		return
	}
	if s.procMgr == nil {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return
	}
	sessionID, _, _ := s.procMgr.DeliverySubmissionIdentity()
	if c.Query("session_id") != sessionID {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return
	}
	submissions, err := deliveryJournal.ListSubmissions(c.Request.Context(), c.Query("session_id"))
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, submissions)
}

func (s *Server) handleDeliverySubmissionRetire(c *gin.Context) {
	if _, ok := s.getDeliveryJournal(c); !ok {
		return
	}
	if s.procMgr == nil {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return
	}
	if err := s.procMgr.RetireDeliverySubmission(
		c.Request.Context(), c.Param("id"), s.procMgr.DeliveryHarnessGeneration(),
	); err != nil {
		writeDeliveryError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleDeliveryReplay(c *gin.Context) {
	deliveryJournal, ok := s.getDeliveryJournal(c)
	if !ok {
		return
	}
	streamID := c.Query("stream_id")
	if streamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "STREAM_REQUIRED"})
		return
	}
	if !s.validateDeliveryStreamID(c, streamID) {
		return
	}
	after, err := parseDeliveryUint(c.Query("after"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_CURSOR"})
		return
	}
	limit := 1000
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_LIMIT"})
			return
		}
	}
	events, stream, err := deliveryJournal.Replay(c.Request.Context(), streamID, after, limit)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	if !s.validateDeliveryStream(c, stream) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events, "stream": stream})
}

func (s *Server) handleDeliveryAcknowledgement(c *gin.Context) {
	deliveryJournal, ok := s.getDeliveryJournal(c)
	if !ok {
		return
	}
	var request deliveryAcknowledgementRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.StreamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_ACKNOWLEDGEMENT"})
		return
	}
	if !s.validateDeliveryStreamID(c, request.StreamID) {
		return
	}
	stream, err := deliveryJournal.GetStream(c.Request.Context(), request.StreamID)
	if err != nil {
		writeDeliveryError(c, err)
		return
	}
	if !s.validateDeliveryStream(c, stream) {
		return
	}
	if err := deliveryJournal.Acknowledge(c.Request.Context(), request.StreamID, request.Sequence); err != nil {
		writeDeliveryError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func maxDeliverySubmissionRequestBytes(payloadLimit int64) int64 {
	if payloadLimit <= 0 {
		return deliverySubmissionRequestOverhead
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if payloadLimit > (maxInt64-deliverySubmissionRequestOverhead)/2 {
		return maxInt64
	}
	return payloadLimit*2 + deliverySubmissionRequestOverhead
}

func (s *Server) validateDeliveryStreamID(c *gin.Context, streamID string) bool {
	if s.procMgr == nil || streamID != s.procMgr.DeliveryStreamID() {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	return true
}

func (s *Server) validateDeliveryStream(c *gin.Context, stream journal.Stream) bool {
	if !s.validateDeliveryStreamID(c, stream.StreamID) {
		return false
	}
	sessionID, incarnationID, generation := s.procMgr.DeliverySubmissionIdentity()
	if stream.SessionID != sessionID || stream.IncarnationID != incarnationID || stream.HarnessGeneration != generation {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	return true
}

func (s *Server) bindDeliverySubmissionOwner(submission *journal.Submission) error {
	if s.procMgr == nil {
		return journal.ErrOwnerMismatch
	}
	sessionID, incarnationID, generation := s.procMgr.DeliverySubmissionIdentity()
	if submission.SessionID == "" && submission.IncarnationID == "" && submission.HarnessGeneration == 0 {
		submission.SessionID = sessionID
		submission.IncarnationID = incarnationID
		submission.HarnessGeneration = generation
	}
	if submission.SessionID != sessionID || submission.IncarnationID != incarnationID || submission.HarnessGeneration != generation {
		return journal.ErrOwnerMismatch
	}
	return nil
}

func (s *Server) validateDeliverySubmission(c *gin.Context, submission journal.Submission) bool {
	if s.procMgr == nil {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	sessionID, incarnationID, generation := s.procMgr.DeliverySubmissionIdentity()
	legacy := submission.SessionID == "" && submission.IncarnationID == "" && submission.HarnessGeneration == 0
	if !legacy && (submission.SessionID != sessionID || submission.IncarnationID != incarnationID || submission.HarnessGeneration != generation) {
		writeDeliveryError(c, journal.ErrOwnerMismatch)
		return false
	}
	return true
}

// loadAgentStreamReplay reads the retained prefix before the live update
// channel is selected. Agentctl commits events before publishing them, so a
// backend reconnect can safely use this boundary without racing a live event.
func (s *Server) loadAgentStreamReplay(ctx context.Context, after uint64) ([]adapter.AgentEvent, error) {
	capability := s.procMgr.DeliveryCapability()
	if !capability.Durable {
		if capability.Reason == "storage_not_durable" {
			return nil, nil
		}
		return nil, errors.New("durable delivery storage is unavailable")
	}
	deliveryJournal, err := s.procMgr.DeliveryJournal()
	if err != nil {
		return nil, err
	}
	streamID := s.procMgr.DeliveryStreamID()
	events, stream, err := deliveryJournal.Replay(ctx, streamID, after, 1000)
	if err != nil {
		// A first connection has no cursor and no retained stream yet. That is
		// a normal empty history, not a cursor loss.
		if after == 0 && errors.Is(err, journal.ErrStreamNotFound) {
			return nil, nil
		}
		return nil, err
	}
	boundary := stream.HighWater
	replay := make([]adapter.AgentEvent, 0, len(events))
	appendEvents := func(page []journal.Event) error {
		for _, event := range page {
			var notification adapter.AgentEvent
			if err := json.Unmarshal(event.Payload, &notification); err != nil {
				return err
			}
			notification.DeliveryStreamID = event.StreamID
			notification.DeliveryIncarnationID = event.IncarnationID
			notification.DeliveryHarnessGeneration = event.HarnessGeneration
			notification.DeliverySequence = event.Sequence
			notification.DeliverySubmissionID = event.SubmissionID
			replay = append(replay, notification)
			after = event.Sequence
		}
		return nil
	}
	if err := appendEvents(events); err != nil {
		return nil, err
	}
	for after < boundary {
		page, pageStream, pageErr := deliveryJournal.Replay(ctx, streamID, after, 1000)
		if pageErr != nil {
			return nil, pageErr
		}
		if len(page) == 0 {
			return nil, fmt.Errorf("durable delivery replay stopped before high-water %d", boundary)
		}
		if pageStream.HighWater < boundary {
			return nil, fmt.Errorf("durable delivery replay high-water regressed from %d to %d", boundary, pageStream.HighWater)
		}
		if err := appendEvents(page); err != nil {
			return nil, err
		}
	}
	return replay, nil
}

func parseDeliveryUint(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func writeDeliveryError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "DURABLE_DELIVERY_ERROR"
	switch {
	case errors.Is(err, journal.ErrCursorExpired):
		status, code = http.StatusGone, "CURSOR_EXPIRED"
	case errors.Is(err, journal.ErrStreamNotFound):
		status, code = http.StatusNotFound, "STREAM_NOT_FOUND"
	case errors.Is(err, journal.ErrSequenceConflict):
		status, code = http.StatusConflict, "SEQUENCE_CONFLICT"
	case errors.Is(err, journal.ErrOwnerMismatch):
		status, code = http.StatusConflict, "OWNER_MISMATCH"
	case errors.Is(err, journal.ErrSubmissionNotFound):
		status, code = http.StatusNotFound, "SUBMISSION_NOT_FOUND"
	case errors.Is(err, journal.ErrSubmissionGeneration):
		status, code = http.StatusConflict, "SUBMISSION_GENERATION_CONFLICT"
	}
	c.JSON(status, gin.H{"code": code, "message": code})
}
