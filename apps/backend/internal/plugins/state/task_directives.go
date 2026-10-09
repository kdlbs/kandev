package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrTaskDirectiveNotFound        = errors.New("plugin task directive was not found")
	ErrTaskDirectiveConflict        = errors.New("plugin task directive state conflict")
	ErrTaskDirectivePayloadConflict = errors.New("plugin task directive idempotency payload conflict")
)

const (
	taskDirectiveStatePending  = "pending"
	taskDirectiveStateResolved = "resolved"
)

// TaskDirectiveRecord is a Host-owned, non-amplifying authorization record.
// Its capability class is still subject to the current workspace approval and
// managed-agent policy when a worker attempts an operation.
type TaskDirectiveRecord struct {
	ID                             string `db:"directive_id"`
	OperationID                    string `db:"operation_id"`
	InstallationID                 string `db:"installation_id"`
	WorkspaceID                    string `db:"workspace_id"`
	TaskID                         string `db:"task_id"`
	SessionID                      string `db:"session_id"`
	IdempotencyKey                 string `db:"idempotency_key"`
	PayloadDigest                  string `db:"payload_digest"`
	CapabilityClass                string `db:"capability_class"`
	InstructionDigest              string `db:"instruction_digest"`
	ExpectedTaskResourceVersion    string `db:"expected_task_resource_version"`
	ExpectedSessionResourceVersion string `db:"expected_session_resource_version"`
	ApprovalRevision               uint64 `db:"approval_revision"`
	State                          string `db:"state"`
	ExpiresAt                      string `db:"expires_at"`
	Resolution                     string `db:"resolution"`
	ResolutionDigest               string `db:"resolution_digest"`
	ResolutionOperationID          string `db:"resolution_operation_id"`
	AuditID                        string `db:"audit_id"`
	CreatedAt                      string `db:"created_at"`
	UpdatedAt                      string `db:"updated_at"`
}

// ResourceVersion is an opaque version for the directive's visible state.
func (d TaskDirectiveRecord) ResourceVersion() string {
	canonical := d.ID + "\x00" + d.State + "\x00" + d.UpdatedAt + "\x00" + d.ResolutionDigest
	digest := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s *CommandStore) initTaskDirectiveSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_host_task_directives (
			directive_id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL UNIQUE,
			installation_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			payload_digest TEXT NOT NULL,
			capability_class TEXT NOT NULL,
			instruction_digest TEXT NOT NULL,
			expected_task_resource_version TEXT NOT NULL,
			expected_session_resource_version TEXT NOT NULL,
			approval_revision BIGINT NOT NULL,
			state TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			resolution TEXT NOT NULL DEFAULT '',
			resolution_digest TEXT NOT NULL DEFAULT '',
			resolution_operation_id TEXT NOT NULL DEFAULT '',
			audit_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (installation_id, workspace_id, idempotency_key)
		);
	`)
	return err
}

// IssueTaskDirective commits one directive and its idempotency tombstone.
//
//nolint:cyclop // The store validates and persists one directive with its operation identity.
func (s *CommandStore) IssueTaskDirective(ctx context.Context, input TaskDirectiveRecord) (TaskDirectiveRecord, bool, error) {
	if input.ID == "" || input.OperationID == "" || input.InstallationID == "" || input.WorkspaceID == "" ||
		input.TaskID == "" || input.SessionID == "" || input.IdempotencyKey == "" || input.PayloadDigest == "" ||
		input.CapabilityClass == "" || input.InstructionDigest == "" || input.ExpectedTaskResourceVersion == "" ||
		input.ExpectedSessionResourceVersion == "" || input.ApprovalRevision == 0 || input.ExpiresAt == "" || input.AuditID == "" {
		return TaskDirectiveRecord{}, false, errors.New("plugin task directive: required fields are missing")
	}
	if _, err := time.Parse(time.RFC3339Nano, input.ExpiresAt); err != nil {
		return TaskDirectiveRecord{}, false, errors.New("plugin task directive: expiry is invalid")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.State = taskDirectiveStatePending
	input.CreatedAt = now
	input.UpdatedAt = now
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO plugin_host_task_directives (
			directive_id, operation_id, installation_id, workspace_id, task_id, session_id,
			idempotency_key, payload_digest, capability_class, instruction_digest,
			expected_task_resource_version, expected_session_resource_version, approval_revision,
			state, expires_at, audit_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(installation_id, workspace_id, idempotency_key) DO NOTHING
	`), input.ID, input.OperationID, input.InstallationID, input.WorkspaceID, input.TaskID, input.SessionID,
		input.IdempotencyKey, input.PayloadDigest, input.CapabilityClass, input.InstructionDigest,
		input.ExpectedTaskResourceVersion, input.ExpectedSessionResourceVersion, input.ApprovalRevision,
		input.State, input.ExpiresAt, input.AuditID, input.CreatedAt, input.UpdatedAt)
	if err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	if affected == 0 {
		record, getErr := getTaskDirective(ctx, tx, input.InstallationID, input.WorkspaceID, input.ID)
		if errors.Is(getErr, sql.ErrNoRows) {
			record, getErr = getTaskDirectiveByIdempotencyKey(ctx, tx, input.InstallationID, input.WorkspaceID, input.IdempotencyKey)
		}
		if getErr != nil {
			return TaskDirectiveRecord{}, false, getErr
		}
		if record.PayloadDigest != input.PayloadDigest {
			return TaskDirectiveRecord{}, false, ErrTaskDirectivePayloadConflict
		}
		if err := tx.Commit(); err != nil {
			return TaskDirectiveRecord{}, false, err
		}
		return record, true, nil
	}
	if err := tx.Commit(); err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	return input, false, nil
}

