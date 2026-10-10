package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrSourceWritebackConflict = errors.New("plugin source writeback receipt conflict")

// SourceWritebackIntent is the immutable identity and payload fence for one
// provider write. The payload itself is deliberately not persisted.
type SourceWritebackIntent struct {
	OperationID           string `db:"operation_id"`
	InstallationID        string `db:"installation_id"`
	WorkspaceID           string `db:"workspace_id"`
	TaskID                string `db:"task_id"`
	Provider              string `db:"provider"`
	SourceID              string `db:"source_id"`
	Operation             string `db:"operation"`
	ExpectedSourceVersion string `db:"expected_source_version"`
	PayloadDigest         string `db:"payload_digest"`
}

// SourceWritebackReceipt records the provider effect lifecycle and its
// host-owned source subject. A sending receipt without a final result is
// treated as uncertain after restart and is never sent again automatically.
type SourceWritebackReceipt struct {
	SourceWritebackIntent
	ReceiptID         string `db:"receipt_id"`
	State             string `db:"state"`
	ResultStatus      string `db:"result_status"`
	Reason            string `db:"reason"`
	ProviderReceiptID string `db:"provider_receipt_id"`
	CreatedAt         string `db:"created_at"`
	UpdatedAt         string `db:"updated_at"`
}

func (s *CommandStore) initSourceWritebackSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_host_source_writeback_receipts (
			operation_id TEXT PRIMARY KEY,
			receipt_id TEXT NOT NULL UNIQUE,
			installation_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			source_id TEXT NOT NULL,
			operation TEXT NOT NULL,
			expected_source_version TEXT NOT NULL,
			payload_digest TEXT NOT NULL,
			state TEXT NOT NULL,
			result_status TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			provider_receipt_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	return err
}

// PrepareSourceWriteback persists the provider subject before a write can be
// sent. A retry may only reuse the exact immutable identity and payload.
func (s *CommandStore) PrepareSourceWriteback(ctx context.Context, input SourceWritebackIntent) (SourceWritebackReceipt, bool, error) {
	if err := validateSourceWritebackIntent(input); err != nil {
		return SourceWritebackReceipt{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	receiptID := uuid.NewString()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO plugin_host_source_writeback_receipts (
			operation_id, receipt_id, installation_id, workspace_id, task_id, provider, source_id,
			operation, expected_source_version, payload_digest, state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'prepared', ?, ?)
		ON CONFLICT(operation_id) DO NOTHING
	`), input.OperationID, receiptID, input.InstallationID, input.WorkspaceID, input.TaskID, input.Provider,
		input.SourceID, input.Operation, input.ExpectedSourceVersion, input.PayloadDigest, now, now)
	if err != nil {
		return SourceWritebackReceipt{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return SourceWritebackReceipt{}, false, err
	}
	receipt, err := s.GetSourceWriteback(ctx, input.OperationID)
	if err != nil {
		return SourceWritebackReceipt{}, false, err
	}
	if !sameSourceWritebackIntent(receipt.SourceWritebackIntent, input) {
		return SourceWritebackReceipt{}, false, ErrSourceWritebackConflict
	}
	return receipt, affected == 0, nil
}

// StartSourceWriteback atomically reserves the only provider-send attempt.
// Existing sending or terminal receipts are returned without authorizing a
// second send.
func (s *CommandStore) StartSourceWriteback(ctx context.Context, operationID string) (SourceWritebackReceipt, bool, error) {
	if strings.TrimSpace(operationID) == "" {
		return SourceWritebackReceipt{}, false, errors.New("plugin source writeback: operation id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_host_source_writeback_receipts
		SET state = 'sending', updated_at = ?
		WHERE operation_id = ? AND state = 'prepared'
	`), now, operationID)
	if err != nil {
		return SourceWritebackReceipt{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return SourceWritebackReceipt{}, false, err
	}
	receipt, err := s.GetSourceWriteback(ctx, operationID)
	return receipt, affected == 1, err
}

// FinishSourceWriteback stores a final or uncertain provider result. Repeating
// the same result is safe; a different result cannot overwrite the receipt.
func (s *CommandStore) FinishSourceWriteback(ctx context.Context, operationID, resultStatus, reason, providerReceiptID string) (SourceWritebackReceipt, error) {
	if strings.TrimSpace(operationID) == "" || strings.TrimSpace(resultStatus) == "" {
		return SourceWritebackReceipt{}, errors.New("plugin source writeback: operation id and result status are required")
	}
	state := commandReceiptStateCompleted
	if resultStatus == "UNCERTAIN" {
		state = "uncertain"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_host_source_writeback_receipts
		SET state = ?, result_status = ?, reason = ?, provider_receipt_id = ?, updated_at = ?
		WHERE operation_id = ? AND state IN ('prepared', 'sending')
	`), state, resultStatus, reason, providerReceiptID, now, operationID)
	if err != nil {
		return SourceWritebackReceipt{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return SourceWritebackReceipt{}, err
	}
	receipt, err := s.GetSourceWriteback(ctx, operationID)
	if err != nil {
		return SourceWritebackReceipt{}, err
	}
	if affected == 0 && (receipt.State != state || receipt.ResultStatus != resultStatus || receipt.Reason != reason || receipt.ProviderReceiptID != providerReceiptID) {
		return SourceWritebackReceipt{}, ErrSourceWritebackConflict
	}
	return receipt, nil
}

// GetSourceWriteback returns one durable source issue receipt.
func (s *CommandStore) GetSourceWriteback(ctx context.Context, operationID string) (SourceWritebackReceipt, error) {
	if strings.TrimSpace(operationID) == "" {
		return SourceWritebackReceipt{}, errors.New("plugin source writeback: operation id is required")
	}
	var receipt SourceWritebackReceipt
	err := s.ro.GetContext(ctx, &receipt, s.ro.Rebind(`
		SELECT operation_id, installation_id, workspace_id, task_id, provider, source_id,
		receipt_id, operation, expected_source_version, payload_digest, state, result_status,
			reason, provider_receipt_id, created_at, updated_at
		FROM plugin_host_source_writeback_receipts WHERE operation_id = ?
	`), operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceWritebackReceipt{}, sql.ErrNoRows
	}
	return receipt, err
}

func validateSourceWritebackIntent(input SourceWritebackIntent) error {
	for name, value := range map[string]string{
		"operation_id": input.OperationID, "installation_id": input.InstallationID,
		"workspace_id": input.WorkspaceID, "task_id": input.TaskID, "provider": input.Provider,
		"source_id": input.SourceID, "operation": input.Operation,
		"expected_source_version": input.ExpectedSourceVersion, "payload_digest": input.PayloadDigest,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("plugin source writeback: invalid %s", name)
		}
	}
	if input.Provider != "jira" && input.Provider != "linear" {
		return errors.New("plugin source writeback: unsupported provider")
	}
	if input.Operation != "comment" && input.Operation != "transition" {
		return errors.New("plugin source writeback: unsupported operation")
	}
	return nil
}

func sameSourceWritebackIntent(a, b SourceWritebackIntent) bool {
	return a.OperationID == b.OperationID && a.InstallationID == b.InstallationID &&
		a.WorkspaceID == b.WorkspaceID && a.TaskID == b.TaskID && a.Provider == b.Provider &&
		a.SourceID == b.SourceID && a.Operation == b.Operation &&
		a.ExpectedSourceVersion == b.ExpectedSourceVersion && a.PayloadDigest == b.PayloadDigest
}
