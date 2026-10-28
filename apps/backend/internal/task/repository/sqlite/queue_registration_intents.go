package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	TaskChangeRegistrationSourceMessage        = "message"
	TaskChangeRegistrationSourceQueueAdmission = "queue_admission"
	TaskChangeRegistrationSourceQueueEdit      = "queue_edit"
	registrationOriginsMetadataKey             = "registration_origin_ids"
)

// CreateQueuedTaskChangeRegistrationIntentsTx records selected references in
// the same transaction as an accepted queue row and its admission receipt.
func (r *Repository) CreateQueuedTaskChangeRegistrationIntentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, entryID, admissionID, actorID string,
	candidateReferences []apiv1.EntityReference,
) ([]apiv1.EntityReference, []string, error) {
	return r.createQueueRegistrationIntentsTx(
		ctx, tx, TaskChangeRegistrationSourceQueueAdmission, taskID, sessionID, entryID,
		admissionID, actorID, candidateReferences,
	)
}

// CreateQueueEditTaskChangeRegistrationIntentsTx records only new or relinked
// selections after the queue row's lease-guarded save has passed validation.
func (r *Repository) CreateQueueEditTaskChangeRegistrationIntentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, entryID, operationID, actorID string,
	previousReferences, updatedReferences []apiv1.EntityReference,
) ([]apiv1.EntityReference, []string, error) {
	previousBySelection := make(map[string]apiv1.EntityReference, len(previousReferences))
	for _, reference := range previousReferences {
		previousBySelection[reference.SelectionID] = reference
	}
	newSelections := make([]apiv1.EntityReference, 0, len(updatedReferences))
	for index, reference := range updatedReferences {
		if reference.AssociationMode != apiv1.EntityReferenceModeLinkOnSend {
			updatedReferences[index].RegistrationStatus = ""
			updatedReferences[index].RegistrationErrorCode = ""
			continue
		}
		previous, unchanged := previousBySelection[reference.SelectionID]
		if unchanged && previous.Ref == reference.Ref && previous.AssociationMode == apiv1.EntityReferenceModeLinkOnSend {
			updatedReferences[index].RegistrationStatus = previous.RegistrationStatus
			updatedReferences[index].RegistrationErrorCode = previous.RegistrationErrorCode
			continue
		}
		updatedReferences[index].RegistrationStatus = apiv1.EntityReferenceRegistrationPending
		updatedReferences[index].RegistrationErrorCode = ""
		newSelections = append(newSelections, reference)
	}
	metadata, err := r.readQueuedRegistrationMetadataTx(ctx, tx, taskID, sessionID, entryID)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(updatedReferences)
	if err != nil {
		return nil, nil, err
	}
	metadata["entity_references"] = encoded
	if err := r.writeQueuedRegistrationMetadataTx(ctx, tx, taskID, sessionID, entryID, metadata); err != nil {
		return nil, nil, err
	}
	if len(newSelections) == 0 {
		return updatedReferences, decodeRegistrationOriginIDs(metadata[registrationOriginsMetadataKey]), nil
	}
	return r.createQueueRegistrationIntentsTx(
		ctx, tx, TaskChangeRegistrationSourceQueueEdit, taskID, sessionID, entryID,
		operationID, actorID, newSelections,
	)
}

func (r *Repository) createQueueRegistrationIntentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sourceKind, taskID, sessionID, entryID, originID, actorID string,
	selectedReferences []apiv1.EntityReference,
) ([]apiv1.EntityReference, []string, error) {
	if err := validateQueueRegistrationIdentity(tx, taskID, sessionID, entryID, originID, actorID); err != nil {
		return nil, nil, err
	}
	queueMetadata, err := r.readQueuedRegistrationMetadataTx(ctx, tx, taskID, sessionID, entryID)
	if err != nil {
		return nil, nil, err
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(queueMetadata["entity_references"], &references); err != nil {
		return nil, nil, fmt.Errorf("decode accepted queue references: %w", err)
	}
	selected, err := entityrefs.NormalizeForSubmission(selectedReferences)
	if err != nil {
		return nil, nil, fmt.Errorf("validate queue registration references: %w", err)
	}
	workspaceID, err := r.taskWorkspaceIDTx(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	originIDs := decodeRegistrationOriginIDs(queueMetadata[registrationOriginsMetadataKey])
	if !containsString(originIDs, originID) {
		originIDs = append(originIDs, originID)
	}
	for _, reference := range selected {
		if reference.AssociationMode != apiv1.EntityReferenceModeLinkOnSend {
			continue
		}
		if err := r.createQueueRegistrationIntentForReferenceTx(
			ctx, tx, taskID, workspaceID, sessionID, entryID, originID, sourceKind, actorID, reference, references,
		); err != nil {
			return nil, nil, err
		}
	}
	encodedReferences, err := json.Marshal(references)
	if err != nil {
		return nil, nil, fmt.Errorf("encode accepted queue references: %w", err)
	}
	encodedOriginIDs, err := json.Marshal(originIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("encode queue registration origin IDs: %w", err)
	}
	queueMetadata["entity_references"] = encodedReferences
	queueMetadata[registrationOriginsMetadataKey] = encodedOriginIDs
	if err := r.writeQueuedRegistrationMetadataTx(ctx, tx, taskID, sessionID, entryID, queueMetadata); err != nil {
		return nil, nil, err
	}
	return references, originIDs, nil
}

func validateQueueRegistrationIdentity(tx *sqlx.Tx, taskID, sessionID, entryID, originID, actorID string) error {
	if tx == nil || strings.TrimSpace(taskID) == "" || strings.TrimSpace(sessionID) == "" ||
		strings.TrimSpace(entryID) == "" || strings.TrimSpace(originID) == "" || strings.TrimSpace(actorID) == "" {
		return fmt.Errorf("queue registration identity is incomplete")
	}
	return nil
}

func (r *Repository) createQueueRegistrationIntentForReferenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, workspaceID, sessionID, entryID, originID, sourceKind, actorID string,
	reference apiv1.EntityReference,
	references []apiv1.EntityReference,
) error {
	if reference.SelectionID == "" {
		return fmt.Errorf("queue registration selection identity is missing")
	}
	for index := range references {
		if references[index].SelectionID == reference.SelectionID {
			references[index].RegistrationStatus = apiv1.EntityReferenceRegistrationPending
			references[index].RegistrationErrorCode = ""
		}
	}
	intent, err := r.newQueueRegistrationIntent(
		ctx, tx, taskID, workspaceID, sessionID, entryID, originID, sourceKind, actorID, reference,
	)
	if err != nil {
		return err
	}
	if _, _, err := r.CreateTaskChangeRegistrationIntentTx(ctx, tx, intent); err != nil {
		return err
	}
	return nil
}

func (r *Repository) newQueueRegistrationIntent(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, workspaceID, sessionID, entryID, originID, sourceKind, actorID string,
	reference apiv1.EntityReference,
) (*TaskChangeRegistrationIntent, error) {
	providerHost, err := registrationProviderHost(reference)
	if err != nil {
		return nil, err
	}
	identity := r.captureMessageRegistrationIdentityTx(ctx, tx, taskID, providerHost, reference)
	snapshot, err := json.Marshal(reference)
	if err != nil {
		return nil, fmt.Errorf("encode queue registration snapshot: %w", err)
	}
	operationID := registrationMessageOperationID(taskID, originID, reference.SelectionID)
	fingerprintData, err := json.Marshal(struct {
		TaskID      string `json:"task_id"`
		SessionID   string `json:"session_id"`
		EntryID     string `json:"entry_id"`
		OriginID    string `json:"origin_id"`
		SourceKind  string `json:"source_kind"`
		SelectionID string `json:"selection_id"`
		RefHash     string `json:"ref_hash"`
	}{taskID, sessionID, entryID, originID, sourceKind, reference.SelectionID, sha256Hex(snapshot)})
	if err != nil {
		return nil, fmt.Errorf("encode queue registration fingerprint: %w", err)
	}
	return &TaskChangeRegistrationIntent{
		TaskID: taskID, WorkspaceID: workspaceID, ActorID: actorID,
		SourceKind: sourceKind, OriginID: originID, QueueSessionID: sessionID, QueueEntryID: entryID,
		SourceRevision: "1", SelectionID: reference.SelectionID, ReferenceRef: reference.Ref,
		ReferenceSnapshot: string(snapshot), Provider: reference.Provider,
		ProviderHost: firstNonEmptyRegistrationValue(identity.providerHost, providerHost),
		RepositoryID: identity.repositoryID, ConnectionScope: identity.connectionScope,
		ProviderRepositoryID: identity.providerRepositoryID,
		ChangeRequestID:      strconv.FormatInt(identity.changeRequestID, 10),
		ExpectedRevision:     identity.expectedRevision, OperationID: operationID,
		OperationFingerprint: sha256Hex(fingerprintData), State: "pending",
	}, nil
}

func (r *Repository) readQueuedRegistrationMetadataTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, entryID string,
) (map[string]json.RawMessage, error) {
	query := `SELECT metadata_json FROM queued_messages WHERE task_id = ? AND session_id = ? AND id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var raw string
	if err := tx.GetContext(ctx, &raw, tx.Rebind(query), taskID, sessionID, entryID); err != nil {
		return nil, fmt.Errorf("read accepted queue registration metadata: %w", err)
	}
	metadata := make(map[string]json.RawMessage)
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return nil, fmt.Errorf("decode accepted queue registration metadata: %w", err)
		}
	}
	return metadata, nil
}

type queuedRegistrationRow struct {
	ID        string `db:"id"`
	SessionID string `db:"session_id"`
	Metadata  string `db:"metadata_json"`
}

func (r *Repository) findQueuedRegistrationRow(
	ctx context.Context, reader sqlx.QueryerContext, taskID, selectionID string,
) (*queuedRegistrationRow, error) {
	var rows []queuedRegistrationRow
	if err := sqlx.SelectContext(ctx, reader, &rows, r.db.Rebind(`
		SELECT id, session_id, metadata_json FROM queued_messages WHERE task_id = ? ORDER BY id`), taskID); err != nil {
		return nil, err
	}
	for _, row := range rows {
		var metadata struct {
			References []apiv1.EntityReference `json:"entity_references"`
		}
		if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
			return nil, err
		}
		for _, reference := range metadata.References {
			if reference.SelectionID == selectionID {
				return &row, nil
			}
		}
	}
	return nil, nil
}

func (r *Repository) setQueueRegistrationStatusTx(
	ctx context.Context, tx *sqlx.Tx, taskID, _ string, selectionID, state, errorCode string,
) (string, bool, error) {
	status, err := messageRegistrationStatus(state)
	if err != nil {
		return "", false, err
	}
	row, err := r.findQueuedRegistrationRow(ctx, tx, taskID, selectionID)
	if err != nil || row == nil {
		return "", false, err
	}
	if err := messagequeue.LockSessionInTransaction(ctx, tx, r.db, row.SessionID); err != nil {
		return "", false, err
	}
	current, err := r.findQueuedRegistrationRow(ctx, tx, taskID, selectionID)
	if err != nil || current == nil {
		return "", false, err
	}
	if current.SessionID != row.SessionID {
		return "", false, ErrTaskChangeRegistrationConflict
	}
	metadata, err := r.readQueuedRegistrationMetadataTx(ctx, tx, taskID, current.SessionID, current.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(metadata["entity_references"], &references); err != nil {
		return "", false, err
	}
	for index := range references {
		if references[index].SelectionID == selectionID {
			references[index].RegistrationStatus = status
			references[index].RegistrationErrorCode = boundedRegistrationError(errorCode)
		}
	}
	encoded, err := json.Marshal(references)
	if err != nil {
		return "", false, err
	}
	metadata["entity_references"] = encoded
	if err := r.writeQueuedRegistrationMetadataTx(ctx, tx, taskID, current.SessionID, current.ID, metadata); err != nil {
		return "", false, err
	}
	return current.SessionID, true, nil
}

func (r *Repository) writeQueuedRegistrationMetadataTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, entryID string,
	metadata map[string]json.RawMessage,
) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode queue registration metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE queued_messages SET metadata_json = ?
		WHERE task_id = ? AND session_id = ? AND id = ?`), string(encoded), taskID, sessionID, entryID)
	if err != nil {
		return fmt.Errorf("write queue registration metadata: %w", err)
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

func (r *Repository) taskWorkspaceIDTx(ctx context.Context, tx *sqlx.Tx, taskID string) (string, error) {
	var workspaceID string
	if err := tx.GetContext(ctx, &workspaceID, tx.Rebind(`SELECT workspace_id FROM tasks WHERE id = ?`), taskID); err != nil {
		return "", fmt.Errorf("resolve queue registration workspace: %w", err)
	}
	return workspaceID, nil
}

func registrationProviderHost(reference apiv1.EntityReference) (string, error) {
	parsed, err := url.Parse(reference.URL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("queue registration reference URL is invalid")
	}
	host := strings.ToLower(parsed.Host)
	if reference.Provider == messageRegistrationProviderGitHub {
		return "https://" + strings.ToLower(parsed.Hostname()), nil
	}
	return parsed.Scheme + "://" + host, nil
}

func decodeRegistrationOriginIDs(raw json.RawMessage) []string {
	var ids []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &ids)
	}
	return ids
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
