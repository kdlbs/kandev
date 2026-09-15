package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"go.uber.org/zap"
	"time"
)

var webhookCounters = expvar.NewMap("automation_webhooks")

// ProcessWebhookReceipts retries persisted admissions and unclaimed dispatches.
// The backend has one automation worker; the DB claim also fences duplicate bus delivery.
func (s *Service) ProcessWebhookReceipts(ctx context.Context) error {
	if s.pluginAutomation == nil {
		return nil
	}
	if err := s.cleanupWebhookSecrets(ctx); err != nil {
		return err
	}
	receipts := []WebhookReceipt{}
	if err := s.store.db.SelectContext(ctx, &receipts, `SELECT * FROM automation_webhook_receipts WHERE state IN ('pending','dispatch') ORDER BY created_at LIMIT 100`); err != nil {
		return err
	}
	for i := range receipts {
		if err := s.processWebhookReceipt(ctx, &receipts[i]); err != nil {
			webhookCounters.Add("processing_failures", 1)
			s.logger.Warn("webhook receipt processing failed", zap.String("receipt_id", receipts[i].ID), zap.Error(err))
		}
	}
	_, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`DELETE FROM automation_webhook_receipts WHERE state NOT IN ('pending','dispatch','processing') AND finished_at > 0 AND finished_at < ?`), time.Now().Add(-7*24*time.Hour).Unix())
	return err
}
func (s *Service) processWebhookReceipt(ctx context.Context, r *WebhookReceipt) error {
	b, err := s.store.webhookBinding(ctx, r.BindingID, false)
	if err != nil {
		return err
	}
	if b == nil || b.Revision != r.Revision {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "binding changed")
	}
	t, err := s.store.GetTrigger(ctx, b.TriggerID)
	if err != nil {
		return err
	}
	if t == nil || !t.Enabled {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "condition disabled")
	}
	cfg, err := parsePluginEventConfig(t.Config)
	if err != nil {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "condition changed")
	}
	_, generation, adapter, release, err := s.pluginAutomation.AcquireAutomationAdapter(cfg.PluginID, cfg.ConditionKey)
	if err != nil {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "plugin unavailable")
	}
	defer release()
	if b.Revision != bindingRevision(t, generation, b.ConnectionRevision)+":"+b.SecretID {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "adapter changed")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	description, err := adapter.DescribeAutomationCondition(callCtx, &pluginsdk.AutomationConditionRequest{WorkspaceId: b.WorkspaceID, ConditionKey: cfg.ConditionKey, Config: cfg.Settings})
	if err != nil {
		return err
	}
	if !webhookConnectionMatches(description, b) {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "connection changed")
	}
	return s.processCurrentWebhookReceipt(ctx, r, b, t)
}
func (s *Service) processCurrentWebhookReceipt(ctx context.Context, r *WebhookReceipt, b *WebhookBinding, t *AutomationTrigger) error {
	unlock := s.automationRunLock(b.AutomationID)
	defer unlock()
	current, err := s.store.webhookBinding(ctx, b.ID, false)
	if err != nil {
		return err
	}
	if current == nil || current.Revision != b.Revision {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "binding changed")
	}
	a, err := s.store.GetAutomation(ctx, b.AutomationID)
	if err != nil {
		return err
	}
	if a == nil || !a.Enabled {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "automation disabled")
	}
	valid, err := s.currentWebhookAuthority(ctx, b, t)
	if err != nil {
		return err
	}
	if !valid {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "condition changed")
	}
	if r.State == "pending" {
		if err := s.admitWebhookReceipt(ctx, a, t, r); err != nil {
			return err
		}
	}
	if r.State != "dispatch" {
		return nil
	}
	run, err := s.store.GetRun(ctx, r.RunID)
	if err != nil {
		return err
	}
	if run == nil {
		return s.finishWebhookReceipt(ctx, r, "cancelled", "run history removed")
	}
	evt := &AutomationTriggeredEvent{RunID: r.RunID, AutomationID: a.ID, TriggerID: t.ID, TriggerType: TriggerTypePluginEvent, TriggerData: json.RawMessage(r.Payload), DedupKey: r.ID}
	return s.eventBus.Publish(ctx, events.AutomationTriggered, bus.NewEvent(events.AutomationTriggered, "automation_webhook", evt))
}
func (s *Service) admitWebhookReceipt(ctx context.Context, a *Automation, t *AutomationTrigger, r *WebhookReceipt) error {
	reason, _, full, err := s.automationCapacity(ctx, a)
	if err != nil {
		return err
	}
	if full {
		r.State = "skipped"
		return s.finishWebhookReceipt(ctx, r, "skipped", reason)
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	runID := uuid.NewString()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_webhook_receipts SET state='dispatch',run_id=? WHERE id=? AND state='pending'`), runID, r.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return nil
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automation_runs
 (id,automation_id,trigger_id,trigger_type,status,dedup_key,trigger_data,display_title,created_at) VALUES (?,?,?,?,?,?,?,?,?)`), runID, a.ID, t.ID, TriggerTypePluginEvent, RunStatusTriggered, r.ID, r.Payload, RenderRunDisplayTitle(a, TriggerTypePluginEvent, json.RawMessage(r.Payload)), time.Now().UTC())
	if err != nil {
		return err
	}
	if err = recordWebhookAdmission(ctx, tx, a.ID, t.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	r.RunID = runID
	r.State = "dispatch"
	return nil
}
func (s *Service) finishWebhookReceipt(ctx context.Context, r *WebhookReceipt, state, reason string) error {
	result, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`UPDATE automation_webhook_receipts SET state=?,reason=?,payload='{}',finished_at=? WHERE id=? AND state IN ('pending','dispatch')`), state, reason, time.Now().Unix(), r.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	webhookCounters.Add(state, 1)
	if r.RunID != "" && (state == "cancelled" || state == "failed") {
		err = s.store.MarkRunTerminal(ctx, r.RunID, "", "", RunStatusFailed, reason)
	}
	return err
}

