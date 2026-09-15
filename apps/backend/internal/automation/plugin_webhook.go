package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	ws "github.com/kandev/kandev/pkg/websocket"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

const pluginWebhookLimit = 1 << 20
const webhookAccepted = "accepted"
const webhookIgnored = "ignored"

func parsePluginEventConfig(raw json.RawMessage) (PluginEventConfig, error) {
	var cfg PluginEventConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid plugin event config")
	}
	if cfg.PluginID == "" || cfg.ConditionKey == "" || cfg.ConfigVersion < 1 {
		return cfg, fmt.Errorf("plugin, condition and positive config version required")
	}
	var settings map[string]any
	if json.Unmarshal(cfg.Settings, &settings) != nil || settings == nil {
		return cfg, fmt.Errorf("condition settings must be an object")
	}
	return cfg, nil
}

func validateConditionSettings(c manifest.AutomationCondition, raw []byte) error {
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil || values == nil || len(raw) > 65536 {
		return fmt.Errorf("invalid condition settings")
	}
	props, _ := c.ConfigSchema["properties"].(map[string]any)
	for key, value := range values {
		prop, ok := props[key].(map[string]any)
		if !ok {
			return fmt.Errorf("unknown condition field %q", key)
		}
		if !manifest.ValidAutomationValue(prop, value) {
			return fmt.Errorf("invalid condition field %q", key)
		}
	}
	if required, ok := c.ConfigSchema["required"].([]any); ok {
		for _, item := range required {
			key, _ := item.(string)
			v, found := values[key]
			if text, _ := v.(string); !found || (propIsString(props[key]) && text == "") {
				return fmt.Errorf("condition field %q required", key)
			}
		}
	}
	return nil
}
func bindingRevision(t *AutomationTrigger, generation string, connection string) string {
	sum := sha256.Sum256([]byte(generation + "\x00" + connection + "\x00" + t.UpdatedAt.UTC().Format(time.RFC3339Nano) + "\x00" + string(t.Config)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) configureWebhookBinding(ctx context.Context, triggerID string, rotate bool) (*WebhookBinding, error) {
	t, err := s.GetTrigger(ctx, triggerID)
	if err != nil {
		return nil, err
	}
	if t == nil || t.Type != TriggerTypePluginEvent {
		return nil, fmt.Errorf("plugin condition required")
	}
	a, err := s.GetAutomation(ctx, t.AutomationID)
	if err != nil || a == nil {
		return nil, fmt.Errorf("automation unavailable")
	}
	if s.pluginAutomation == nil {
		return nil, fmt.Errorf("plugin adapters unavailable")
	}
	cfg, err := parsePluginEventConfig(t.Config)
	if err != nil {
		return nil, err
	}
	c, generation, adapter, release, err := s.pluginAutomation.AcquireAutomationAdapter(cfg.PluginID, cfg.ConditionKey)
	if err != nil {
		return nil, err
	}
	defer release()
	if cfg.ConfigVersion != c.ConfigVersion {
		return nil, fmt.Errorf("condition config version unavailable")
	}
	if err = validateConditionSettings(c, cfg.Settings); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := adapter.DescribeAutomationCondition(callCtx, &pluginsdk.AutomationConditionRequest{WorkspaceId: a.WorkspaceID, ConditionKey: cfg.ConditionKey, Config: cfg.Settings})
	if err != nil || !validWebhookDescription(resp) {
		return nil, fmt.Errorf("condition or connection unavailable")
	}
	return s.saveConfiguredWebhook(ctx, t, a, resp, generation, rotate)
}
func (s *Service) saveConfiguredWebhook(ctx context.Context, t *AutomationTrigger, a *Automation, resp *pluginsdk.AutomationConditionResponse, generation string, rotate bool) (*WebhookBinding, error) {
	unlock := s.automationRunLock(a.ID)
	defer unlock()
	currentTrigger, err := s.store.GetTrigger(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	if currentTrigger == nil || !currentTrigger.UpdatedAt.Equal(t.UpdatedAt) || string(currentTrigger.Config) != string(t.Config) {
		return nil, fmt.Errorf("condition changed; retry configuration")
	}
	b, err := s.store.webhookBinding(ctx, t.ID, true)
	if err != nil {
		return nil, err
	}
	if b == nil {
		b = &WebhookBinding{ID: uuid.NewString(), TriggerID: t.ID, AutomationID: a.ID, WorkspaceID: a.WorkspaceID}
	}
	if err := s.store.enqueueWebhookSecrets(ctx, "trigger_id", t.ID); err != nil {
		return nil, err
	}
	oldSecret := b.SecretID
	changed := b.Revision != bindingRevision(t, generation, resp.ConnectionRevision)+":"+b.SecretID
	if rotate || changed || b.SecretID == "" {
		if err := s.cancelTriggerWebhookReceipts(ctx, t.ID, "binding reconfigured"); err != nil {
			return nil, err
		}
		b.SecretID = "automation-webhook:" + uuid.NewString()
		if err = s.pluginAutomation.SetAutomationSecret(ctx, b.SecretID, generateSecret()); err != nil {
			return nil, err
		}
	}
	b.ConnectionID = resp.ConnectionId
	b.ConnectionRevision = resp.ConnectionRevision
	b.Revision = bindingRevision(t, generation, resp.ConnectionRevision) + ":" + b.SecretID
	if err = s.store.saveWebhookBinding(ctx, b); err != nil {
		if b.SecretID != oldSecret {
			_ = s.pluginAutomation.DeleteAutomationSecret(ctx, b.SecretID)
		}
		return nil, err
	}
	return b, nil
}

func (s *Service) handlePluginWebhook(c *gin.Context) {
	defer func() { webhookCounters.Add(fmt.Sprintf("http_%d", c.Writer.Status()), 1) }()
	if c.GetHeader("Content-Encoding") != "" {
		c.Status(http.StatusUnsupportedMediaType)
		return
	}
	b, err := s.store.webhookBinding(c.Request.Context(), c.Param("binding_id"), false)
	if err != nil {
		c.Status(503)
		return
	}
	if b == nil || s.pluginAutomation == nil {
		c.Status(401)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, pluginWebhookLimit+1))
	if err != nil {
		c.Status(400)
		return
	}
	if len(body) > pluginWebhookLimit {
		c.Status(413)
		return
	}
	s.handleBoundPluginWebhook(c, b, body)
}
func (s *Service) handleBoundPluginWebhook(c *gin.Context, b *WebhookBinding, body []byte) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	t, err := s.store.GetTrigger(ctx, b.TriggerID)
	if err != nil {
		c.Status(503)
		return
	}
	if t == nil || !t.Enabled || t.Type != TriggerTypePluginEvent {
		c.Status(401)
		return
	}
	a, err := s.store.GetAutomation(ctx, b.AutomationID)
	if err != nil {
		c.Status(503)
		return
	}
	if a == nil || !a.Enabled {
		c.Status(401)
		return
	}
	cfg, err := parsePluginEventConfig(t.Config)
	if err != nil {
		c.Status(401)
		return
	}
	declaration, generation, adapter, release, err := s.pluginAutomation.AcquireAutomationAdapter(cfg.PluginID, cfg.ConditionKey)
	if err != nil {
		c.Status(503)
		return
	}
	defer release()
	if cfg.ConfigVersion != declaration.ConfigVersion {
		c.Status(401)
		return
	}
	if b.Revision != bindingRevision(t, generation, b.ConnectionRevision)+":"+b.SecretID {
		c.Status(401)
		return
	}
	s.verifyAndPersistWebhook(c, ctx, b, t, a, cfg, declaration, adapter, body)
}
func (s *Service) verifyAndPersistWebhook(c *gin.Context, ctx context.Context, b *WebhookBinding, t *AutomationTrigger, a *Automation, cfg PluginEventConfig, declaration manifest.AutomationCondition, adapter pluginsdk.AutomationAdapter, body []byte) {
	secret, err := s.pluginAutomation.ReadAutomationSecret(ctx, b.SecretID)
	if err != nil {
		c.Status(503)
		return
	}
	headers := map[string]string{}
	for name, values := range c.Request.Header {
		key := strings.ToLower(name)
		if slices.Contains(declaration.VerificationHeaders, key) {
			if len(values) != 1 {
				c.Status(400)
				return
			}
			headers[key] = values[0]
		}
	}
	resp, err := adapter.VerifyAutomationWebhook(ctx, &pluginsdk.AutomationWebhookRequest{WorkspaceId: b.WorkspaceID, ConditionKey: cfg.ConditionKey, ConnectionId: b.ConnectionID, ConnectionRevision: b.ConnectionRevision, Config: cfg.Settings, Body: body, Headers: headers, Secret: secret})
	if err != nil || resp == nil {
		c.Status(503)
		return
	}
	if resp.Outcome == "malformed" {
		c.Status(400)
		return
	}
	if resp.Outcome == "rejected" {
		c.Status(401)
		return
	}
	if resp.Outcome != webhookAccepted && resp.Outcome != webhookIgnored {
		c.Status(503)
		return
	}
	if resp.Outcome == webhookAccepted && (resp.EventKind != cfg.ConditionKey || !json.Valid(resp.Data) || len(resp.Data) > pluginWebhookLimit) {
		c.Status(503)
		return
	}
	s.persistVerifiedWebhook(c, ctx, b, t, a, cfg, resp, body)
}
func (s *Service) persistVerifiedWebhook(c *gin.Context, ctx context.Context, b *WebhookBinding, t *AutomationTrigger, a *Automation, cfg PluginEventConfig, resp *pluginsdk.AutomationWebhookResponse, body []byte) {
	var err error
	var payload []byte
	if resp.Outcome == webhookAccepted {
		var original, normalized map[string]any
		if json.Unmarshal(body, &original) != nil || original == nil || json.Unmarshal(resp.Data, &normalized) != nil || normalized == nil {
			c.Status(400)
			return
		}
		payload, err = json.Marshal(map[string]json.RawMessage{"webhook": body, "data": resp.Data})
		if err != nil {
			c.Status(503)
			return
		}
	}
	s.commitWebhookReceipt(c, ctx, b, t, a, resp, payload, body)
}
func (s *Service) commitWebhookReceipt(c *gin.Context, ctx context.Context, b *WebhookBinding, t *AutomationTrigger, a *Automation, resp *pluginsdk.AutomationWebhookResponse, payload, body []byte) {
	// Original bytes provide deduplication even when the provider's request ID is unsigned.
	digest := sha256.Sum256(body)
	receipt := &WebhookReceipt{ID: uuid.NewString(), BindingID: b.ID, AutomationID: b.AutomationID, Revision: b.Revision, Identity: hex.EncodeToString(digest[:]), State: "pending", Payload: string(payload), CreatedAt: time.Now().Unix()}
	if resp.Outcome == webhookIgnored {
		receipt.State = webhookIgnored
		receipt.FinishedAt = receipt.CreatedAt
		receipt.Payload = "{}"
	}
	unlock := s.automationRunLock(a.ID)
	defer unlock()
	current, err := s.store.webhookBinding(ctx, b.ID, false)
	if err != nil {
		c.Status(503)
		return
	}
	if current == nil || current.Revision != b.Revision {
		c.Status(401)
		return
	}
	valid, err := s.currentWebhookAuthority(ctx, b, t)
	if err != nil {
		c.Status(503)
		return
	}
	if !valid {
		c.Status(401)
		return
	}
	result, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`INSERT INTO automation_webhook_receipts
 (id,binding_id,automation_id,revision,identity,state,payload,created_at,finished_at) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(binding_id,identity) DO NOTHING`), receipt.ID, receipt.BindingID, receipt.AutomationID, receipt.Revision, receipt.Identity, receipt.State, receipt.Payload, receipt.CreatedAt, receipt.FinishedAt)
	if err != nil {
		c.Status(503)
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		c.Status(503)
		return
	}
	if n == 0 {
		webhookCounters.Add("duplicate", 1)
		c.JSON(200, gin.H{"status": "duplicate"})
		return
	}
	if receipt.State == webhookIgnored {
		webhookCounters.Add(webhookIgnored, 1)
		c.JSON(200, gin.H{"status": webhookIgnored})
		return
	}
	webhookCounters.Add(webhookAccepted, 1)
	c.JSON(202, gin.H{"status": webhookAccepted, "receipt_id": receipt.ID})
}

