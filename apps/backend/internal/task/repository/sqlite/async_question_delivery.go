package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ListAsyncQuestionDeliveries(ctx context.Context, now time.Time, limit int) ([]*models.AsyncQuestionDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT admission_id, question_id, outcome_version, kind, destination_task_id,
		       destination_session_id, destination_incarnation_id, content, metadata_json, state, attempts,
		       next_attempt_at, last_error, created_at, admitted_at
		  FROM task_async_question_deliveries
		 WHERE state = 'pending' AND next_attempt_at <= ?
		 ORDER BY created_at ASC, admission_id ASC
		 LIMIT ?`), now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAsyncQuestionDeliveryRows(rows, limit)
}

func (r *Repository) ListAsyncQuestionDeliveriesForQuestion(
	ctx context.Context,
	questionID string,
	kind models.AsyncQuestionDeliveryKind,
) ([]*models.AsyncQuestionDelivery, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT admission_id, question_id, outcome_version, kind, destination_task_id,
		       destination_session_id, destination_incarnation_id, content, metadata_json, state, attempts,
		       next_attempt_at, last_error, created_at, admitted_at
		  FROM task_async_question_deliveries
		 WHERE question_id = ? AND kind = ?
		 ORDER BY created_at ASC, admission_id ASC`), questionID, string(kind))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAsyncQuestionDeliveryRows(rows, 0)
}

func scanAsyncQuestionDeliveryRows(rows *sqlx.Rows, capacity int) ([]*models.AsyncQuestionDelivery, error) {
	deliveries := make([]*models.AsyncQuestionDelivery, 0, capacity)
	for rows.Next() {
		var (
			delivery     models.AsyncQuestionDelivery
			kind         string
			state        string
			metadataJSON string
			admittedAt   sql.NullTime
		)
		if err := rows.Scan(&delivery.AdmissionID, &delivery.QuestionID, &delivery.OutcomeVersion,
			&kind, &delivery.DestinationTaskID, &delivery.DestinationSession, &delivery.DestinationIncarnationID,
			&delivery.Content, &metadataJSON, &state, &delivery.Attempts, &delivery.NextAttemptAt, &delivery.LastError,
			&delivery.CreatedAt, &admittedAt); err != nil {
			return nil, err
		}
		delivery.Kind = models.AsyncQuestionDeliveryKind(kind)
		delivery.State = models.AsyncQuestionDeliveryState(state)
		if admittedAt.Valid {
			value := admittedAt.Time
			delivery.AdmittedAt = &value
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			_ = json.Unmarshal([]byte(metadataJSON), &delivery.Metadata)
		}
		deliveries = append(deliveries, &delivery)
	}
	return deliveries, rows.Err()
}

// CreateAsyncQuestionDelivery persists one additional outbox intent (a wake
// released by withdrawal). Admission identity remains caller-owned and stable.
func (r *Repository) CreateAsyncQuestionDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) error {
	if delivery == nil || delivery.AdmissionID == "" || delivery.QuestionID == "" {
		return errors.New("async question delivery identity is incomplete")
	}
	if delivery.State == "" {
		delivery.State = models.AsyncQuestionDeliveryPending
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = r.nowUTC()
	}
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = delivery.CreatedAt
	}
	metadataJSON, err := json.Marshal(delivery.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_async_question_deliveries
			(admission_id, question_id, outcome_version, kind, destination_task_id,
			 destination_session_id, content, metadata_json, state, attempts,
			 next_attempt_at, last_error, created_at, admitted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT (admission_id) DO NOTHING`),
		delivery.AdmissionID, delivery.QuestionID, delivery.OutcomeVersion,
		string(delivery.Kind), delivery.DestinationTaskID, delivery.DestinationSession,
		delivery.Content, string(metadataJSON), string(delivery.State), delivery.Attempts,
		delivery.NextAttemptAt, delivery.LastError, delivery.CreatedAt,
	)
	return err
}

// MarkAsyncQuestionDeliveryAdmitted records the exact queue admission receipt.
func (r *Repository) MarkAsyncQuestionDeliveryAdmitted(ctx context.Context, admissionID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_deliveries
		   SET state = 'admitted', admitted_at = ?, last_error = ''
		 WHERE admission_id = ?`), at, admissionID)
	return err
}

// MarkAsyncQuestionDeliveryFailed records a retryable admission failure.
func (r *Repository) MarkAsyncQuestionDeliveryFailed(ctx context.Context, admissionID string, attempts int, nextAttemptAt time.Time, message string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_deliveries
		   SET attempts = ?, next_attempt_at = ?, last_error = ?
		 WHERE admission_id = ?`), attempts, nextAttemptAt, message, admissionID)
	return err
}

// MarkAsyncQuestionDeliveryAbandoned stops retries for an intent whose
// destination can no longer accept an answer.
func (r *Repository) MarkAsyncQuestionDeliveryAbandoned(ctx context.Context, admissionID, message string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_deliveries
		   SET state = 'abandoned', last_error = ?
		 WHERE admission_id = ?`), message, admissionID)
	return err
}

// CreateAsyncQuestionWait persists one explicit dependency gate and its
// dependency set. At most one active gate per session is allowed.
