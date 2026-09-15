package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/pkg/pluginsdk"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPluginWebhookInstallationRequiresRebinding(t *testing.T) {
	s, a, b, router := webhookTestSetup(t)
	ctx := context.Background()
	oldSecret := b.SecretID
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{"old":1}`, "valid").Code)
	require.NoError(t, s.CancelPluginWebhookDeliveries("plugin"))
	a.generation = "v2:new-installation"
	require.Equal(t, 401, sendPluginWebhook(router, b.ID, `{"new":1}`, "valid").Code)
	b, err := s.configureWebhookBinding(ctx, b.TriggerID, false)
	require.NoError(t, err)
	require.NotEqual(t, oldSecret, b.SecretID)
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{"new":1}`, "valid").Code)
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
}

func TestPluginWebhookDeletionSettlesUnclaimedRun(t *testing.T) {
	for _, operation := range []string{"trigger", "binding"} {
		t.Run(operation, func(t *testing.T) {
			s, _, b, router := webhookTestSetup(t)
			ctx := context.Background()
			require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
			require.NoError(t, s.ProcessWebhookReceipts(ctx))
			rows, err := s.store.webhookReceipts(ctx, b.AutomationID)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			if operation == "trigger" {
				require.NoError(t, s.DeleteTrigger(ctx, b.TriggerID))
			} else {
				msg, err := ws.NewRequest("1", "automation.webhook_binding", map[string]string{"automation_id": b.AutomationID, "trigger_id": b.TriggerID, "operation": "delete"})
				require.NoError(t, err)
				response, err := s.pluginBindingAction(ctx, msg)
				require.NoError(t, err)
				require.Equal(t, ws.MessageTypeResponse, response.Type)
			}
			run, err := s.store.GetRun(ctx, rows[0].RunID)
			require.NoError(t, err)
			require.Equal(t, RunStatusFailed, run.Status)
			count, err := s.store.CountActiveRuns(ctx, b.AutomationID)
			require.NoError(t, err)
			require.Zero(t, count)
			won, err := s.ClaimPluginWebhookRun(ctx, run.ID)
			require.NoError(t, err)
			require.False(t, won)
		})
	}
}

type failingDescriptionProvider struct {
	*testAutomationAdapter
	calls int
}

func (a *failingDescriptionProvider) AcquireAutomationAdapter(string, string) (manifest.AutomationCondition, string, pluginsdk.AutomationAdapter, func(), error) {
	return manifest.AutomationCondition{}, a.generation, a, func() {}, nil
}
func (a *failingDescriptionProvider) DescribeAutomationCondition(context.Context, *pluginsdk.AutomationConditionRequest) (*pluginsdk.AutomationConditionResponse, error) {
	a.calls++
	return nil, fmt.Errorf("describe unavailable")
}

func TestPluginWebhookRetriesBackOffAndDoNotStarveNewReceipts(t *testing.T) {
	s, a, b, router := webhookTestSetup(t)
	ctx := context.Background()
	for i := 0; i < 101; i++ {
		require.Equal(t, 202, sendPluginWebhook(router, b.ID, fmt.Sprintf(`{"event":%d}`, i), "valid").Code)
	}
	p := &failingDescriptionProvider{testAutomationAdapter: a}
	s.SetPluginAutomationProvider(p)
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	require.Equal(t, 100, p.calls)
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	require.Equal(t, 101, p.calls)
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	require.Equal(t, 101, p.calls)
	var delayed int
	require.NoError(t, s.store.db.Get(&delayed, `SELECT COUNT(*) FROM automation_webhook_receipts WHERE attempt_count=1 AND next_attempt_at>?`, time.Now().Unix()))
	require.Equal(t, 101, delayed)
	// Reopening the store preserves the persisted retry budget and delay.
	reopened, err := NewStore(s.store.db, s.store.db)
	require.NoError(t, err)
	s.store = reopened
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	require.Equal(t, 101, p.calls)
}

