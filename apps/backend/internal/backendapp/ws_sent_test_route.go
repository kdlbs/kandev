package backendapp

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
)

type wsSentSource interface {
	GetSentEventsFor(connectionID string, sinceConnectionSeq int64) ([]gateways.WsSentEvent, int64, bool)
	GetSentEventsForSession(connectionID, sessionID string) ([]gateways.WsSentEvent, int64, bool)
}

func registerWsSentTestRoute(router *gin.Engine, source wsSentSource, log *logger.Logger) {
	if source == nil {
		return
	}
	group := router.Group("/api/v1/_test")
	group.GET("/ws-sent", handleWsSentTestRoute(source, log))
}

func handleWsSentTestRoute(source wsSentSource, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		connectionID := c.Query("connection_id")
		if connectionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection_id is required"})
			return
		}
		sinceSeq, ok := parseSinceConnectionSeq(c)
		if !ok {
			return
		}
		sessionID := c.Query("session_id")
		if sessionID != "" {
			respondWsSentSession(c, source, connectionID, sessionID)
			return
		}
		events, maxSeq, found := source.GetSentEventsFor(connectionID, sinceSeq)
		if !found {
			if log != nil {
				log.Debug("ws sent-log connection not found")
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "connection_id not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"connection_id":      connectionID,
			"events":             events,
			"max_connection_seq": maxSeq,
		})
	}
}

func parseSinceConnectionSeq(c *gin.Context) (int64, bool) {
	raw := c.Query("since_seq")
	if raw == "" {
		return 0, true
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "since_seq must be a non-negative integer"})
		return 0, false
	}
	return seq, true
}

func respondWsSentSession(
	c *gin.Context,
	source wsSentSource,
	connectionID string,
	sessionID string,
) {
	events, maxSeq, found := source.GetSentEventsForSession(connectionID, sessionID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection_id not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"connection_id":   connectionID,
		"session_id":      sessionID,
		"events":          events,
		"max_session_seq": maxSeq,
	})
}