// GetTaskDirective returns one directive scoped to its installation and workspace.
func (s *CommandStore) GetTaskDirective(ctx context.Context, installationID, workspaceID, directiveID string) (TaskDirectiveRecord, error) {
	record, err := getTaskDirective(ctx, s.ro, installationID, workspaceID, directiveID)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskDirectiveRecord{}, ErrTaskDirectiveNotFound
	}
	return record, err
}

// ListTaskDirectives returns a stable bounded-by-caller snapshot in creation order.
func (s *CommandStore) ListTaskDirectives(ctx context.Context, installationID, workspaceID, taskID string) ([]TaskDirectiveRecord, error) {
	query := `SELECT ` + taskDirectiveColumns + ` FROM plugin_host_task_directives WHERE installation_id = ? AND workspace_id = ?`
	args := []any{installationID, workspaceID}
	if taskID != "" {
		query += ` AND task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY created_at, directive_id`
	var records []TaskDirectiveRecord
	if err := s.ro.SelectContext(ctx, &records, s.ro.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list plugin task directives: %w", err)
	}
	return records, nil
}

// ResolveTaskDirective consumes one pending directive. The operation identity
// makes a lost acknowledgement replayable without allowing a second resolution.
//
//nolint:cyclop // The store resolves one directive and records an idempotent result.
func (s *CommandStore) ResolveTaskDirective(
	ctx context.Context,
	installationID, workspaceID, directiveID, expectedResourceVersion, operationID, resolution, resolutionDigest string,
) (TaskDirectiveRecord, bool, error) {
	if operationID == "" || resolution == "" || resolutionDigest == "" {
		return TaskDirectiveRecord{}, false, errors.New("plugin task directive: resolution is incomplete")
	}
	record, err := getTaskDirective(ctx, s.db, installationID, workspaceID, directiveID)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskDirectiveRecord{}, false, ErrTaskDirectiveNotFound
	}
	if err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	if record.State == taskDirectiveStateResolved && record.ResolutionOperationID == operationID {
		if record.Resolution != resolution || record.ResolutionDigest != resolutionDigest {
			return TaskDirectiveRecord{}, false, ErrTaskDirectivePayloadConflict
		}
		return record, true, nil
	}
	if record.State != taskDirectiveStatePending || record.ResourceVersion() != expectedResourceVersion {
		return record, false, ErrTaskDirectiveConflict
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		return TaskDirectiveRecord{}, false, fmt.Errorf("parse stored task directive expiry: %w", err)
	}
	if !time.Now().UTC().Before(expiresAt) {
		_, _ = s.MarkTaskDirectiveState(ctx, installationID, workspaceID, directiveID, record.ResourceVersion(), "expired")
		return TaskDirectiveRecord{}, false, ErrTaskDirectiveConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_host_task_directives
		SET state = 'resolved', resolution = ?, resolution_digest = ?, resolution_operation_id = ?, updated_at = ?
		WHERE directive_id = ? AND installation_id = ? AND workspace_id = ? AND state = 'pending' AND updated_at = ?
	`), resolution, resolutionDigest, operationID, now, directiveID, installationID, workspaceID, record.UpdatedAt)
	if err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return TaskDirectiveRecord{}, false, err
	}
	if affected == 0 {
		current, getErr := getTaskDirective(ctx, s.db, installationID, workspaceID, directiveID)
		if getErr != nil {
			return TaskDirectiveRecord{}, false, getErr
		}
		if current.State == taskDirectiveStateResolved && current.ResolutionOperationID == operationID &&
			current.Resolution == resolution && current.ResolutionDigest == resolutionDigest {
			return current, true, nil
		}
		return current, false, ErrTaskDirectiveConflict
	}
	resolved, err := getTaskDirective(ctx, s.ro, installationID, workspaceID, directiveID)
	return resolved, false, err
}

// MarkTaskDirectiveState records an irreversible terminal state for a pending
// directive after expiry, revocation, or session-generation replacement.
func (s *CommandStore) MarkTaskDirectiveState(
	ctx context.Context, installationID, workspaceID, directiveID, expectedResourceVersion, terminalState string,
) (TaskDirectiveRecord, error) {
	if terminalState != "expired" && terminalState != "revoked" && terminalState != "generation_fenced" {
		return TaskDirectiveRecord{}, errors.New("plugin task directive: terminal state is invalid")
	}
	record, err := getTaskDirective(ctx, s.db, installationID, workspaceID, directiveID)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskDirectiveRecord{}, ErrTaskDirectiveNotFound
	}
	if err != nil {
		return TaskDirectiveRecord{}, err
	}
	if record.State == terminalState || record.State != taskDirectiveStatePending {
		return record, nil
	}
	if record.ResourceVersion() != expectedResourceVersion {
		return record, ErrTaskDirectiveConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_host_task_directives SET state = ?, updated_at = ?
		WHERE directive_id = ? AND installation_id = ? AND workspace_id = ? AND state = 'pending' AND updated_at = ?
	`), terminalState, now, directiveID, installationID, workspaceID, record.UpdatedAt)
	if err != nil {
		return TaskDirectiveRecord{}, err
	}
	return getTaskDirective(ctx, s.ro, installationID, workspaceID, directiveID)
}

