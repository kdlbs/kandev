package channels

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
)

// Handler provides HTTP handlers for channel routes.
type Handler struct {
	svc *ChannelService
}

// NewHandler creates a new channel Handler.
func NewHandler(svc *ChannelService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers channel routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/agents/:id/channels", h.listChannels)
	api.POST("/agents/:id/channels", h.setupChannel)
	api.DELETE("/agents/:id/channels/:channelId", h.deleteChannel)
	api.POST("/channels/:channelId/inbound", h.channelInbound)
}

func (h *Handler) listChannels(c *gin.Context) {
	channels, err := h.svc.ListChannelsByAgent(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ChannelListResponse{Channels: channels})
}

func (h *Handler) setupChannel(c *gin.Context) {
	var req CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	channel := &models.Channel{
		WorkspaceID:    req.WorkspaceID,
		AgentProfileID: c.Param("id"),
		Platform:       req.Platform,
		Config:         req.Config,
		Status:         req.Status,
	}
	if err := h.svc.SetupChannel(c.Request.Context(), channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"channel": channel})
}

func (h *Handler) deleteChannel(c *gin.Context) {
	if err := h.svc.DeleteChannel(c.Request.Context(), c.Param("channelId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// channelInbound handles inbound messages from external platforms.
// POST /api/v1/office/channels/:channelId/inbound
//
// Security notes:
//   - Body is limited to 64 KiB to prevent resource exhaustion.
//   - The ?author= query param is ignored; the sender identity is always "external"
//     to prevent callers from impersonating internal agents.
//   - Signature verification is performed when a webhook_secret is configured on the
//     channel: HMAC-SHA256 for Slack/Discord/generic platforms, raw token comparison
//     for Telegram (X-Telegram-Bot-Api-Secret-Token header).
func (h *Handler) channelInbound(c *gin.Context) {
	channelID := c.Param("channelId")
	channel, err := h.svc.GetChannelByID(c.Request.Context(), channelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
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
	if channel.WebhookSecret != "" && !verifyWebhookSignature(channel.WebhookSecret, channel.Platform, body, webhookSignature(c)) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// Always use "external" as the author regardless of the query param to
	// prevent callers from spoofing internal agent identities.
	const authorName = "external"

	if err := h.svc.HandleChannelInbound(c.Request.Context(), channelID, authorName, text); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func webhookSignature(c *gin.Context) string {
	for _, header := range []string{
		"X-Webhook-Signature",
		"X-Hub-Signature-256",
		"X-Telegram-Bot-Api-Secret-Token",
	} {
		if value := c.GetHeader(header); value != "" {
			return value
		}
	}
	return ""
}

func signWebhookBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyWebhookSignature(secret, platform string, body []byte, signature string) bool {
	if secret == "" {
		return true
	}
	if signature == "" {
		return false
	}
	switch platform {
	case "telegram":
		// Telegram sends the raw secret token in X-Telegram-Bot-Api-Secret-Token.
		return hmac.Equal([]byte(signature), []byte(secret))
	default:
		// Slack, Discord, and generic webhooks use HMAC-SHA256.
		expected := signWebhookBody(secret, body)
		return hmac.Equal([]byte(signature), []byte(expected))
	}
}
