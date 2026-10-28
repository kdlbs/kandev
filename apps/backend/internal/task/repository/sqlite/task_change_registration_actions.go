package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type taskChangeRegistrationActionReceipt struct {
	TaskID      string `db:"task_id"`
	OperationID string `db:"operation_id"`
	RequestedID string `db:"requested_id"`
	IntentID    string `db:"intent_id"`
	ActorID     string `db:"actor_id"`
	Action      string `db:"action"`
}

// ActTaskChangeRegistration applies one idempotent retry or explicit relink
// action. A relink gets a fresh selection and operation identity while keeping
// the accepted message as its origin.
func (r *Repository) ActTaskChangeRegistration(
	ctx context.Context,
	taskID, registrationID, operationID, actorID, action string,
) (*TaskChangeRegistrationIntent, bool, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(registrationID) == "" ||
		!validTaskChangeRegistrationOperationID(operationID) ||
		(action != "retry" && action != "relink") {
		return nil, false, errors.New("task change registration action is incomplete")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if intent, replayed, err := replayTaskChangeRegistrationActionTx(
		ctx, tx, taskID, registrationID, operationID, actorID, action,
	); err != nil || replayed {
		return intent, replayed, err
	}
	requested, err := getTaskChangeRegistrationIntentByIdentifierTx(ctx, tx, taskID, registrationID)
	if err != nil {
		return nil, false, err
	}
	result, err := r.applyTaskChangeRegistrationActionTx(
		ctx, tx, taskID, registrationID, operationID, actorID, action, requested,
	)
	if err != nil {
		return nil, false, err
	}
	if err := recordTaskChangeRegistrationActionTx(ctx, tx, taskChangeRegistrationActionReceipt{
		TaskID: taskID, OperationID: operationID, RequestedID: registrationID,
		IntentID: result.ID, ActorID: actorID, Action: action,
	}); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return result, false, nil
}

func replayTaskChangeRegistrationActionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, registrationID, operationID, actorID, action string,
) (*TaskChangeRegistrationIntent, bool, error) {
	receipt, err := readTaskChangeRegistrationActionTx(ctx, tx, taskID, operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !sameTaskChangeRegistrationAction(receipt, registrationID, actorID, action) {
		return nil, false, ErrTaskChangeRegistrationOperationConflict
	}
	intent, err := getTaskChangeRegistrationIntentByIDTx(ctx, tx, taskID, receipt.IntentID)
	if err != nil {
		return nil, false, err
	}
	return intent, true, tx.Commit()
}

func (r *Repository) applyTaskChangeRegistrationActionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, registrationID, operationID, actorID, action string,
	requested *TaskChangeRegistrationIntent,
) (*TaskChangeRegistrationIntent, error) {
	switch action {
	case "retry":
		return r.retryTaskChangeRegistrationTx(ctx, tx, taskID, requested)
	case "relink":
		return r.relinkTaskChangeRegistrationTx(
			ctx, tx, taskID, registrationID, operationID, actorID, requested,
		)
	default:
		return nil, ErrTaskChangeRegistrationNotRetryable
	}
}

func (r *Repository) retryTaskChangeRegistrationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	requested *TaskChangeRegistrationIntent,
) (*TaskChangeRegistrationIntent, error) {
	if !retryableTaskChangeRegistration(requested) {
		return nil, ErrTaskChangeRegistrationNotRetryable
	}
	updated, err := r.SetTaskChangeRegistrationIntentResultTx(ctx, tx, requested.ID, requested.State,
		taskChangeRegistrationStateRetryable, "", nil, requested.ExpectedRevision, nil, requested.OperationReceipt)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrTaskChangeRegistrationConflict
	}
	current, err := getTaskChangeRegistrationIntentByIDTx(ctx, tx, taskID, requested.ID)
	if err != nil {
		return nil, err
	}
	if _, err := r.ProjectTaskChangeRegistrationStatusTx(ctx, tx, current, taskChangeRegistrationStateRetryable, ""); err != nil {
		return nil, err
	}
	return current, nil
}

