package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (r *sqliteRepository) AsyncQuestionAdmissionDisposition(
	ctx context.Context,
	clientQueueID, questionID string,
) (AsyncQuestionAdmissionState, time.Time, error) {
	identity, acceptedID, receiptMessage, admittedAt, found, err := r.readAsyncQuestionAdmissionReceipt(ctx, clientQueueID, questionID)
	if err != nil {
		return "", time.Time{}, err
	}
	if !found {
		return AsyncQuestionAdmissionMissing, time.Time{}, nil
	}
	if !asyncQuestionQueueMessageMatches(receiptMessage, identity, acceptedID, clientQueueID, questionID) {
		return "", time.Time{}, ErrQueueIDConflict
	}

	row := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		  FROM queued_messages
		 WHERE id = ? AND task_id = ? AND session_id = ?
	`), acceptedID, identity.TaskID, identity.SessionID)
	queued, metadataJSON, err := scanQueuedRowWithMetadataJSON(row)
	if err == nil {
		if !asyncQuestionQueueMessageMatches(queued, identity, acceptedID, clientQueueID, questionID) {
			return "", time.Time{}, ErrQueueIDConflict
		}
		reserved, reserveErr := isReservedMetadataJSON(metadataJSON)
		if reserveErr != nil {
			return "", time.Time{}, reserveErr
		}
		if reserved {
			return AsyncQuestionAdmissionUncertain, admittedAt, nil
		}
		return AsyncQuestionAdmissionQueued, admittedAt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, fmt.Errorf("inspect async question queue row: %w", err)
	}

	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return "", time.Time{}, err
	}
	var accepted bool
	var messageJSON string
	err = r.db.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT accepted, message_json
		  FROM queue_dispatch_claims
		 WHERE entry_id = ? AND session_id = ?
	`), acceptedID, identity.SessionID).Scan(&accepted, &messageJSON)
	if err == nil {
		var dispatch QueuedMessage
		if err := json.Unmarshal([]byte(messageJSON), &dispatch); err != nil {
			return "", time.Time{}, fmt.Errorf("decode async question dispatch claim: %w", err)
		}
		if !asyncQuestionQueueMessageMatches(&dispatch, identity, acceptedID, clientQueueID, questionID) {
			return "", time.Time{}, ErrQueueIDConflict
		}
		if accepted {
			return AsyncQuestionAdmissionAccepted, admittedAt, nil
		}
		return AsyncQuestionAdmissionUncertain, admittedAt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, fmt.Errorf("inspect async question dispatch claim: %w", err)
	}
	return AsyncQuestionAdmissionSettled, admittedAt, nil
}

func (r *sqliteRepository) AsyncQuestionAdmissionIDForQueueMessage(
	ctx context.Context,
	message *QueuedMessage,
) (string, bool, error) {
	questionID := asyncQuestionID(message)
	candidate, found, err := r.asyncQuestionCandidateForAcceptedID(ctx, message)
	if err != nil || !found {
		return "", found, err
	}
	if questionID != candidate.QuestionID {
		return "", false, nil
	}
	return candidate.ClientQueueID, true, nil
}

