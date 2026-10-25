package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

var (
	ErrCommandPayloadConflict    = errors.New("plugin command idempotency key payload conflict")
	ErrCommandCompletionConflict = errors.New("plugin command receipt completion conflict")
)

const commandReceiptStateCompleted = "completed"

// CommandIntent is the durable, host-authored admission record for an exact
// plugin command. It remains after the plugin is removed so an old identity
// cannot replay an effect after reinstall.
type CommandIntent struct {
	OperationID             string
	InstallationID          string
	WorkspaceID             string
	RequestID               string
	IdempotencyKey          string
	Method                  string
	CapabilityID            string
	PayloadDigest           string
	TargetID                string
	ExpectedResourceVersion string
	ApprovalRevision        uint64
	ManifestDigest          string
	CreatedAt               string
	UpdatedAt               string
}

// CommandReceipt records the durable result of one exact command intent.
type CommandReceipt struct {
	ID              string
	OperationID     string
	State           string
	ResultStatus    string
	Reason          string
	TargetID        string
	ResourceVersion string
	ResultData      string
	CreatedAt       string
	UpdatedAt       string
}

// CommandRecord joins one intent to its receipt.
type CommandRecord struct {
	Intent  CommandIntent
	Receipt CommandReceipt
}

// CommandStore persists exact Host command intents and receipts in the shared
// application database.
type CommandStore struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewCommandStore opens the durable command intent and receipt tables.
func NewCommandStore(pool *db.Pool) (*CommandStore, error) {
	if pool == nil || pool.Writer() == nil || pool.Reader() == nil {
		return nil, errors.New("plugin command store: database pool is unavailable")
	}
	store := &CommandStore{db: pool.Writer(), ro: pool.Reader()}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("plugin command store schema: %w", err)
	}
	return store, nil
}

func (s *CommandStore) initSchema() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_host_command_intents (
			operation_id TEXT PRIMARY KEY,
			installation_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			request_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			method TEXT NOT NULL,
			capability_id TEXT NOT NULL,
			payload_digest TEXT NOT NULL,
			target_id TEXT NOT NULL,
			expected_resource_version TEXT NOT NULL,
			approval_revision BIGINT NOT NULL,
			manifest_digest TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (installation_id, workspace_id, method, idempotency_key)
		);
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_plugin_host_command_intents_target
			ON plugin_host_command_intents(installation_id, workspace_id, target_id, created_at);
	`); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_host_command_receipts (
			receipt_id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL UNIQUE,
			state TEXT NOT NULL,
			result_status TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL,
			resource_version TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_host_command_result_data (
			operation_id TEXT PRIMARY KEY,
			result_data TEXT NOT NULL
		);
	`); err != nil {
		return err
	}
	if err := s.initSourceWritebackSchema(); err != nil {
		return err
	}
	return s.initTaskDirectiveSchema()
}