func (r *Repository) relinkTaskChangeRegistrationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, registrationID, operationID, actorID string,
	requested *TaskChangeRegistrationIntent,
) (*TaskChangeRegistrationIntent, error) {
	messageID := registrationIntentMessageID(requested)
	if requested.State != "unlinked" || requested.AssociationRevision == nil ||
		messageID == "" || requested.SelectionID == "" || requested.OriginID == "" {
		return nil, ErrTaskChangeRegistrationNotRetryable
	}
	reference, err := relinkedRegistrationReference(requested.ReferenceSnapshot, operationID)
	if err != nil {
		return nil, ErrTaskChangeRegistrationNotRetryable
	}
	snapshot, err := json.Marshal(reference)
	if err != nil {
		return nil, err
	}
	expectedRevision := *requested.AssociationRevision
	fingerprint := registrationActionFingerprint(taskID, registrationID, operationID, "relink", requested.ReferenceRef)
	created := &TaskChangeRegistrationIntent{
		ID: uuid.NewString(), TaskID: requested.TaskID, WorkspaceID: requested.WorkspaceID,
		ActorID: actorID, SourceKind: requested.SourceKind, OriginID: requested.OriginID,
		QueueSessionID: requested.QueueSessionID, QueueEntryID: requested.QueueEntryID,
		MessageID: messageID, SourceRevision: operationID, SelectionID: operationID, ReferenceRef: requested.ReferenceRef,
		ReferenceSnapshot: string(snapshot), Provider: requested.Provider, ProviderHost: requested.ProviderHost,
		RepositoryID: requested.RepositoryID, ConnectionScope: requested.ConnectionScope,
		ProviderRepositoryID: requested.ProviderRepositoryID, ChangeRequestID: requested.ChangeRequestID,
		ExpectedRevision: &expectedRevision, OperationID: operationID, OperationFingerprint: fingerprint,
		InstallationIdentity: requested.InstallationIdentity, ContractVersion: requested.ContractVersion,
		State: cleanupPhasePending,
	}
	result, _, err := r.CreateTaskChangeRegistrationIntentTx(ctx, tx, created)
	if err != nil {
		return nil, err
	}
	if err := r.replaceMessageRegistrationSelectionTx(ctx, tx, messageID, requested.SelectionID, reference); err != nil {
		return nil, err
	}
	return result, nil
}

func relinkedRegistrationReference(snapshot, operationID string) (apiv1.EntityReference, error) {
	var reference apiv1.EntityReference
	if err := json.Unmarshal([]byte(snapshot), &reference); err != nil || reference.Ref == "" {
		return reference, ErrTaskChangeRegistrationNotRetryable
	}
	reference.SelectionID = operationID
	reference.AssociationMode = apiv1.EntityReferenceModeLinkOnSend
	reference.RegistrationStatus = apiv1.EntityReferenceRegistrationPending
	reference.RegistrationErrorCode = ""
	return reference, nil
}

func retryableTaskChangeRegistration(intent *TaskChangeRegistrationIntent) bool {
	return intent != nil && intent.ExpectedRevision != nil && intent.RepositoryID != "" &&
		intent.ProviderRepositoryID != "" && (intent.State == "failed" || intent.State == "uncertain") &&
		intent.ErrorCode != "identity_unresolved" && intent.ErrorCode != "reference_invalid" &&
		intent.ErrorCode != "repository_changed" && intent.ErrorCode != "origin_unavailable"
}

func registrationIntentMessageID(intent *TaskChangeRegistrationIntent) string {
	if intent == nil {
		return ""
	}
	if intent.MessageID != "" {
		return intent.MessageID
	}
	if intent.SourceKind == TaskChangeRegistrationSourceMessage {
		return intent.OriginID
	}
	return ""
}

func validTaskChangeRegistrationOperationID(operationID string) bool {
	return operationID != "" && len(operationID) <= 128 && strings.TrimSpace(operationID) == operationID
}

