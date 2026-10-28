package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	taskChangeRegistrationStatePending    = "pending"
	taskChangeRegistrationStateSucceeded  = "succeeded"
	taskChangeRegistrationStateSuppressed = "suppressed"
	taskChangeRegistrationStateUnlinked   = "unlinked"
	taskChangeRegistrationStateRetryable  = "retryable"
	taskChangeRegistrationStateUncertain  = "uncertain"
	taskChangeRegistrationStateFailed     = "failed"
)

const taskChangeRegistrationSchema = `
CREATE TABLE IF NOT EXISTS task_change_registration_intents (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	workspace_id TEXT NOT NULL,
	actor_id TEXT NOT NULL DEFAULT '',
	source_kind TEXT NOT NULL,
	origin_id TEXT NOT NULL DEFAULT '',
	queue_session_id TEXT NOT NULL DEFAULT '',
	queue_entry_id TEXT NOT NULL DEFAULT '',
	message_id TEXT NOT NULL DEFAULT '',
	source_revision TEXT NOT NULL DEFAULT '',
	selection_id TEXT NOT NULL DEFAULT '',
	reference_ref TEXT NOT NULL,
	reference_snapshot TEXT NOT NULL,
	provider TEXT NOT NULL DEFAULT '',
	provider_host TEXT NOT NULL DEFAULT '',
	repository_id TEXT NOT NULL DEFAULT '',
	connection_scope TEXT NOT NULL DEFAULT '',
	provider_repository_id TEXT NOT NULL DEFAULT '',
	change_request_id TEXT NOT NULL DEFAULT '',
	resolved_identity TEXT NOT NULL DEFAULT '',
	expected_revision BIGINT,
	association_revision BIGINT,
	operation_id TEXT NOT NULL DEFAULT '',
	operation_fingerprint TEXT NOT NULL DEFAULT '',
	operation_receipt TEXT NOT NULL DEFAULT '',
	installation_identity TEXT NOT NULL DEFAULT '',
	contract_version INTEGER,
	state TEXT NOT NULL DEFAULT 'pending',
	error_code TEXT NOT NULL DEFAULT '',
	attempt_count INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	last_attempt_at TIMESTAMP,
	completed_at TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_task_change_registration_origin
	ON task_change_registration_intents(task_id, source_kind, origin_id, source_revision, reference_ref)
	WHERE origin_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_task_change_registration_selection
	ON task_change_registration_intents(task_id, selection_id)
	WHERE selection_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_task_change_registration_operation
	ON task_change_registration_intents(task_id, operation_id)
	WHERE operation_id <> '';
CREATE INDEX IF NOT EXISTS idx_task_change_registration_pending
	ON task_change_registration_intents(state, updated_at) WHERE state IN ('pending', 'retryable', 'uncertain');
CREATE INDEX IF NOT EXISTS idx_task_change_registration_succeeded
	ON task_change_registration_intents(id) WHERE state = 'succeeded';
CREATE TABLE IF NOT EXISTS task_change_registration_actions (
	task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	operation_id TEXT NOT NULL,
	requested_id TEXT NOT NULL,
	intent_id TEXT NOT NULL,
	actor_id TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY(task_id, operation_id)
);
`

var ErrTaskChangeRegistrationConflict = errors.New("task change registration identity conflict")
var ErrTaskChangeRegistrationNotRetryable = errors.New("task change registration cannot be retried with its accepted identity")
var ErrTaskChangeRegistrationOperationConflict = errors.New("task change registration operation identity conflict")

