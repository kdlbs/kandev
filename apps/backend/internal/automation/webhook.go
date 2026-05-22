package automation

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// WebhookHandler handles incoming webhook requests that fire automation triggers.
type WebhookHandler struct {
	svc    *Service
	logger *logger.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(svc *Service, log *logger.Logger) *WebhookHandler {
	return &WebhookHandler{svc: svc, logger: log}
}

// Handle processes an incoming webhook POST request.
// URL format: POST /api/v1/automations/webhook/:id  with X-Webhook-Secret header
func (h *WebhookHandler) Handle(c *gin.Context) {
	automationID := c.Param("id")
	if automationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "automation id required"})
		return
	}

	a, err := h.svc.GetAutomation(c.Request.Context(), automationID)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "automation not found"})
		return
	}
	if !a.Enabled {
		c.JSON(http.StatusConflict, gin.H{"error": "automation is disabled"})
		return
	}

	// Secret must come via header — query params would leak into URLs/logs.
	// Compared in constant time so an attacker can't recover the secret byte
	// by byte from timing differences.
	secret := c.GetHeader("X-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(secret), []byte(a.WebhookSecret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook secret"})
		return
	}

	// Find the first enabled webhook trigger for this automation. Bail out if
	// none — firing with an empty trigger ID papers over a misconfiguration.
	triggerID := ""
	for _, t := range a.Triggers {
		if t.Type == TriggerTypeWebhook && t.Enabled {
			triggerID = t.ID
			break
		}
	}
	if triggerID == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "no enabled webhook trigger"})
		return
	}

	// Read body as trigger data.
	body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
	if readErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Ensure valid JSON; wrap raw text if needed.
	triggerData := json.RawMessage(body)
	if len(body) == 0 || !json.Valid(body) {
		triggerData, _ = json.Marshal(map[string]string{"body": string(body)})
	}

	if fireErr := h.svc.FireTrigger(c.Request.Context(), automationID, triggerID, TriggerTypeWebhook, triggerData, ""); fireErr != nil {
		h.logger.Error("failed to fire webhook trigger",
			zap.String("automation_id", automationID),
			zap.Error(fireErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "trigger failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "triggered"})
}
