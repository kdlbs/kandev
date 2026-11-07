package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// closeUnansweredWithAdmission keeps the receipt lookup and metadata patch in
// one transaction. A receipt proves task/session admission, not the identity
// of the human who submitted it; legacy sender fields are client-supplied.
func (r *Repository) closeUnansweredWithAdmission(
	ctx context.Context, pendingID string, request models.CloseUnansweredClarification, patch []metadataPatchValue,
) ([]*models.Message, error) {
	// One stable snapshot keeps a queue-to-claim-to-transcript transition from
	// disappearing between receipt reads on PostgreSQL.
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	drv := r.db.DriverName()
	var taskID, sourceSessionID string
	query := "SELECT b.task_id,b.session_id FROM " + unansweredBundlesExpr(drv) + " b WHERE b.pending_id = ?"
	args := []any{pendingID}
	if request.SourceSessionID != "" {
		query += " AND b.session_id = ?"
		args = append(args, request.SourceSessionID)
	}
	query += " LIMIT 1"
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), args...).Scan(&taskID, &sourceSessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read unanswered identity: %w", err)
	}
	var targetTaskID string
	if err := tx.QueryRowContext(ctx, tx.Rebind("SELECT task_id FROM task_sessions WHERE id = ?"), request.SessionID).Scan(&targetTaskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, models.ErrUnansweredAdmissionUnknown
		}
		return nil, fmt.Errorf("read late answer session: %w", err)
	}
	if targetTaskID != taskID {
		return nil, models.ErrUnansweredAdmissionUnknown
	}
	if err := validateLateAnswerReceipt(ctx, tx, pendingID, taskID, sourceSessionID, request); err != nil {
		return nil, err
	}
	query = unansweredPatchQuery(drv, patch, "b.pending_id = ? AND b.task_id = ? AND b.session_id = ?", false) + `
		AND task_session_id = ?
		RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		          content, requests_input, type, metadata, created_at, updated_at`
	args = append(metadataPatchArgs(patch), r.nowUTC(), pendingID, taskID, sourceSessionID, sourceSessionID)
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("close admitted unanswered clarification: %w", err)
	}
	messages, _, scanErr := scanMessageRows(rows, 0)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(messages) == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return messages, nil
}

func validateLateAnswerReceipt(ctx context.Context, tx *sqlx.Tx, pendingID, taskID, sourceSessionID string, request models.CloseUnansweredClarification) error {
	// Older unbound receipts remain available for manual close.
	prefix := "late:" + pendingID + ":"
	if !strings.HasPrefix(request.MessageID, prefix) {
		return models.ErrUnansweredAdmissionUnknown
	}
	if _, err := uuid.Parse(strings.TrimPrefix(request.MessageID, prefix)); err != nil {
		return models.ErrUnansweredAdmissionUnknown
	}
	complete, err := lateAnswerBundleComplete(ctx, tx, pendingID, sourceSessionID)
	if err != nil {
		return err
	}
	if !complete {
		return models.ErrUnansweredAdmissionUnknown
	}
	accepted, err := hasLateAnswerAdmission(ctx, tx, taskID, request.SessionID, request.MessageID)
	if err != nil {
		return err
	}
	if !accepted {
		return models.ErrUnansweredAdmissionUnknown
	}
	accepted, err = lateAnswerAdmittedAfterQuestion(ctx, tx, pendingID, taskID, sourceSessionID, request.SessionID, request.MessageID)
	if err != nil {
		return err
	}
	if !accepted {
		return models.ErrUnansweredAdmissionUnknown
	}
	return nil
}

// A partially persisted bundle can be observed while its creator is still
// adding questions. Legacy rows with no declared total retain their old path.
func lateAnswerBundleComplete(ctx context.Context, tx *sqlx.Tx, pendingID, sourceSessionID string) (bool, error) {
	query := `SELECT COALESCE(metadata, '') FROM task_session_messages WHERE task_session_id = ? AND type = 'clarification_request' AND ` +
		dialect.JSONExtract(tx.DriverName(), "metadata", "pending_id") + ` = ?`
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), sourceSessionID, pendingID)
	if err != nil {
		return false, fmt.Errorf("read late answer bundle completeness: %w", err)
	}
	defer func() { _ = rows.Close() }()
	expected, missing := 0, false
	questions := make(map[string]bool)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		var metadata struct {
			Total      *int   `json:"question_total"`
			QuestionID string `json:"question_id"`
			Question   struct {
				ID string `json:"id"`
			} `json:"question"`
		}
		if json.Unmarshal([]byte(raw), &metadata) != nil {
			return false, nil
		}
		id := metadata.QuestionID
		if id == "" {
			id = metadata.Question.ID
		}
		if id == "" {
			return false, nil
		}
		questions[id] = true
		if metadata.Total == nil {
			missing = true
			continue
		}
		if *metadata.Total <= 0 || (expected != 0 && expected != *metadata.Total) {
			return false, nil
		}
		expected = *metadata.Total
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if expected == 0 {
		return true, nil
	}
	return !missing && len(questions) == expected, nil
}