type TaskChangeRegistrationIntent struct {
	ID                   string     `db:"id" json:"id"`
	TaskID               string     `db:"task_id" json:"task_id"`
	WorkspaceID          string     `db:"workspace_id" json:"workspace_id"`
	ActorID              string     `db:"actor_id" json:"actor_id,omitempty"`
	SourceKind           string     `db:"source_kind" json:"source_kind"`
	OriginID             string     `db:"origin_id" json:"origin_id,omitempty"`
	QueueSessionID       string     `db:"queue_session_id" json:"queue_session_id,omitempty"`
	QueueEntryID         string     `db:"queue_entry_id" json:"queue_entry_id,omitempty"`
	MessageID            string     `db:"message_id" json:"message_id,omitempty"`
	SourceRevision       string     `db:"source_revision" json:"source_revision,omitempty"`
	SelectionID          string     `db:"selection_id" json:"selection_id,omitempty"`
	ReferenceRef         string     `db:"reference_ref" json:"reference_ref"`
	ReferenceSnapshot    string     `db:"reference_snapshot" json:"reference_snapshot"`
	Provider             string     `db:"provider" json:"provider,omitempty"`
	ProviderHost         string     `db:"provider_host" json:"provider_host,omitempty"`
	RepositoryID         string     `db:"repository_id" json:"repository_id,omitempty"`
	ConnectionScope      string     `db:"connection_scope" json:"connection_scope,omitempty"`
	ProviderRepositoryID string     `db:"provider_repository_id" json:"provider_repository_id,omitempty"`
	ChangeRequestID      string     `db:"change_request_id" json:"change_request_id,omitempty"`
	ResolvedIdentity     string     `db:"resolved_identity" json:"resolved_identity,omitempty"`
	ExpectedRevision     *int64     `db:"expected_revision" json:"expected_revision,omitempty"`
	AssociationRevision  *int64     `db:"association_revision" json:"association_revision,omitempty"`
	OperationID          string     `db:"operation_id" json:"operation_id,omitempty"`
	OperationFingerprint string     `db:"operation_fingerprint" json:"operation_fingerprint,omitempty"`
	OperationReceipt     string     `db:"operation_receipt" json:"operation_receipt,omitempty"`
	InstallationIdentity string     `db:"installation_identity" json:"installation_identity,omitempty"`
	ContractVersion      *int       `db:"contract_version" json:"contract_version,omitempty"`
	State                string     `db:"state" json:"state"`
	ErrorCode            string     `db:"error_code" json:"error_code,omitempty"`
	AttemptCount         int        `db:"attempt_count" json:"attempt_count"`
	CreatedAt            time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at" json:"updated_at"`
	LastAttemptAt        *time.Time `db:"last_attempt_at" json:"last_attempt_at,omitempty"`
	CompletedAt          *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}

type TaskChangeRegistrationNotifications struct {
	MessageIDs      []string
	QueueSessionIDs []string
}