func (r *sqliteRepository) ListAsyncQuestionAdmissionCandidates(
	ctx context.Context,
) ([]AsyncQuestionAdmissionCandidate, error) {
	messages, err := r.listAsyncQuestionQueueMessages(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(messages))
	candidates := make([]AsyncQuestionAdmissionCandidate, 0, len(messages))
	for i := range messages {
		if err := r.appendAsyncQuestionAdmissionCandidate(ctx, &messages[i], seen, &candidates); err != nil {
			return nil, err
		}
	}
	dispatches, err := r.ListPendingQueueDispatches(ctx)
	if err != nil {
		return nil, err
	}
	for i := range dispatches {
		dispatch := &dispatches[i]
		if dispatch.Accepted || !isAsyncQuestionQueueMessage(&dispatch.Message) {
			continue
		}
		if err := r.appendAsyncQuestionAdmissionCandidate(ctx, &dispatch.Message, seen, &candidates); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

func (r *sqliteRepository) listAsyncQuestionQueueMessages(ctx context.Context) ([]QueuedMessage, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		  FROM queued_messages
		 WHERE queued_by = ?
	`), QueuedByAsyncQuestion)
	if err != nil {
		return nil, fmt.Errorf("list async question queue rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	messages := make([]QueuedMessage, 0)
	for rows.Next() {
		message, scanErr := scanQueuedRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if isAsyncQuestionQueueMessage(message) {
			messages = append(messages, *message)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *sqliteRepository) appendAsyncQuestionAdmissionCandidate(
	ctx context.Context,
	message *QueuedMessage,
	seen map[string]struct{},
	candidates *[]AsyncQuestionAdmissionCandidate,
) error {
	candidate, found, err := r.asyncQuestionCandidateForAcceptedID(ctx, message)
	if err != nil || !found {
		return err
	}
	if _, ok := seen[candidate.ClientQueueID]; ok {
		return nil
	}
	seen[candidate.ClientQueueID] = struct{}{}
	*candidates = append(*candidates, candidate)
	return nil
}

func (r *sqliteRepository) asyncQuestionCandidateForAcceptedID(
	ctx context.Context,
	message *QueuedMessage,
) (AsyncQuestionAdmissionCandidate, bool, error) {
	if !isAsyncQuestionQueueMessage(message) {
		return AsyncQuestionAdmissionCandidate{}, false, nil
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT client_queue_id, session_incarnation_id, response_json, admitted_at
		  FROM queue_admission_receipts
		 WHERE task_id = ? AND session_id = ? AND accepted_queue_id = ?
		 ORDER BY session_incarnation_id
	`), message.TaskID, message.SessionID, message.ID)
	if err != nil {
		return AsyncQuestionAdmissionCandidate{}, false, err
	}
	defer func() { _ = rows.Close() }()
	var candidate AsyncQuestionAdmissionCandidate
	var incarnationID, responseJSON string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return AsyncQuestionAdmissionCandidate{}, false, err
		}
		return AsyncQuestionAdmissionCandidate{}, false, nil
	}
	if err := rows.Scan(&candidate.ClientQueueID, &incarnationID, &responseJSON, &candidate.AdmittedAt); err != nil {
		return AsyncQuestionAdmissionCandidate{}, false, err
	}
	if rows.Next() {
		return AsyncQuestionAdmissionCandidate{}, false, errors.New("async question accepted queue id has ambiguous receipts")
	}
	if err := rows.Err(); err != nil {
		return AsyncQuestionAdmissionCandidate{}, false, err
	}
	identity := QueueSessionIdentity{TaskID: message.TaskID, SessionID: message.SessionID, SessionIncarnationID: incarnationID}
	var receiptMessage QueuedMessage
	if err := json.Unmarshal([]byte(responseJSON), &receiptMessage); err != nil {
		return AsyncQuestionAdmissionCandidate{}, false, fmt.Errorf("decode async question candidate receipt: %w", err)
	}
	if !asyncQuestionQueueMessageMatches(&receiptMessage, identity, message.ID, candidate.ClientQueueID, asyncQuestionID(message)) ||
		!asyncQuestionQueueMessageMatches(message, identity, message.ID, candidate.ClientQueueID, asyncQuestionID(message)) {
		return AsyncQuestionAdmissionCandidate{}, false, nil
	}
	candidate.QuestionID = asyncQuestionID(message)
	return candidate, true, nil
}

func isAsyncQuestionQueueMessage(message *QueuedMessage) bool {
	return message != nil && message.QueuedBy == QueuedByAsyncQuestion && asyncQuestionID(message) != ""
}

func asyncQuestionID(message *QueuedMessage) string {
	if message == nil || message.Metadata == nil {
		return ""
	}
	questionID, _ := message.Metadata[MetadataAsyncQuestion].(string)
	return questionID
}