// Identified queue admissions have their own durable time even when queue
// merging keeps multiple IDs on one row with only the newest row timestamp.
// Prefer that receipt over a later materialized transcript timestamp.
func lateAnswerAdmittedAfterQuestion(ctx context.Context, tx *sqlx.Tx, pendingID, taskID, sourceSessionID, sessionID, id string) (bool, error) {
	exists, err := internaldb.TableExistsContext(ctx, tx, "queue_admission_receipts")
	if err != nil {
		return false, err
	}
	bundles := unansweredBundlesExpr(tx.DriverName())
	if exists {
		var total, timely int
		query := `SELECT COUNT(*), COALESCE(SUM(CASE WHEN q.admitted_at >= b.last_created_at THEN 1 ELSE 0 END), 0)
			FROM queue_admission_receipts q JOIN ` + bundles + ` b ON b.pending_id = ? AND b.task_id = ? AND b.session_id = ?
			WHERE q.task_id = ? AND q.session_id = ? AND q.client_queue_id = ?`
		if err := tx.QueryRowContext(ctx, tx.Rebind(query), pendingID, taskID, sourceSessionID, taskID, sessionID, id).Scan(&total, &timely); err != nil {
			return false, fmt.Errorf("read late answer admission time: %w", err)
		}
		if total > 0 {
			return total == timely, nil
		}
	}
	// Direct message admissions have an exact durable message ID and timestamp.
	// Metadata aliases and legacy merged rows cannot establish their own time.
	query := `SELECT COALESCE(m.metadata, '') FROM task_session_messages m JOIN ` + bundles + ` b ON b.pending_id = ? AND b.task_id = ? AND b.session_id = ?
		WHERE m.task_id = ? AND m.task_session_id = ? AND m.id = ?
		AND m.author_type = 'user' AND m.type = 'message' AND m.created_at >= b.last_created_at`
	var raw string
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), pendingID, taskID, sourceSessionID, taskID, sessionID, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read late answer message time: %w", err)
	}
	if raw == "" {
		raw = "{}"
	}
	var metadata lateAnswerReceiptMetadata
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return false, nil
	}
	return metadata.ClientQueueID == "" && len(metadata.QueueAdmissionIDs) == 0 && len(metadata.SendNowSources) == 0, nil
}

type lateAnswerReceiptMetadata struct {
	ClientQueueID     string   `json:"client_queue_id"`
	QueueAdmissionIDs []string `json:"queue_admission_ids"`
	DeliveryStatus    string   `json:"delivery_status"`
	SendNowSources    []struct {
		ID       string `json:"id"`
		TaskID   string `json:"task_id"`
		QueuedBy string `json:"queued_by"`
		Metadata struct {
			ClientQueueID     string   `json:"client_queue_id"`
			QueueAdmissionIDs []string `json:"queue_admission_ids"`
		} `json:"metadata"`
	} `json:"send_now_sources"`
}

func receiptContainsID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func receiptMetadataMatches(raw, taskID, id string) bool {
	var metadata lateAnswerReceiptMetadata
	if raw == "" {
		raw = "{}"
	}
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return false
	}
	if metadata.DeliveryStatus == models.MessageDeliveryStatusBlocked || metadata.DeliveryStatus == models.MessageDeliveryStatusNotDelivered {
		return false
	}
	if metadata.ClientQueueID == id || receiptContainsID(metadata.QueueAdmissionIDs, id) {
		return true
	}
	for _, source := range metadata.SendNowSources {
		if source.TaskID != taskID || source.QueuedBy == "" || messagequeue.IsReservedQueuedBy(source.QueuedBy) {
			continue
		}
		if source.ID == id || source.Metadata.ClientQueueID == id || receiptContainsID(source.Metadata.QueueAdmissionIDs, id) {
			return true
		}
	}
	return false
}