func (r *Repository) initTaskChangeRegistrationSchema() error {
	if _, err := r.db.ExecContext(context.Background(), taskChangeRegistrationSchema); err != nil {
		return fmt.Errorf("initialize task change registration schema: %w", err)
	}
	if err := r.migrateTaskChangeRegistrationColumns(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) migrateTaskChangeRegistrationColumns() error {
	if r.migrate == nil {
		return nil
	}
	columns := []struct {
		name  string
		sql   string
		label string
	}{
		{"actor_id", `ALTER TABLE task_change_registration_intents ADD COLUMN actor_id TEXT NOT NULL DEFAULT ''`, "actor identity"},
		{"repository_id", `ALTER TABLE task_change_registration_intents ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`, "repository identity"},
		{"queue_session_id", `ALTER TABLE task_change_registration_intents ADD COLUMN queue_session_id TEXT NOT NULL DEFAULT ''`, "queue session"},
		{"queue_entry_id", `ALTER TABLE task_change_registration_intents ADD COLUMN queue_entry_id TEXT NOT NULL DEFAULT ''`, "queue entry"},
		{"message_id", `ALTER TABLE task_change_registration_intents ADD COLUMN message_id TEXT NOT NULL DEFAULT ''`, "message origin"},
	}
	for _, column := range columns {
		if err := r.migrate.Apply("task_change_registration_intents."+column.name, column.sql); err != nil {
			return fmt.Errorf("migrate task change registration %s: %w", column.label, err)
		}
	}
	return nil
}

// CreateTaskChangeRegistrationIntentTx inserts an acceptance receipt in the
// caller's transaction. Exact retries return the original row; changed reuse
// of an origin, selection, or operation key fails closed.
func (r *Repository) CreateTaskChangeRegistrationIntentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	intent *TaskChangeRegistrationIntent,
) (*TaskChangeRegistrationIntent, bool, error) {
	if tx == nil {
		return nil, false, errors.New("task change registration intent is incomplete")
	}
	if err := prepareTaskChangeRegistrationIntent(intent); err != nil {
		return nil, false, err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO task_change_registration_intents (
			id, task_id, workspace_id, source_kind, origin_id, queue_session_id, queue_entry_id, message_id, source_revision, selection_id, reference_ref, actor_id,
			reference_snapshot, provider, provider_host, repository_id, connection_scope, provider_repository_id,
			change_request_id, resolved_identity, expected_revision, association_revision, operation_id,
			operation_fingerprint, operation_receipt, installation_identity, contract_version, state,
			error_code, attempt_count, created_at, updated_at, last_attempt_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`),
		intent.ID, intent.TaskID, intent.WorkspaceID, intent.SourceKind, intent.OriginID, intent.QueueSessionID, intent.QueueEntryID, intent.MessageID, intent.SourceRevision,
		intent.SelectionID, intent.ReferenceRef, intent.ActorID, intent.ReferenceSnapshot, intent.Provider, intent.ProviderHost, intent.RepositoryID,
		intent.ConnectionScope, intent.ProviderRepositoryID, intent.ChangeRequestID, intent.ResolvedIdentity,
		intent.ExpectedRevision, intent.AssociationRevision, intent.OperationID, intent.OperationFingerprint,
		intent.OperationReceipt, intent.InstallationIdentity, intent.ContractVersion,
		intent.State, intent.ErrorCode, intent.AttemptCount, intent.CreatedAt, intent.UpdatedAt, intent.LastAttemptAt, intent.CompletedAt)
	if err != nil {
		return nil, false, fmt.Errorf("insert task change registration intent: %w", err)
	}
	created, err := getTaskChangeRegistrationIntentByConflictKey(ctx, tx, intent)
	if err != nil {
		return nil, false, err
	}
	if !sameTaskChangeRegistrationIntent(created, intent) {
		return nil, false, ErrTaskChangeRegistrationConflict
	}
	return created, created.ID == intent.ID, nil
}

func prepareTaskChangeRegistrationIntent(intent *TaskChangeRegistrationIntent) error {
	if intent == nil || strings.TrimSpace(intent.TaskID) == "" ||
		strings.TrimSpace(intent.WorkspaceID) == "" || strings.TrimSpace(intent.SourceKind) == "" ||
		strings.TrimSpace(intent.ReferenceRef) == "" || len(intent.ReferenceSnapshot) == 0 {
		return errors.New("task change registration intent is incomplete")
	}
	if !json.Valid([]byte(intent.ReferenceSnapshot)) {
		return errors.New("task change registration snapshot must be valid JSON")
	}
	if intent.ID == "" {
		intent.ID = uuid.NewString()
	}
	if intent.State == "" {
		intent.State = cleanupPhasePending
	}
	if !validTaskChangeRegistrationState(intent.State) {
		return errors.New("task change registration state is invalid")
	}
	now := time.Now().UTC()
	if intent.CreatedAt.IsZero() {
		intent.CreatedAt = now
	}
	intent.UpdatedAt = now
	return nil
}

func (r *Repository) GetTaskChangeRegistrationIntent(ctx context.Context, taskID, intentID string) (*TaskChangeRegistrationIntent, error) {
	var intent TaskChangeRegistrationIntent
	err := r.ro.GetContext(ctx, &intent, r.ro.Rebind(`
		SELECT * FROM task_change_registration_intents WHERE task_id = ? AND id = ? LIMIT 1`), taskID, intentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &intent, err
}

func (r *Repository) GetTaskChangeRegistrationIntentTx(
	ctx context.Context, tx *sqlx.Tx, taskID, intentID string,
) (*TaskChangeRegistrationIntent, error) {
	if tx == nil || strings.TrimSpace(taskID) == "" || strings.TrimSpace(intentID) == "" {
		return nil, errors.New("task change registration transaction identity is incomplete")
	}
	return getTaskChangeRegistrationIntentByIDTx(ctx, tx, taskID, intentID)
}

func (r *Repository) GetTaskChangeRegistrationIntentBySelection(
	ctx context.Context, taskID, selectionID string,
) (*TaskChangeRegistrationIntent, error) {
	var intent TaskChangeRegistrationIntent
	err := r.ro.GetContext(ctx, &intent, r.ro.Rebind(`
		SELECT * FROM task_change_registration_intents WHERE task_id = ? AND selection_id = ? LIMIT 1`),
		taskID, selectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &intent, err
}

func (r *Repository) ListPendingTaskChangeRegistrationIntents(ctx context.Context, limit int) ([]*TaskChangeRegistrationIntent, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var rows []TaskChangeRegistrationIntent
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT * FROM task_change_registration_intents
		WHERE state IN ('pending', 'retryable', 'uncertain') ORDER BY updated_at, id LIMIT ?`), limit); err != nil {
		return nil, err
	}
	out := make([]*TaskChangeRegistrationIntent, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// ListSucceededTaskChangeRegistrationIntentsAfter returns a stable keyset page
// of completed message registrations for canonical association reconciliation.
func (r *Repository) ListSucceededTaskChangeRegistrationIntentsAfter(
	ctx context.Context, afterID string, limit int,
) ([]*TaskChangeRegistrationIntent, error) {
	if limit <= 0 || limit > 256 {
		limit = 64
	}
	var rows []TaskChangeRegistrationIntent
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT * FROM task_change_registration_intents
		WHERE state = 'succeeded' AND source_kind IN ('message', 'queue_admission', 'queue_edit') AND id > ?
		ORDER BY id LIMIT ?`), afterID, limit); err != nil {
		return nil, err
	}
	out := make([]*TaskChangeRegistrationIntent, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

func (r *Repository) GetQueuedRegistrationSession(ctx context.Context, taskID, selectionID string) (string, bool, error) {
	row, err := r.findQueuedRegistrationRow(ctx, r.ro, taskID, selectionID)
	if err != nil || row == nil {
		return "", false, err
	}
	return row.SessionID, true, nil
}

func (r *Repository) SetTaskChangeRegistrationIntentResult(
	ctx context.Context,
	intentID, expectedState, state, errorCode string,
	resolvedIdentity json.RawMessage,
	expectedRevision, associationRevision *int64,
	operationReceipt string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := r.SetTaskChangeRegistrationIntentResultTx(ctx, tx, intentID, expectedState, state,
		errorCode, resolvedIdentity, expectedRevision, associationRevision, operationReceipt)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return updated, nil
}

// SetTaskChangeRegistrationIntentResultTx updates an intent in the caller's
// transaction so its result can commit with the provider association.
func (r *Repository) SetTaskChangeRegistrationIntentResultTx(
	ctx context.Context,
	tx *sqlx.Tx,
	intentID, expectedState, state, errorCode string,
	resolvedIdentity json.RawMessage,
	expectedRevision, associationRevision *int64,
	operationReceipt string,
) (bool, error) {
	if tx == nil {
		return false, errors.New("task change registration transaction is required")
	}
	if !validTaskChangeRegistrationState(state) || !json.Valid(resolvedIdentity) && len(resolvedIdentity) > 0 {
		return false, errors.New("task change registration result is invalid")
	}
	now := time.Now().UTC()
	completedAt := any(nil)
	if state == taskChangeRegistrationStateSucceeded || state == taskChangeRegistrationStateFailed || state == taskChangeRegistrationStateSuppressed || state == taskChangeRegistrationStateUnlinked {
		completedAt = now
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_change_registration_intents SET state = ?, error_code = ?, resolved_identity = ?,
			expected_revision = COALESCE(?, expected_revision), association_revision = ?, operation_receipt = ?,
			attempt_count = attempt_count + 1, last_attempt_at = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND state = ?`), state, boundedRegistrationError(errorCode), string(resolvedIdentity),
		expectedRevision, associationRevision, operationReceipt, now, now, completedAt, intentID, expectedState)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// SetMessageRegistrationStatusTx updates the selected reference projection in
// the originating user message. Callers share this transaction with the
// association and intent result so a committed link cannot leave a stale
// pending message projection.
func (r *Repository) SetMessageRegistrationStatusTx(
	ctx context.Context, tx *sqlx.Tx, messageID, selectionID, state, errorCode string,
) error {
	if tx == nil || strings.TrimSpace(messageID) == "" || strings.TrimSpace(selectionID) == "" {
		return errors.New("message registration projection identity is incomplete")
	}
	status, err := messageRegistrationStatus(state)
	if err != nil {
		return err
	}
	metadata, messageFound, err := r.readMessageRegistrationMetadataTx(ctx, tx, messageID)
	if err != nil {
		return fmt.Errorf("read registration origin message: %w", err)
	}
	if !messageFound {
		return nil
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(metadata["entity_references"], &references); err != nil {
		return fmt.Errorf("decode registration origin references: %w", err)
	}
	if !setMessageRegistrationReferenceStatus(references, selectionID, status, errorCode) {
		return nil
	}
	encodedReferences, err := json.Marshal(references)
	if err != nil {
		return fmt.Errorf("encode registration origin references: %w", err)
	}
	metadata["entity_references"] = encodedReferences
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode registration origin message: %w", err)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_session_messages SET metadata = ?, updated_at = ? WHERE id = ?`),
		string(encodedMetadata), time.Now().UTC(), messageID)
	if err != nil {
		return fmt.Errorf("update registration origin message: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func setMessageRegistrationReferenceStatus(
	references []apiv1.EntityReference,
	selectionID string,
	status apiv1.EntityReferenceRegistrationStatus,
	errorCode string,
) bool {
	for index := range references {
		if references[index].SelectionID != selectionID {
			continue
		}
		references[index].RegistrationStatus = status
		references[index].RegistrationErrorCode = boundedRegistrationError(errorCode)
		return true
	}
	return false
}

// ProjectTaskChangeRegistrationStatusTx updates the source that currently owns
// the accepted reference. A queue intent follows its dispatched transcript
// message once message_id is set; otherwise status remains on the queue row.
func (r *Repository) ProjectTaskChangeRegistrationStatusTx(
	ctx context.Context,
	tx *sqlx.Tx,
	intent *TaskChangeRegistrationIntent,
	state, errorCode string,
) (TaskChangeRegistrationNotifications, error) {
	var notifications TaskChangeRegistrationNotifications
	if tx == nil || intent == nil || intent.SelectionID == "" {
		return notifications, errors.New("task change registration projection identity is incomplete")
	}
	messageID := intent.MessageID
	if intent.SourceKind == TaskChangeRegistrationSourceMessage && messageID == "" {
		messageID = intent.OriginID
	}
	if messageID != "" {
		if err := r.SetMessageRegistrationStatusTx(ctx, tx, messageID, intent.SelectionID, state, errorCode); err != nil {
			return notifications, err
		}
		notifications.MessageIDs = append(notifications.MessageIDs, messageID)
		return notifications, nil
	}
	if intent.QueueEntryID == "" {
		return notifications, nil
	}
	queueSessionID, found, err := r.setQueueRegistrationStatusTx(
		ctx, tx, intent.TaskID, intent.QueueEntryID, intent.SelectionID, state, errorCode,
	)
	if err != nil {
		return notifications, err
	}
	if found && queueSessionID != "" {
		notifications.QueueSessionIDs = append(notifications.QueueSessionIDs, queueSessionID)
	}
	return notifications, nil
}

func (r *Repository) readMessageRegistrationMetadataTx(
	ctx context.Context, tx *sqlx.Tx, messageID string,
) (map[string]json.RawMessage, bool, error) {
	query := `SELECT metadata FROM task_session_messages WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var rawMetadata string
	if err := tx.GetContext(ctx, &rawMetadata, tx.Rebind(query), messageID); errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
		return nil, false, fmt.Errorf("decode registration origin message: %w", err)
	}
	return metadata, true, nil
}

// MarkTaskChangeRegistrationUnlinked projects a committed provider tombstone
// onto every earlier successful mention of that exact association. The
// revision comparison prevents a delayed deletion event from overwriting a
// newer explicit relink.
func (r *Repository) MarkTaskChangeRegistrationUnlinked(
	ctx context.Context,
	taskID, workspaceID, provider, associationID, providerHost, connectionScope, providerRepositoryID, changeRequestID string,
	associationRevision int64,
) (TaskChangeRegistrationNotifications, error) {
	var notifications TaskChangeRegistrationNotifications
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(workspaceID) == "" ||
		strings.TrimSpace(provider) == "" || strings.TrimSpace(associationID) == "" || associationRevision <= 0 {
		return notifications, errors.New("task change registration unlink identity is incomplete")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return notifications, err
	}
	defer func() { _ = tx.Rollback() }()
	candidates, err := taskChangeRegistrationUnlinkCandidates(ctx, tx, taskID, workspaceID, provider,
		associationID, providerHost, connectionScope, providerRepositoryID, changeRequestID, associationRevision)
	if err != nil {
		return notifications, err
	}
	updatedAt := time.Now().UTC()
	for _, candidate := range candidates {
		projected, changed, err := r.markTaskChangeRegistrationCandidateUnlinkedTx(
			ctx, tx, taskID, candidate, associationRevision, updatedAt)
		if err != nil {
			return notifications, err
		}
		if changed {
			notifications = mergeTaskChangeRegistrationNotifications(notifications, projected)
		}
	}
	if err := tx.Commit(); err != nil {
		return notifications, err
	}
	return notifications, nil
}

type taskChangeRegistrationUnlinkCandidate struct {
	ID             string `db:"id"`
	OriginID       string `db:"origin_id"`
	SourceKind     string `db:"source_kind"`
	QueueSessionID string `db:"queue_session_id"`
	QueueEntryID   string `db:"queue_entry_id"`
	MessageID      string `db:"message_id"`
	SelectionID    string `db:"selection_id"`
	State          string `db:"state"`
}

func taskChangeRegistrationUnlinkCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, workspaceID, provider, associationID, providerHost, connectionScope, providerRepositoryID, changeRequestID string,
	associationRevision int64,
) ([]taskChangeRegistrationUnlinkCandidate, error) {
	var candidates []taskChangeRegistrationUnlinkCandidate
	err := tx.SelectContext(ctx, &candidates, tx.Rebind(`
		SELECT id, origin_id, source_kind, queue_session_id, queue_entry_id, message_id, selection_id, state
		FROM task_change_registration_intents
		WHERE task_id = ? AND workspace_id = ? AND provider = ? AND selection_id <> '' AND origin_id <> '' AND (
			(operation_receipt = ? AND state = 'succeeded' AND association_revision < ?)
			OR (provider_host = ? AND connection_scope = ? AND provider_repository_id = ? AND change_request_id = ?
				AND state IN ('pending', 'retryable', 'uncertain', 'failed', 'suppressed', 'succeeded')
				AND COALESCE(association_revision, expected_revision, 0) < ?)
		  )`),
		taskID, workspaceID, provider, associationID, associationRevision,
		providerHost, connectionScope, providerRepositoryID, changeRequestID, associationRevision)
	return candidates, err
}

func (r *Repository) markTaskChangeRegistrationCandidateUnlinkedTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	candidate taskChangeRegistrationUnlinkCandidate,
	associationRevision int64,
	updatedAt time.Time,
) (TaskChangeRegistrationNotifications, bool, error) {
	var notifications TaskChangeRegistrationNotifications
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_change_registration_intents
		SET state = 'unlinked', error_code = 'unlinked', association_revision = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND state = ? AND COALESCE(association_revision, expected_revision, 0) < ?`),
		associationRevision, updatedAt, updatedAt, candidate.ID, candidate.State, associationRevision)
	if err != nil {
		return notifications, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return notifications, false, err
	}
	projection := &TaskChangeRegistrationIntent{
		TaskID: taskID, SourceKind: candidate.SourceKind, OriginID: candidate.OriginID,
		QueueSessionID: candidate.QueueSessionID, QueueEntryID: candidate.QueueEntryID,
		MessageID: candidate.MessageID, SelectionID: candidate.SelectionID,
	}
	projected, err := r.ProjectTaskChangeRegistrationStatusTx(
		ctx, tx, projection, taskChangeRegistrationStateUnlinked, taskChangeRegistrationStateUnlinked,
	)
	return projected, true, err
}

func mergeTaskChangeRegistrationNotifications(
	current, added TaskChangeRegistrationNotifications,
) TaskChangeRegistrationNotifications {
	current.MessageIDs = appendMissingRegistrationIDs(current.MessageIDs, added.MessageIDs)
	current.QueueSessionIDs = appendMissingRegistrationIDs(current.QueueSessionIDs, added.QueueSessionIDs)
	return current
}

func appendMissingRegistrationIDs(current, additions []string) []string {
	seen := make(map[string]struct{}, len(current)+len(additions))
	for _, value := range current {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, exists := seen[value]; exists {
			continue
		}
		current = append(current, value)
		seen[value] = struct{}{}
	}
	return current
}

func messageRegistrationStatus(state string) (apiv1.EntityReferenceRegistrationStatus, error) {
	switch state {
	case cleanupPhasePending, taskChangeRegistrationStateRetryable:
		return apiv1.EntityReferenceRegistrationPending, nil
	case taskChangeRegistrationStateSucceeded:
		return apiv1.EntityReferenceRegistrationLinked, nil
	case taskChangeRegistrationStateFailed:
		return apiv1.EntityReferenceRegistrationFailed, nil
	case taskChangeRegistrationStateSuppressed:
		return apiv1.EntityReferenceRegistrationSuppressed, nil
	case taskChangeRegistrationStateUnlinked:
		return apiv1.EntityReferenceRegistrationUnlinked, nil
	case taskChangeRegistrationStateUncertain:
		return apiv1.EntityReferenceRegistrationUncertain, nil
	default:
		return "", errors.New("message registration state is invalid")
	}
}

func getTaskChangeRegistrationIntentByConflictKey(
	ctx context.Context,
	tx *sqlx.Tx,
	intent *TaskChangeRegistrationIntent,
) (*TaskChangeRegistrationIntent, error) {
	var existing TaskChangeRegistrationIntent
	queries := []struct {
		query string
		args  []any
	}{}
	if intent.OriginID != "" {
		queries = append(queries, struct {
			query string
			args  []any
		}{`SELECT * FROM task_change_registration_intents WHERE task_id = ? AND source_kind = ? AND origin_id = ? AND source_revision = ? AND reference_ref = ? LIMIT 1`,
			[]any{intent.TaskID, intent.SourceKind, intent.OriginID, intent.SourceRevision, intent.ReferenceRef}})
	}
	if intent.SelectionID != "" {
		queries = append(queries, struct {
			query string
			args  []any
		}{`SELECT * FROM task_change_registration_intents WHERE task_id = ? AND selection_id = ? LIMIT 1`,
			[]any{intent.TaskID, intent.SelectionID}})
	}
	if intent.OperationID != "" {
		queries = append(queries, struct {
			query string
			args  []any
		}{`SELECT * FROM task_change_registration_intents WHERE task_id = ? AND operation_id = ? LIMIT 1`,
			[]any{intent.TaskID, intent.OperationID}})
	}
	queries = append(queries, struct {
		query string
		args  []any
	}{`SELECT * FROM task_change_registration_intents WHERE id = ? LIMIT 1`, []any{intent.ID}})
	for _, candidate := range queries {
		err := tx.GetContext(ctx, &existing, tx.Rebind(candidate.query), candidate.args...)
		if err == nil {
			return &existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, sql.ErrNoRows
}

func sameTaskChangeRegistrationIntent(existing, requested *TaskChangeRegistrationIntent) bool {
	return existing != nil && requested != nil && sameRegistrationOrigin(existing, requested) &&
		sameRegistrationTarget(existing, requested) && sameRegistrationOperation(existing, requested)
}

func sameRegistrationOrigin(existing, requested *TaskChangeRegistrationIntent) bool {
	return existing.TaskID == requested.TaskID && existing.WorkspaceID == requested.WorkspaceID &&
		existing.ActorID == requested.ActorID && existing.SourceKind == requested.SourceKind &&
		existing.OriginID == requested.OriginID && existing.QueueSessionID == requested.QueueSessionID &&
		existing.QueueEntryID == requested.QueueEntryID && existing.MessageID == requested.MessageID &&
		existing.SourceRevision == requested.SourceRevision && existing.SelectionID == requested.SelectionID &&
		existing.ReferenceRef == requested.ReferenceRef && existing.ReferenceSnapshot == requested.ReferenceSnapshot
}

func sameRegistrationTarget(existing, requested *TaskChangeRegistrationIntent) bool {
	return existing.Provider == requested.Provider && existing.ProviderHost == requested.ProviderHost &&
		existing.RepositoryID == requested.RepositoryID && existing.ConnectionScope == requested.ConnectionScope &&
		existing.ProviderRepositoryID == requested.ProviderRepositoryID &&
		existing.ChangeRequestID == requested.ChangeRequestID
}

func sameRegistrationOperation(existing, requested *TaskChangeRegistrationIntent) bool {
	return existing.OperationID == requested.OperationID &&
		existing.OperationFingerprint == requested.OperationFingerprint &&
		existing.InstallationIdentity == requested.InstallationIdentity &&
		equalIntPointers(existing.ContractVersion, requested.ContractVersion) &&
		equalInt64Pointers(existing.ExpectedRevision, requested.ExpectedRevision)
}

func equalIntPointers(left, right *int) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func validTaskChangeRegistrationState(state string) bool {
	switch state {
	case cleanupPhasePending, taskChangeRegistrationStateRetryable, taskChangeRegistrationStateSucceeded, taskChangeRegistrationStateFailed,
		taskChangeRegistrationStateSuppressed, taskChangeRegistrationStateUncertain, taskChangeRegistrationStateUnlinked:
		return true
	default:
		return false
	}
}

func boundedRegistrationError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}
