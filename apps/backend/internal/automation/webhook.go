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

const (
	responseErrorKey  = "error"
	responseStatusKey = "status"
	// responseStatusTriggered is the fixed value returned for every
	// well-formed webhook POST, regardless of outcome — not to be confused
	// with RunStatusTriggered, an unrelated run lifecycle state.
	responseStatusTriggered = "triggered"
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
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "automation id required"})
		return
	}

	a, err := h.svc.GetAutomation(c.Request.Context(), automationID)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{responseErrorKey: "automation not found"})
		return
	}
	if !a.Enabled {
		c.JSON(http.StatusConflict, gin.H{responseErrorKey: "automation is disabled"})
		return
	}

	// Secret must come via header — query params would leak into URLs/logs.
	// Compared in constant time so an attacker can't recover the secret byte
	// by byte from timing differences.
	secret := c.GetHeader("X-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(secret), []byte(a.WebhookSecret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{responseErrorKey: "invalid webhook secret"})
		return
	}

	// Find the first enabled webhook trigger for this automation. Bail out if
	// none — firing with an empty trigger ID papers over a misconfiguration.
	trigger := firstEnabledWebhookTrigger(a)
	if trigger == nil {
		c.JSON(http.StatusConflict, gin.H{responseErrorKey: "no enabled webhook trigger"})
		return
	}
	var cfg WebhookTriggerConfig
	_ = json.Unmarshal(trigger.Config, &cfg)

	// Read body as trigger data.
	body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
	if readErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "failed to read body"})
		return
	}

	// Ensure valid JSON; wrap raw text if needed.
	triggerData := json.RawMessage(body)
	if len(body) == 0 || !json.Valid(body) {
		triggerData, _ = json.Marshal(map[string]string{"body": string(body)})
	}

	// Filters run before dedup and outside any lock — pure evaluation, no
	// store access. The response is always 200 regardless of outcome (see
	// below) so a rejecting filter configuration can never be probed from a
	// well-formed POST's HTTP status.
	var payload map[string]interface{}
	_ = json.Unmarshal(triggerData, &payload)
	if rejectedIndex, ok := EvaluateFilters(cfg.Filters, payload); !ok {
		if recordErr := h.svc.RecordFilteredTrigger(
			c.Request.Context(), a, trigger.ID, TriggerTypeWebhook, triggerData, rejectedIndex,
		); recordErr != nil {
			h.logger.Warn("failed to record filtered webhook trigger",
				zap.String("automation_id", automationID), zap.Error(recordErr))
		}
		c.JSON(http.StatusOK, gin.H{responseStatusKey: responseStatusTriggered})
		return
	}

	dedup := resolveWebhookDedupBinding(cfg.DedupKey, triggerData)

	if _, fireErr := h.svc.FireTrigger(c.Request.Context(), automationID, trigger.ID, TriggerTypeWebhook, triggerData, dedup); fireErr != nil {
		h.logger.Error("failed to fire webhook trigger",
			zap.String("automation_id", automationID),
			zap.Error(fireErr))
		c.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "trigger failed"})
		return
	}

	// A well-formed POST always answers 200 — whether the trigger fired, was
	// filtered, or was deduplicated. A non-2xx status would make the sender
	// retry, and echoing back which of those three happened would give a
	// secret-holder an oracle for probing filter/dedup configuration.
	c.JSON(http.StatusOK, gin.H{responseStatusKey: responseStatusTriggered})
}

func firstEnabledWebhookTrigger(a *Automation) *AutomationTrigger {
	for i := range a.Triggers {
		if a.Triggers[i].Type == TriggerTypeWebhook && a.Triggers[i].Enabled {
			return &a.Triggers[i]
		}
	}
	return nil
}

// resolveWebhookDedupBinding resolves a webhook trigger's declared dedup key
// path against the payload: trim, then test non-empty, then namespace with
// "webhook:" — a value that trims to empty is treated as unresolved, never
// becoming the literal key "webhook:   ".
func resolveWebhookDedupBinding(dedupKeyPath string, triggerData json.RawMessage) DedupBinding {
	if dedupKeyPath == "" {
		return DedupNotConfigured()
	}
	value, ok := ResolvePayloadPath(triggerData, dedupKeyPath)
	if !ok {
		return DedupUnresolved()
	}
	return DedupKey("webhook:" + value)
}
