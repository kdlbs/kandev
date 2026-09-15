package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testAutomationAdapter struct {
	onVerify   func()
	secret     string
	generation string
	connection string
	seen       *pluginsdk.AutomationWebhookRequest
}

func (a *testAutomationAdapter) AutomationConditions(context.Context, string) []PluginConditionInfo {
	return nil
}
func (a *testAutomationAdapter) AcquireAutomationAdapter(string, string) (manifest.AutomationCondition, string, pluginsdk.AutomationAdapter, func(), error) {
	return manifest.AutomationCondition{Key: "push", ConfigVersion: 1, VerificationHeaders: []string{"x-signature"}, ConfigSchema: map[string]any{"type": "object"}}, a.generation, a, func() {}, nil
}
func (a *testAutomationAdapter) SetAutomationSecret(_ context.Context, _, value string) error {
	a.secret = value
	return nil
}
func (a *testAutomationAdapter) ReadAutomationSecret(context.Context, string) (string, error) {
	return a.secret, nil
}
func (a *testAutomationAdapter) DeleteAutomationSecret(context.Context, string) error { return nil }
func (a *testAutomationAdapter) DescribeAutomationCondition(context.Context, *pluginsdk.AutomationConditionRequest) (*pluginsdk.AutomationConditionResponse, error) {
	return &pluginsdk.AutomationConditionResponse{Available: true, ConnectionId: "connection", ConnectionRevision: a.connection}, nil
}
func (a *testAutomationAdapter) VerifyAutomationWebhook(_ context.Context, req *pluginsdk.AutomationWebhookRequest) (*pluginsdk.AutomationWebhookResponse, error) {
	a.seen = req
	if a.onVerify != nil {
		a.onVerify()
	}
	if req.Headers["x-signature"] != "valid" {
		return &pluginsdk.AutomationWebhookResponse{Outcome: "rejected"}, nil
	}
	return &pluginsdk.AutomationWebhookResponse{Outcome: "accepted", EventKind: "push", Data: req.Body}, nil
}
func webhookTestSetup(t *testing.T) (*Service, *testAutomationAdapter, *WebhookBinding, *gin.Engine) {
	t.Helper()
	svc := newTestService(t)
	svc.store.db.SetMaxOpenConns(1)
	require.NoError(t, func() error { _, err := svc.store.db.Exec(`PRAGMA foreign_keys=ON`); return err }())
	adapter := &testAutomationAdapter{generation: "v1", connection: "revision-1"}
	svc.SetPluginAutomationProvider(adapter)
	a := &Automation{WorkspaceID: "workspace", Name: "Webhook", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, svc.store.CreateAutomation(context.Background(), a))
	config, _ := json.Marshal(PluginEventConfig{PluginID: "plugin", ConditionKey: "push", ConfigVersion: 1, Settings: json.RawMessage(`{}`)})
	trigger := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypePluginEvent, Config: config, Enabled: true}
	require.NoError(t, svc.store.CreateTrigger(context.Background(), trigger))
	binding, err := svc.configureWebhookBinding(context.Background(), trigger.ID, false)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/hooks/:binding_id", svc.handlePluginWebhook)
	return svc, adapter, binding, router
}
func sendPluginWebhook(router *gin.Engine, id, body, signature string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", "/hooks/"+id, strings.NewReader(body))
	request.Header.Set("X-Signature", signature)
	request.Header.Set("Authorization", "Bearer kandev_pat_secret")
	request.Header.Set("Cookie", "session=secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
func TestPluginWebhookAuthenticationAndDedup(t *testing.T) {
	svc, adapter, binding, router := webhookTestSetup(t)
	require.Equal(t, 401, sendPluginWebhook(router, binding.ID, `{}`, "").Code)
	raw := "{\n \"title\": \"こんにちは\"\n}"
	require.Equal(t, 202, sendPluginWebhook(router, binding.ID, raw, "valid").Code)
	require.Equal(t, []byte(raw), adapter.seen.Body)
	require.Empty(t, adapter.seen.Headers["authorization"])
	require.Empty(t, adapter.seen.Headers["cookie"])
	require.Equal(t, "workspace", adapter.seen.WorkspaceId)
	require.Equal(t, 200, sendPluginWebhook(router, binding.ID, raw, "valid").Code)
	rows, err := svc.store.webhookReceipts(context.Background(), binding.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 413, sendPluginWebhook(router, binding.ID, string(bytes.Repeat([]byte("x"), pluginWebhookLimit+1)), "valid").Code)
}
func TestPluginWebhookAdmissionAndDurableClaim(t *testing.T) {
	svc, _, binding, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, binding.ID, `{"event":1}`, "valid").Code)
	require.NoError(t, svc.ProcessWebhookReceipts(ctx))
	rows, err := svc.store.webhookReceipts(ctx, binding.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "dispatch", rows[0].State)
	require.NotEmpty(t, rows[0].RunID)
	require.NoError(t, svc.ReconcileOpenRuns(ctx))
	run, err := svc.store.GetRun(ctx, rows[0].RunID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTriggered, run.Status)
	won, err := svc.ClaimPluginWebhookRun(ctx, run.ID)
	require.NoError(t, err)
	require.True(t, won)
	won, err = svc.ClaimPluginWebhookRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, won)
	require.NoError(t, svc.recoverWebhookClaims(ctx))
	rows, err = svc.store.webhookReceipts(ctx, binding.AutomationID)
	require.NoError(t, err)
	require.Equal(t, "failed", rows[0].State)
	_, err = svc.store.db.Exec(`DELETE FROM automation_runs`)
	require.NoError(t, err)
	require.Equal(t, 200, sendPluginWebhook(router, binding.ID, `{"event":1}`, "valid").Code)
}
func TestPluginWebhookConnectionRevocation(t *testing.T) {
	svc, adapter, binding, router := webhookTestSetup(t)
	require.Equal(t, 202, sendPluginWebhook(router, binding.ID, `{}`, "valid").Code)
	adapter.connection = "revoked"
	require.NoError(t, svc.ProcessWebhookReceipts(context.Background()))
	rows, err := svc.store.webhookReceipts(context.Background(), binding.AutomationID)
	require.NoError(t, err)
	require.Equal(t, "cancelled", rows[0].State)
	require.Empty(t, rows[0].RunID)
}
func TestPluginWebhookSecretRequiresWorkspaceAuthority(t *testing.T) {
	svc, _, binding, _ := webhookTestSetup(t)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return errors.New("denied") })
	req, err := ws.NewRequest("1", "automation.webhook_binding", map[string]string{"operation": "reveal", "automation_id": binding.AutomationID, "trigger_id": binding.TriggerID})
	require.NoError(t, err)
	response, err := svc.pluginBindingAction(context.Background(), req)
	require.NoError(t, err)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret\":")
}

