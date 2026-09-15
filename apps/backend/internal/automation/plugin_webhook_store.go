package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/secrets"
)

const pluginWebhookTablesSQL = `
 CREATE TABLE IF NOT EXISTS automation_webhook_secret_cleanup (secret_id TEXT PRIMARY KEY);
 CREATE TABLE IF NOT EXISTS automation_webhook_bindings (
 id TEXT PRIMARY KEY, trigger_id TEXT NOT NULL UNIQUE REFERENCES automation_triggers(id) ON DELETE CASCADE,
 automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE, workspace_id TEXT NOT NULL,
 revision TEXT NOT NULL, connection_id TEXT NOT NULL, connection_revision TEXT NOT NULL, secret_id TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS automation_webhook_receipts (
 id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES automation_webhook_bindings(id) ON DELETE CASCADE,
 automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
 revision TEXT NOT NULL, identity TEXT NOT NULL, state TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
 payload TEXT NOT NULL DEFAULT '{}', run_id TEXT NOT NULL DEFAULT '', created_at BIGINT NOT NULL, finished_at BIGINT NOT NULL DEFAULT 0,
 UNIQUE(binding_id, identity));
 CREATE INDEX IF NOT EXISTS automation_webhook_receipts_pending ON automation_webhook_receipts(state,created_at);
`

func (s *Store) webhookBinding(ctx context.Context, id string, byTrigger bool) (*WebhookBinding, error) {
	column := "id"
	if byTrigger {
		column = "trigger_id"
	}
	b := &WebhookBinding{}
	err := s.db.GetContext(ctx, b, s.db.Rebind("SELECT * FROM automation_webhook_bindings WHERE "+column+" = ?"), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}
func (s *Store) saveWebhookBinding(ctx context.Context, b *WebhookBinding) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO automation_webhook_bindings
 (id,trigger_id,automation_id,workspace_id,revision,connection_id,connection_revision,secret_id) VALUES (?,?,?,?,?,?,?,?)
 ON CONFLICT(trigger_id) DO UPDATE SET revision=excluded.revision,connection_id=excluded.connection_id,
 connection_revision=excluded.connection_revision,secret_id=excluded.secret_id`), b.ID, b.TriggerID, b.AutomationID, b.WorkspaceID, b.Revision, b.ConnectionID, b.ConnectionRevision, b.SecretID)
	return err
}
func (s *Store) webhookReceipts(ctx context.Context, automationID string) ([]WebhookReceipt, error) {
	rows := []WebhookReceipt{}
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`SELECT r.*, COALESCE(a.task_id,'') AS linked_task_id FROM automation_webhook_receipts r LEFT JOIN automation_runs a ON a.id=r.run_id WHERE r.automation_id=? ORDER BY r.created_at DESC LIMIT 50`), automationID)
	return rows, err
}

func (s *Store) enqueueWebhookSecrets(ctx context.Context, column, id string) error {
	// Both callers supply a fixed column, never request input.
	_, err := s.db.ExecContext(ctx, s.db.Rebind("INSERT INTO automation_webhook_secret_cleanup (secret_id) SELECT secret_id FROM automation_webhook_bindings WHERE "+column+"=? ON CONFLICT(secret_id) DO NOTHING"), id)
	return err
}
func (s *Service) cleanupWebhookSecrets(ctx context.Context) error {
	if s.pluginAutomation == nil {
		return nil
	}
	ids := []string{}
	if err := s.store.db.SelectContext(ctx, &ids, `SELECT secret_id FROM automation_webhook_secret_cleanup WHERE secret_id NOT IN (SELECT secret_id FROM automation_webhook_bindings) LIMIT 100`); err != nil {
		return err
	}
	var cleanupErr error
	for _, id := range ids {
		if err := s.pluginAutomation.DeleteAutomationSecret(ctx, id); err != nil {
			if !errors.Is(err, secrets.ErrNotFound) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete webhook secret %q: %w", id, err))
				continue
			}
		}
		if _, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`DELETE FROM automation_webhook_secret_cleanup WHERE secret_id=?`), id); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete webhook secret cleanup row %q: %w", id, err))
		}
	}
	return cleanupErr
}
