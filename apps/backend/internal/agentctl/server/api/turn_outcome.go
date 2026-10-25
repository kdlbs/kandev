package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleGetTurnOutcome implements the read side of
// AC-EXECUTORS-SURVIVAL-004.2/.6: a repeatable, non-discarding read of the
// named instance's last retained terminal turn outcome. 404s only when the
// instance itself is unknown; "nothing retained" is a normal 200 answer
// (the case AC-EXECUTORS-SURVIVAL-004.5 maps to "publish as running").
func (m *ControlServer) handleGetTurnOutcome(c *gin.Context) {
	id := c.Param("id")

	outcome, hasOutcome, instanceFound := m.instMgr.PeekTurnOutcome(id)
	if !instanceFound {
		c.JSON(http.StatusNotFound, gin.H{errKey: instanceNotFoundMessage})
		return
	}
	if !hasOutcome {
		c.JSON(http.StatusOK, gin.H{"retained": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"retained": true,
		"turn_id":  outcome.TurnID,
		"event":    outcome.Event,
	})
}

type ackTurnOutcomeRequest struct {
	TurnID int64 `json:"turn_id" binding:"required"`
}

// handleAckTurnOutcome implements AC-EXECUTORS-SURVIVAL-004.6's
// acknowledgement: discards the retained outcome only when turn_id names the
// one currently retained. An identifier the server no longer holds, or never
// held, is accepted and changes nothing -- so this always answers 200,
// making a retried acknowledgement safe to send again.
func (m *ControlServer) handleAckTurnOutcome(c *gin.Context) {
	id := c.Param("id")

	var req ackTurnOutcomeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "turn_id is required"})
		return
	}

	m.instMgr.AckTurnOutcome(id, req.TurnID)
	c.JSON(http.StatusOK, gin.H{lspStatusKey: "acked"})
}
