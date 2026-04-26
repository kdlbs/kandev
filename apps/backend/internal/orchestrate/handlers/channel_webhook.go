package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// channelInbound handles inbound messages from external platforms.
// POST /api/v1/orchestrate/channels/:channelId/inbound
// For now, accepts a raw text body as the message content.
func (h *Handlers) channelInbound(c *gin.Context) {
	channelID := c.Param("channelId")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}
	text := string(body)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty message body"})
		return
	}

	authorName := c.Query("author")
	if authorName == "" {
		authorName = "external"
	}

	err = h.ctrl.Svc.HandleChannelInbound(c.Request.Context(), channelID, authorName, text)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