func TestPluginWebhookUnclaimedDispatchHasBoundedAttempts(t *testing.T) {
	s, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	for i := 0; i < webhookMaxAttempts+1; i++ {
		_, err := s.store.db.Exec(`UPDATE automation_webhook_receipts SET next_attempt_at=0`)
		require.NoError(t, err)
		require.NoError(t, s.ProcessWebhookReceipts(ctx))
	}
	rows, err := s.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "failed", rows[0].State)
	require.Equal(t, webhookMaxAttempts, rows[0].AttemptCount)
	count, err := s.store.CountActiveRuns(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestPluginWebhookCleanupMissingSecretDoesNotBlockReceipts(t *testing.T) {
	s, adapter, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	_, err := s.store.db.Exec(`INSERT INTO automation_webhook_secret_cleanup (secret_id) VALUES (?)`, "automation-webhook:already-deleted")
	require.NoError(t, err)
	adapter.deleteErr = secrets.ErrNotFound

	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	rows, err := s.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "dispatch", rows[0].State)
	var cleanupRows int
	require.NoError(t, s.store.db.Get(&cleanupRows, `SELECT COUNT(*) FROM automation_webhook_secret_cleanup`))
	require.Zero(t, cleanupRows)
	require.Equal(t, 1, adapter.deleteCalls)
}

func TestPluginWebhookCleanupFailureDoesNotBlockReceipts(t *testing.T) {
	s, adapter, b, router := webhookTestSetup(t)
	ctx := context.Background()
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	_, err := s.store.db.Exec(`INSERT INTO automation_webhook_secret_cleanup (secret_id) VALUES (?)`, "automation-webhook:retry-later")
	require.NoError(t, err)
	adapter.deleteErr = errors.New("vault unavailable")

	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	rows, err := s.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "dispatch", rows[0].State)
	var cleanupRows int
	require.NoError(t, s.store.db.Get(&cleanupRows, `SELECT COUNT(*) FROM automation_webhook_secret_cleanup`))
	require.Equal(t, 1, cleanupRows)
}

func TestPluginWebhookManualRunDoesNotInvalidateBinding(t *testing.T) {
	s, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	_, err := s.store.db.Exec(`UPDATE automations SET max_concurrent_runs=2 WHERE id=?`, b.AutomationID)
	require.NoError(t, err)

	result, err := s.FireTrigger(ctx, b.AutomationID, b.TriggerID, TriggerType("manual"), json.RawMessage(`{"source":"manual"}`), "")
	require.NoError(t, err)
	require.False(t, result.Skipped)
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	rows, err := s.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "dispatch", rows[0].State)
}

func TestPluginWebhookConcurrencySkipDoesNotInvalidateBinding(t *testing.T) {
	s, _, b, router := webhookTestSetup(t)
	ctx := context.Background()
	active := &AutomationRun{
		AutomationID: b.AutomationID,
		TriggerID:    b.TriggerID,
		TriggerType:  TriggerTypePluginEvent,
		Status:       RunStatusTaskCreated,
		DedupKey:     "active-run",
		TriggerData:  json.RawMessage(`{}`),
	}
	require.NoError(t, s.store.CreateRun(ctx, active))

	result, err := s.FireTrigger(ctx, b.AutomationID, b.TriggerID, TriggerType("manual"), json.RawMessage(`{"source":"manual"}`), "")
	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, 202, sendPluginWebhook(router, b.ID, `{}`, "valid").Code)
	require.NoError(t, s.ProcessWebhookReceipts(ctx))
	rows, err := s.store.webhookReceipts(ctx, b.AutomationID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "skipped", rows[0].State)
}

func TestPluginWebhookRejectsMixedSchedules(t *testing.T) {
	s, _, b, _ := webhookTestSetup(t)
	ctx := context.Background()
	_, err := s.AddTrigger(ctx, &AddTriggerRequest{AutomationID: b.AutomationID, Type: TriggerTypeScheduled, Enabled: true, Config: json.RawMessage(`{"cron_expression":"* * * * *"}`)})
	require.ErrorContains(t, err, "cannot be combined")
	require.Error(t, validateTriggerCombination([]TriggerType{TriggerTypeScheduled, TriggerTypePluginEvent}))
	require.NoError(t, validateTriggerCombination([]TriggerType{TriggerTypeScheduled, TriggerTypeGitHubPR}))
}