func TestPluginWebhookDisableCannotResumeOldDelivery(t *testing.T) {
	svc, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	require.NoError(t, svc.DisableAutomation(ctx, b.AutomationID))
	require.NoError(t, svc.EnableAutomation(ctx, b.AutomationID))
	require.NoError(t, svc.ProcessWebhookReceipts(ctx))
	rows, err := svc.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Equal(t, "cancelled", rows[0].State)
	require.Empty(t, rows[0].RunID)
}
func TestPluginWebhookVerificationRacesConditionEdit(t *testing.T) {
	svc, adapter, b, router := webhookTestSetup(t)
	adapter.onVerify = func() {
		enabled := false
		err := svc.UpdateTrigger(context.Background(), b.TriggerID, &UpdateTriggerRequest{Enabled: &enabled})
		require.NoError(t, err)
	}
	require.Equal(t, 401, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	rows, err := svc.store.webhookReceipts(context.Background(), b.AutomationID)
	require.NoError(t, err)
	require.Empty(t, rows)
}
func TestPluginWebhookRotationFencesPublishedRun(t *testing.T) {
	svc, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	require.NoError(t, svc.ProcessWebhookReceipts(ctx))
	rows, err := svc.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	_, err = svc.configureWebhookBinding(ctx, b.TriggerID, true)
	require.NoError(t, err)
	won, err := svc.ClaimPluginWebhookRun(ctx, rows[0].RunID)
	require.NoError(t, err)
	require.False(t, won)
}
func TestPluginWebhookRetentionStartsAtCompletion(t *testing.T) {
	svc, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := svc.store.db.Exec(`UPDATE automation_webhook_receipts SET created_at=?,state='ignored',finished_at=?`, old, time.Now().Unix())
	require.NoError(t, err)
	require.NoError(t, svc.ProcessWebhookReceipts(ctx))
	rows, err := svc.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	_, err = svc.store.db.Exec(`UPDATE automation_webhook_receipts SET finished_at=?`, old)
	require.NoError(t, err)
	require.NoError(t, svc.ProcessWebhookReceipts(ctx))
	rows, err = svc.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Empty(t, rows)
}
func TestPluginWebhookPromptNamespaces(t *testing.T) {
	data := json.RawMessage(`{"webhook":{"repository":{"name":"original"},"trigger":{"type":"spoofed"}},"data":{"repository":"normalized"}}`)
	require.Equal(t, "original normalized plugin_event", InterpolatePrompt("{{webhook.repository.name}} {{data.repository}} {{trigger.type}}", TriggerTypePluginEvent, data))
	require.JSONEq(t, `{"repository":{"name":"original"},"trigger":{"type":"spoofed"}}`, InterpolatePrompt("{{webhook.body}}", TriggerTypePluginEvent, data))
}

func TestPluginWebhookConcurrentConsumersClaimOnlyOnce(t *testing.T) {
	svc, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	require.NoError(t, svc.ProcessWebhookReceipts(ctx))
	rows, err := svc.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	type result struct {
		won bool
		err error
	}
	results := make(chan result, 12)
	for range 12 {
		go func() { won, err := svc.ClaimPluginWebhookRun(ctx, rows[0].RunID); results <- result{won, err} }()
	}
	winners := 0
	for range 12 {
		result := <-results
		require.NoError(t, result.err)
		if result.won {
			winners++
		}
	}
	require.Equal(t, 1, winners)
}

func TestPluginWebhookReceiptSurvivesDatabaseReopen(t *testing.T) {
	svc, adapter, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{"event":"persisted"}`, "valid").Code)
	path := filepath.Join(t.TempDir(), "restarted.db")
	_, err := svc.store.db.Exec(`VACUUM INTO ?`, path)
	require.NoError(t, err)
	db, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	require.NoError(t, err)
	restarted := NewService(store, svc.eventBus, svc.logger)
	restarted.SetPluginAutomationProvider(adapter)
	router = gin.New()
	router.POST("/hooks/:binding_id", restarted.handlePluginWebhook)
	require.Equal(t, 200, sendPluginWebhook(router, b.ID, `{"event":"persisted"}`, "valid").Code)
	require.NoError(t, restarted.ProcessWebhookReceipts(ctx))
	rows, err := store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "dispatch", rows[0].State)
	require.NotEmpty(t, rows[0].RunID)
}