// ClaimPluginWebhookRun is consumed before task creation; a claimed run is never dispatched twice.
func (s *Service) ClaimPluginWebhookRun(ctx context.Context, runID string) (bool, error) {
	r := &WebhookReceipt{}
	err := s.store.db.GetContext(ctx, r, s.store.db.Rebind(`SELECT * FROM automation_webhook_receipts WHERE run_id=? AND state='dispatch' AND EXISTS (SELECT 1 FROM automation_runs WHERE id=?)`), runID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	b, err := s.store.webhookBinding(ctx, r.BindingID, false)
	if err != nil {
		return false, err
	}
	if b == nil {
		return false, nil
	}
	t, err := s.store.GetTrigger(ctx, b.TriggerID)
	if err != nil {
		return false, err
	}
	if t == nil {
		return false, nil
	}
	return s.claimCurrentWebhookRun(ctx, r, b, t)
}
func (s *Service) claimCurrentWebhookRun(ctx context.Context, r *WebhookReceipt, b *WebhookBinding, t *AutomationTrigger) (bool, error) {
	runID := r.RunID
	cfg, err := parsePluginEventConfig(t.Config)
	if err != nil {
		return false, err
	}
	_, generation, adapter, release, err := s.pluginAutomation.AcquireAutomationAdapter(cfg.PluginID, cfg.ConditionKey)
	if err != nil {
		return false, err
	}
	defer release()
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := adapter.DescribeAutomationCondition(callCtx, &pluginsdk.AutomationConditionRequest{WorkspaceId: b.WorkspaceID, ConditionKey: cfg.ConditionKey, Config: cfg.Settings})
	if err != nil {
		return false, err
	}
	unlock := s.automationRunLock(b.AutomationID)
	defer unlock()
	current, err := s.store.webhookBinding(ctx, b.ID, false)
	if err != nil {
		return false, err
	}
	valid, err := s.currentWebhookAuthority(ctx, b, t)
	if err != nil {
		return false, err
	}
	if !valid || current == nil || current.Revision != r.Revision || b.Revision != bindingRevision(t, generation, b.ConnectionRevision)+":"+b.SecretID || !webhookConnectionMatches(info, b) {
		return false, s.finishWebhookReceipt(ctx, r, "cancelled", "authority changed")
	}
	result, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`UPDATE automation_webhook_receipts SET state='processing' WHERE run_id=? AND state='dispatch' AND EXISTS (SELECT 1 FROM automation_runs WHERE id=?)`), runID, runID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (s *Service) CompletePluginWebhookRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("webhook run disappeared")
	}
	state := "admitted"
	if run.Status == RunStatusFailed || run.TaskID == "" {
		state = "failed"
	}
	result, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`UPDATE automation_webhook_receipts SET state=?,payload='{}',finished_at=? WHERE run_id=? AND state='processing'`), state, time.Now().Unix(), runID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n > 0 {
		webhookCounters.Add(state, n)
	}
	return err
}
func (s *Service) recoverWebhookClaims(ctx context.Context) error {
	webhookCounters.Add("recovery_passes", 1)
	// A crash after a durable dispatch claim has an indeterminate downstream outcome.
	// Mark the receipt failed and expose the existing run; never repeat task creation.
	_, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`UPDATE automation_webhook_receipts SET state='failed',reason='host stopped during dispatch',payload='{}',finished_at=? WHERE state='processing'`), time.Now().Unix())
	return err
}
func (s *Service) hasPendingWebhookDispatch(ctx context.Context, runID string) (bool, error) {
	var count int
	err := s.store.db.GetContext(ctx, &count, s.store.db.Rebind(`SELECT COUNT(*) FROM automation_webhook_receipts WHERE run_id=? AND state='dispatch' AND EXISTS (SELECT 1 FROM automation_runs WHERE id=?)`), runID, runID)
	return count > 0, err
}
func (s *Service) runWebhookWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ProcessWebhookReceipts(ctx); err != nil {
				s.logger.Warn("webhook worker failed", zap.Error(err))
			}
		}
	}
}

func webhookConnectionMatches(info *pluginsdk.AutomationConditionResponse, b *WebhookBinding) bool {
	return info != nil && info.Available && info.ConnectionId == b.ConnectionID && info.ConnectionRevision == b.ConnectionRevision
}

func recordWebhookAdmission(ctx context.Context, tx *sqlx.Tx, automationID, triggerID string) error {
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET last_triggered_at=? WHERE id=?`), now, automationID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_triggers SET last_evaluated_at=? WHERE id=?`), now, triggerID)
	return err
}