const taskDirectiveColumns = `directive_id, operation_id, installation_id, workspace_id, task_id, session_id,
	idempotency_key, payload_digest, capability_class, instruction_digest,
	expected_task_resource_version, expected_session_resource_version, approval_revision,
	state, expires_at, resolution, resolution_digest, resolution_operation_id, audit_id, created_at, updated_at`

type taskDirectiveQuery interface {
	GetContext(context.Context, any, string, ...any) error
	Rebind(string) string
}

func getTaskDirective(ctx context.Context, q taskDirectiveQuery, installationID, workspaceID, directiveID string) (TaskDirectiveRecord, error) {
	var record TaskDirectiveRecord
	err := q.GetContext(ctx, &record,
		q.Rebind(`SELECT `+taskDirectiveColumns+` FROM plugin_host_task_directives WHERE installation_id = ? AND workspace_id = ? AND directive_id = ?`),
		installationID, workspaceID, directiveID)
	return record, err
}

func getTaskDirectiveByIdempotencyKey(ctx context.Context, q taskDirectiveQuery, installationID, workspaceID, idempotencyKey string) (TaskDirectiveRecord, error) {
	var record TaskDirectiveRecord
	err := q.GetContext(ctx, &record,
		q.Rebind(`SELECT `+taskDirectiveColumns+` FROM plugin_host_task_directives WHERE installation_id = ? AND workspace_id = ? AND idempotency_key = ?`),
		installationID, workspaceID, idempotencyKey)
	return record, err
}

var _ taskDirectiveQuery = (*sqlx.DB)(nil)
var _ taskDirectiveQuery = (*sqlx.Tx)(nil)