func registrationActionFingerprint(taskID, registrationID, operationID, action, referenceRef string) string {
	fingerprint := sha256.Sum256([]byte(strings.Join([]string{
		taskID, registrationID, operationID, action, referenceRef,
	}, "\x00")))
	return hex.EncodeToString(fingerprint[:])
}

func sameTaskChangeRegistrationAction(
	receipt *taskChangeRegistrationActionReceipt,
	requestedID, actorID, action string,
) bool {
	return receipt != nil && receipt.RequestedID == requestedID && receipt.ActorID == actorID && receipt.Action == action
}

func readTaskChangeRegistrationActionTx(
	ctx context.Context, tx *sqlx.Tx, taskID, operationID string,
) (*taskChangeRegistrationActionReceipt, error) {
	var receipt taskChangeRegistrationActionReceipt
	err := tx.GetContext(ctx, &receipt, tx.Rebind(`
		SELECT task_id, operation_id, requested_id, intent_id, actor_id, action
		FROM task_change_registration_actions WHERE task_id = ? AND operation_id = ?`), taskID, operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &receipt, err
}

func recordTaskChangeRegistrationActionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	receipt taskChangeRegistrationActionReceipt,
) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO task_change_registration_actions (task_id, operation_id, requested_id, intent_id, actor_id, action, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(task_id, operation_id) DO NOTHING`),
		receipt.TaskID, receipt.OperationID, receipt.RequestedID, receipt.IntentID,
		receipt.ActorID, receipt.Action, time.Now().UTC()); err != nil {
		return err
	}
	stored, err := readTaskChangeRegistrationActionTx(ctx, tx, receipt.TaskID, receipt.OperationID)
	if err != nil {
		return err
	}
	if stored.RequestedID != receipt.RequestedID || stored.IntentID != receipt.IntentID ||
		stored.ActorID != receipt.ActorID || stored.Action != receipt.Action {
		return ErrTaskChangeRegistrationOperationConflict
	}
	return nil
}

func getTaskChangeRegistrationIntentByIdentifierTx(
	ctx context.Context, tx *sqlx.Tx, taskID, identifier string,
) (*TaskChangeRegistrationIntent, error) {
	var intent TaskChangeRegistrationIntent
	err := tx.GetContext(ctx, &intent, tx.Rebind(`
		SELECT * FROM task_change_registration_intents
		WHERE task_id = ? AND (id = ? OR selection_id = ?) LIMIT 1`), taskID, identifier, identifier)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &intent, err
}

func getTaskChangeRegistrationIntentByIDTx(
	ctx context.Context, tx *sqlx.Tx, taskID, intentID string,
) (*TaskChangeRegistrationIntent, error) {
	var intent TaskChangeRegistrationIntent
	err := tx.GetContext(ctx, &intent, tx.Rebind(`
		SELECT * FROM task_change_registration_intents WHERE task_id = ? AND id = ?`), taskID, intentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &intent, err
}

func (r *Repository) replaceMessageRegistrationSelectionTx(
	ctx context.Context, tx *sqlx.Tx, messageID, previousSelectionID string, replacement apiv1.EntityReference,
) error {
	if tx == nil || messageID == "" || previousSelectionID == "" || replacement.SelectionID == "" {
		return errors.New("message registration replacement identity is incomplete")
	}
	metadata, messageFound, err := r.readMessageRegistrationMetadataTx(ctx, tx, messageID)
	if err != nil {
		return err
	}
	if !messageFound {
		return nil
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(metadata["entity_references"], &references); err != nil {
		return err
	}
	found := false
	for index := range references {
		if references[index].SelectionID == previousSelectionID || references[index].SelectionID == replacement.SelectionID {
			references[index] = replacement
			found = true
			break
		}
	}
	if !found {
		return ErrTaskChangeRegistrationConflict
	}
	encodedReferences, err := json.Marshal(references)
	if err != nil {
		return err
	}
	metadata["entity_references"] = encodedReferences
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_session_messages SET metadata = ?, updated_at = ? WHERE id = ?`),
		string(encodedMetadata), time.Now().UTC(), messageID); err != nil {
		return fmt.Errorf("update relinked message reference: %w", err)
	}
	return nil
}