// Admit records an intent before the domain service is allowed to perform its
// effect. Exact retries return the original operation and receipt identities.
func (s *CommandStore) Admit(ctx context.Context, input CommandIntent) (CommandRecord, bool, error) {
	if err := validateCommandIntent(input); err != nil {
		return CommandRecord{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	operationID := uuid.NewString()
	receiptID := uuid.NewString()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return CommandRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO plugin_host_command_intents (
			operation_id, installation_id, workspace_id, request_id, idempotency_key,
			method, capability_id, payload_digest, target_id, expected_resource_version,
			approval_revision, manifest_digest, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(installation_id, workspace_id, method, idempotency_key) DO NOTHING
	`), operationID, input.InstallationID, input.WorkspaceID, input.RequestID, input.IdempotencyKey,
		input.Method, input.CapabilityID, input.PayloadDigest, input.TargetID, input.ExpectedResourceVersion,
		input.ApprovalRevision, input.ManifestDigest, now, now)
	if err != nil {
		return CommandRecord{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CommandRecord{}, false, err
	}
	if affected == 0 {
		record, err := getCommandByKey(ctx, tx, input.InstallationID, input.WorkspaceID, input.Method, input.IdempotencyKey)
		if err != nil {
			return CommandRecord{}, false, err
		}
		if record.Intent.PayloadDigest != input.PayloadDigest {
			return CommandRecord{}, false, ErrCommandPayloadConflict
		}
		if err := tx.Commit(); err != nil {
			return CommandRecord{}, false, err
		}
		return record, true, nil
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO plugin_host_command_receipts (
			receipt_id, operation_id, state, target_id, created_at, updated_at
		) VALUES (?, ?, 'accepted', ?, ?, ?)
	`), receiptID, operationID, input.TargetID, now, now)
	if err != nil {
		return CommandRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return CommandRecord{}, false, err
	}
	input.OperationID = operationID
	input.CreatedAt = now
	input.UpdatedAt = now
	return CommandRecord{
		Intent: input,
		Receipt: CommandReceipt{
			ID: receiptID, OperationID: operationID, State: "accepted", TargetID: input.TargetID,
			CreatedAt: now, UpdatedAt: now,
		},
	}, false, nil
}

// Complete stores the command outcome and is safe to repeat with the same
// result after a lost acknowledgement.
func (s *CommandStore) Complete(ctx context.Context, operationID, resultStatus, reason, targetID, resourceVersion string) (CommandRecord, error) {
	return s.CompleteWithResultData(ctx, operationID, resultStatus, reason, targetID, resourceVersion, "")
}

// CompleteWithResultData persists a method-specific replay payload atomically
// with the command outcome.
//
//nolint:cyclop // The command store completes one durable operation and its typed replay result.
func (s *CommandStore) CompleteWithResultData(ctx context.Context, operationID, resultStatus, reason, targetID, resourceVersion, resultData string) (CommandRecord, error) {
	if operationID == "" || resultStatus == "" {
		return CommandRecord{}, errors.New("plugin command store: operation id and result status are required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return CommandRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE plugin_host_command_receipts
		SET state = 'completed', result_status = ?, reason = ?, target_id = ?, resource_version = ?, updated_at = ?
		WHERE operation_id = ? AND state = 'accepted'
	`), resultStatus, reason, targetID, resourceVersion, now, operationID)
	if err != nil {
		return CommandRecord{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CommandRecord{}, err
	}
	if affected == 0 {
		record, getErr := getCommandByOperation(ctx, tx, operationID)
		if getErr != nil {
			return CommandRecord{}, getErr
		}
		if record.Receipt.State != commandReceiptStateCompleted || record.Receipt.ResultStatus != resultStatus ||
			record.Receipt.Reason != reason || record.Receipt.TargetID != targetID ||
			record.Receipt.ResourceVersion != resourceVersion || record.Receipt.ResultData != resultData {
			return CommandRecord{}, ErrCommandCompletionConflict
		}
		if err := tx.Commit(); err != nil {
			return CommandRecord{}, err
		}
		return record, nil
	}
	if resultData != "" {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO plugin_host_command_result_data (operation_id, result_data) VALUES (?, ?)
			ON CONFLICT(operation_id) DO UPDATE SET result_data = excluded.result_data
		`), operationID, resultData); err != nil {
			return CommandRecord{}, err
		}
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_host_command_intents SET updated_at = ? WHERE operation_id = ?`), now, operationID)
	if err != nil {
		return CommandRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommandRecord{}, err
	}
	return s.Get(ctx, operationID)
}

// Get returns the durable intent and receipt for operationID.
func (s *CommandStore) Get(ctx context.Context, operationID string) (CommandRecord, error) {
	return getCommandByOperation(ctx, s.ro, operationID)
}

func getCommandByKey(ctx context.Context, q sqlx.ExtContext, installationID, workspaceID, method, idempotencyKey string) (CommandRecord, error) {
	var row commandRow
	query := commandSelect + ` WHERE i.installation_id = ? AND i.workspace_id = ? AND i.method = ? AND i.idempotency_key = ?`
	reader, ok := q.(commandQueryer)
	if !ok {
		return CommandRecord{}, errors.New("plugin command store: query handle cannot read command records")
	}
	err := reader.GetContext(ctx, &row, reader.Rebind(query), installationID, workspaceID, method, idempotencyKey)
	return row.toRecord(), normalizeCommandReadError(err)
}

func getCommandByOperation(ctx context.Context, q sqlx.ExtContext, operationID string) (CommandRecord, error) {
	var row commandRow
	query := commandSelect + ` WHERE i.operation_id = ?`
	reader, ok := q.(commandQueryer)
	if !ok {
		return CommandRecord{}, errors.New("plugin command store: query handle cannot read command records")
	}
	err := reader.GetContext(ctx, &row, reader.Rebind(query), operationID)
	return row.toRecord(), normalizeCommandReadError(err)
}

const commandSelect = `
	SELECT i.operation_id AS i_operation_id, i.installation_id AS i_installation_id,
		i.workspace_id AS i_workspace_id, i.request_id AS i_request_id,
		i.idempotency_key AS i_idempotency_key, i.method AS i_method,
		i.capability_id AS i_capability_id, i.payload_digest AS i_payload_digest,
		i.target_id AS i_target_id, i.expected_resource_version AS i_expected_resource_version,
		i.approval_revision AS i_approval_revision, i.manifest_digest AS i_manifest_digest,
		i.created_at AS i_created_at, i.updated_at AS i_updated_at,
		r.receipt_id AS r_receipt_id, r.operation_id AS r_operation_id, r.state AS r_state,
		r.result_status AS r_result_status, r.reason AS r_reason, r.target_id AS r_target_id,
		r.resource_version AS r_resource_version, COALESCE(d.result_data, '') AS r_result_data,
		r.created_at AS r_created_at, r.updated_at AS r_updated_at
	FROM plugin_host_command_intents i
	JOIN plugin_host_command_receipts r ON r.operation_id = i.operation_id
	LEFT JOIN plugin_host_command_result_data d ON d.operation_id = i.operation_id`

type commandQueryer interface {
	GetContext(context.Context, any, string, ...any) error
	Rebind(string) string
}

type commandRow struct {
	OperationID             string `db:"i_operation_id"`
	InstallationID          string `db:"i_installation_id"`
	WorkspaceID             string `db:"i_workspace_id"`
	RequestID               string `db:"i_request_id"`
	IdempotencyKey          string `db:"i_idempotency_key"`
	Method                  string `db:"i_method"`
	CapabilityID            string `db:"i_capability_id"`
	PayloadDigest           string `db:"i_payload_digest"`
	ExpectedResourceVersion string `db:"i_expected_resource_version"`
	IntentTargetID          string `db:"i_target_id"`
	ApprovalRevision        uint64 `db:"i_approval_revision"`
	ManifestDigest          string `db:"i_manifest_digest"`
	IntentCreatedAt         string `db:"i_created_at"`
	IntentUpdatedAt         string `db:"i_updated_at"`
	ReceiptID               string `db:"r_receipt_id"`
	ReceiptOperationID      string `db:"r_operation_id"`
	ReceiptState            string `db:"r_state"`
	ResultStatus            string `db:"r_result_status"`
	Reason                  string `db:"r_reason"`
	ReceiptTargetID         string `db:"r_target_id"`
	ResourceVersion         string `db:"r_resource_version"`
	ResultData              string `db:"r_result_data"`
	ReceiptCreatedAt        string `db:"r_created_at"`
	ReceiptUpdatedAt        string `db:"r_updated_at"`
}

func (row commandRow) toRecord() CommandRecord {
	return CommandRecord{
		Intent: CommandIntent{
			OperationID: row.OperationID, InstallationID: row.InstallationID, WorkspaceID: row.WorkspaceID,
			RequestID: row.RequestID, IdempotencyKey: row.IdempotencyKey, Method: row.Method,
			CapabilityID: row.CapabilityID, PayloadDigest: row.PayloadDigest, TargetID: row.IntentTargetID,
			ExpectedResourceVersion: row.ExpectedResourceVersion, ApprovalRevision: row.ApprovalRevision,
			ManifestDigest: row.ManifestDigest, CreatedAt: row.IntentCreatedAt, UpdatedAt: row.IntentUpdatedAt,
		},
		Receipt: CommandReceipt{
			ID: row.ReceiptID, OperationID: row.ReceiptOperationID, State: row.ReceiptState,
			ResultStatus: row.ResultStatus, Reason: row.Reason, TargetID: row.ReceiptTargetID,
			ResourceVersion: row.ResourceVersion, ResultData: row.ResultData,
			CreatedAt: row.ReceiptCreatedAt, UpdatedAt: row.ReceiptUpdatedAt,
		},
	}
}

func normalizeCommandReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	return err
}

func validateCommandIntent(input CommandIntent) error {
	for name, value := range map[string]string{
		"installation_id": input.InstallationID,
		"workspace_id":    input.WorkspaceID,
		"request_id":      input.RequestID,
		"idempotency_key": input.IdempotencyKey,
		"method":          input.Method,
		"capability_id":   input.CapabilityID,
		"payload_digest":  input.PayloadDigest,
		"target_id":       input.TargetID,
		"manifest_digest": input.ManifestDigest,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("plugin command store: invalid %s", name)
		}
	}
	if input.Method != "CreateTaskExact" && strings.TrimSpace(input.ExpectedResourceVersion) == "" {
		return errors.New("plugin command store: expected resource version is required")
	}
	if input.ApprovalRevision == 0 {
		return errors.New("plugin command store: approval revision is required")
	}
	return nil
}