func (s *Service) pluginBindingAction(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		AutomationID string `json:"automation_id"`
		TriggerID    string `json:"trigger_id"`
		Operation    string `json:"operation"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request", nil)
	}
	a, err := s.GetAutomation(ctx, req.AutomationID)
	if err != nil || a == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "automation unavailable", nil)
	}
	if req.Operation == "receipts" {
		rows, err := s.store.webhookReceipts(ctx, a.ID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "receipt lookup failed", nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, rows)
	}
	t, err := s.GetTrigger(ctx, req.TriggerID)
	if err != nil || t == nil || t.AutomationID != a.ID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "condition unavailable", nil)
	}
	var b *WebhookBinding
	switch req.Operation {
	case "configure", "rotate":
		b, err = s.configureWebhookBinding(ctx, t.ID, req.Operation == "rotate")
	case "get", "reveal", "delete":
		b, err = s.store.webhookBinding(ctx, t.ID, true)
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "unknown binding operation", nil)
	}
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	if b == nil {
		return ws.NewResponse(msg.ID, msg.Action, nil)
	}
	return s.webhookBindingResponse(ctx, msg, req.Operation, a, t, b)
}
func (s *Service) webhookBindingResponse(ctx context.Context, msg *ws.Message, operation string, a *Automation, t *AutomationTrigger, b *WebhookBinding) (*ws.Message, error) {
	var err error
	if operation == "delete" {
		unlock := s.automationRunLock(a.ID)
		defer unlock()
		if err = s.cancelTriggerWebhookReceipts(ctx, t.ID, "binding revoked"); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "binding cancellation failed", nil)
		}
		if err = s.store.enqueueWebhookSecrets(ctx, "trigger_id", t.ID); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "binding deletion failed", nil)
		}
		_, err = s.store.db.ExecContext(ctx, s.store.db.Rebind(`DELETE FROM automation_webhook_bindings WHERE id=?`), b.ID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "binding deletion failed", nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, nil)
	}
	response := map[string]any{"binding": b, "path": "/api/v1/automations/webhook-bindings/" + b.ID}
	if operation == "reveal" {
		if s.pluginAutomation == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "vault unavailable", nil)
		}
		secret, revealErr := s.pluginAutomation.ReadAutomationSecret(ctx, b.SecretID)
		if revealErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "secret unavailable", nil)
		}
		response["secret"] = secret
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

func (s *Service) validatePluginTrigger(ctx context.Context, workspaceID string, kind TriggerType, raw json.RawMessage, enabled bool) error {
	if kind != TriggerTypePluginEvent {
		return nil
	}
	cfg, err := parsePluginEventConfig(raw)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	if s.pluginAutomation == nil {
		return fmt.Errorf("plugin adapters unavailable")
	}
	declaration, _, adapter, release, err := s.pluginAutomation.AcquireAutomationAdapter(cfg.PluginID, cfg.ConditionKey)
	if err != nil {
		return err
	}
	defer release()
	if cfg.ConfigVersion != declaration.ConfigVersion {
		return fmt.Errorf("condition config version unavailable")
	}
	if err = validateConditionSettings(declaration, cfg.Settings); err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := adapter.DescribeAutomationCondition(callCtx, &pluginsdk.AutomationConditionRequest{WorkspaceId: workspaceID, ConditionKey: cfg.ConditionKey, Config: cfg.Settings})
	if err != nil {
		return err
	}
	if info == nil || !info.Available {
		return fmt.Errorf("condition configuration or connection unavailable")
	}
	return nil
}

// CancelPluginWebhookDeliveries is invoked under the plugin lifecycle write lease.
func (s *Service) CancelPluginWebhookDeliveries(pluginID string) error {
	ctx := context.Background()
	triggers := []AutomationTrigger{}
	if err := s.store.db.SelectContext(ctx, &triggers, s.store.db.Rebind(`SELECT * FROM automation_triggers WHERE type=?`), TriggerTypePluginEvent); err != nil {
		return err
	}
	hydrateTriggers(triggers)
	for _, t := range triggers {
		cfg, err := parsePluginEventConfig(t.Config)
		if err != nil || cfg.PluginID != pluginID {
			continue
		}
		unlock := s.automationRunLock(t.AutomationID)
		rows := []WebhookReceipt{}
		err = s.store.db.SelectContext(ctx, &rows, s.store.db.Rebind(`SELECT r.* FROM automation_webhook_receipts r JOIN automation_webhook_bindings b ON b.id=r.binding_id WHERE b.trigger_id=? AND r.state IN ('pending','dispatch')`), t.ID)
		if err == nil {
			for i := range rows {
				if err = s.finishWebhookReceipt(ctx, &rows[i], "cancelled", "plugin lifecycle changed"); err != nil {
					break
				}
			}
		}
		unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func propIsString(value any) bool { prop, _ := value.(map[string]any); return prop["type"] == "string" }

// Called under the automation lock, after verification and before durable admission.
func (s *Service) currentWebhookAuthority(ctx context.Context, b *WebhookBinding, previous *AutomationTrigger) (bool, error) {
	t, err := s.store.GetTrigger(ctx, b.TriggerID)
	if err != nil {
		return false, err
	}
	if t == nil || !t.Enabled || t.Type != TriggerTypePluginEvent || !t.UpdatedAt.Equal(previous.UpdatedAt) || string(t.Config) != string(previous.Config) {
		return false, nil
	}
	a, err := s.store.GetAutomation(ctx, b.AutomationID)
	return a != nil && a.Enabled, err
}

// Caller holds the automation lock; cancellation survives an immediate re-enable.
func (s *Service) cancelAutomationWebhookReceipts(ctx context.Context, id string) error {
	rows := []WebhookReceipt{}
	if err := s.store.db.SelectContext(ctx, &rows, s.store.db.Rebind(`SELECT * FROM automation_webhook_receipts WHERE automation_id=? AND state IN ('pending','dispatch')`), id); err != nil {
		return err
	}
	for i := range rows {
		if err := s.finishWebhookReceipt(ctx, &rows[i], "cancelled", "automation disabled"); err != nil {
			return err
		}
	}
	return nil
}

func validWebhookDescription(resp *pluginsdk.AutomationConditionResponse) bool {
	return resp != nil && resp.Available && resp.ConnectionId != "" && resp.ConnectionRevision != ""
}

// Caller holds the automation lock; settle admitted runs before cascading receipts.
func (s *Service) cancelTriggerWebhookReceipts(ctx context.Context, triggerID, reason string) error {
	rows := []WebhookReceipt{}
	if err := s.store.db.SelectContext(ctx, &rows, s.store.db.Rebind(`SELECT r.* FROM automation_webhook_receipts r JOIN automation_webhook_bindings b ON b.id=r.binding_id WHERE b.trigger_id=? AND r.state IN ('pending','dispatch')`), triggerID); err != nil {
		return err
	}
	for i := range rows {
		if err := s.finishWebhookReceipt(ctx, &rows[i], "cancelled", reason); err != nil {
			return err
		}
	}
	return nil
}