func hasLateAnswerAdmission(ctx context.Context, tx *sqlx.Tx, taskID, sessionID, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	// LIKE is only a bounded candidate filter; every metadata identity is
	// compared exactly after decoding, and LIKE metacharacters are escaped.
	encodedID, _ := json.Marshal(id)
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(string(encodedID)) + "%"
	queries := []struct {
		table, sql  string
		args        []any
		checkSender bool
	}{
		{"task_session_messages", `SELECT id, COALESCE(metadata,''), '' FROM task_session_messages
		 WHERE task_id=? AND task_session_id=? AND author_type='user' AND type='message'
		 AND (id=? OR COALESCE(metadata,'') LIKE ? ESCAPE '\')`, []any{taskID, sessionID, id, pattern}, false},
		{"queued_messages", `SELECT id, COALESCE(metadata_json,''), queued_by FROM queued_messages
		 WHERE task_id=? AND session_id=? AND (id=? OR COALESCE(metadata_json,'') LIKE ? ESCAPE '\')`, []any{taskID, sessionID, id, pattern}, true},
	}
	for _, q := range queries {
		exists, err := internaldb.TableExistsContext(ctx, tx, q.table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		rows, err := tx.QueryContext(ctx, tx.Rebind(q.sql), q.args...)
		if err != nil {
			return false, fmt.Errorf("read late answer receipt: %w", err)
		}
		accepted, err := matchReceiptRows(rows, q.checkSender, taskID, id)
		if err != nil || accepted {
			return accepted, err
		}
	}
	return hasLateAnswerClaim(ctx, tx, taskID, sessionID, id)
}

func matchReceiptRows(rows *sql.Rows, checkSender bool, taskID, id string) (bool, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rowID, metadata, sender string
		if err := rows.Scan(&rowID, &metadata, &sender); err != nil {
			return false, err
		}
		if checkSender && (sender == "" || messagequeue.IsReservedQueuedBy(sender)) {
			continue
		}
		if rowID == id {
			var value lateAnswerReceiptMetadata
			if metadata == "" {
				metadata = "{}"
			}
			if json.Unmarshal([]byte(metadata), &value) == nil && value.DeliveryStatus != models.MessageDeliveryStatusBlocked && value.DeliveryStatus != models.MessageDeliveryStatusNotDelivered {
				return true, nil
			}
		} else if receiptMetadataMatches(metadata, taskID, id) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, rows.Close()
}

func hasLateAnswerClaim(ctx context.Context, tx *sqlx.Tx, taskID, sessionID, id string) (bool, error) {
	drv := tx.DriverName()
	for _, table := range []string{"queue_dispatch_claims", "queue_send_now_claims"} {
		exists, err := internaldb.TableExistsContext(ctx, tx, table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		var query string
		switch {
		case table == "queue_dispatch_claims":
			query = `SELECT ` + dialect.JSONExtract(drv, "message_json", "id") + `, ` + dialect.JSONExtract(drv, "message_json", "task_id") + `, ` + dialect.JSONExtract(drv, "message_json", "session_id") + `, ` + dialect.JSONExtract(drv, "message_json", "queued_by") + `, COALESCE(` + dialect.JSONExtract(drv, "message_json", "metadata") + `,'') FROM queue_dispatch_claims WHERE session_id=?`
		case dialect.IsPostgres(drv):
			query = `SELECT src.value->>'id', src.value->>'task_id', src.value->>'session_id', src.value->>'queued_by', COALESCE(src.value->>'metadata','') FROM queue_send_now_claims c CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(c.claim_json::jsonb->'sources')='array' THEN c.claim_json::jsonb->'sources' ELSE '[]'::jsonb END) src(value) WHERE c.session_id=?`
		default:
			query = `SELECT json_extract(src.value,'$.id'), json_extract(src.value,'$.task_id'), json_extract(src.value,'$.session_id'), json_extract(src.value,'$.queued_by'), COALESCE(json_extract(src.value,'$.metadata'),'') FROM queue_send_now_claims c, json_each(c.claim_json,'$.sources') src WHERE c.session_id=?`
		}
		rows, err := tx.QueryContext(ctx, tx.Rebind(query), sessionID)
		if err != nil {
			return false, fmt.Errorf("read late answer dispatch claim: %w", err)
		}
		accepted, err := matchClaimRows(rows, taskID, sessionID, id)
		if err != nil || accepted {
			return accepted, err
		}
	}
	return false, nil
}

func matchClaimRows(rows *sql.Rows, taskID, sessionID, id string) (bool, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rowID, rowTask, rowSession, sender, metadata sql.NullString
		if err := rows.Scan(&rowID, &rowTask, &rowSession, &sender, &metadata); err != nil {
			_ = rows.Close()
			return false, err
		}
		if rowTask.String != taskID || rowSession.String != sessionID || sender.String == "" || messagequeue.IsReservedQueuedBy(sender.String) {
			continue
		}
		if rowID.String == id || receiptMetadataMatches(metadata.String, taskID, id) {
			_ = rows.Close()
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, rows.Close()
}
