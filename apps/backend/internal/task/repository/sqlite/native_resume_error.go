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

func (r *Repository) restoreAuthenticatedNativePresentationTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence, completedAt time.Time) (bool, error) {
	valid, err := r.restoreAuthenticatedNativeNoticeTx(ctx, tx, proof, completedAt)
	if err != nil || !valid {
		return false, err
	}
	return true, r.clearAuthenticatedNativeErrorTx(ctx, tx, proof, completedAt)
}

func (r *Repository) clearAuthenticatedNativeErrorTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence, completedAt time.Time) error {
	var raw sql.NullString
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT metadata FROM task_sessions WHERE id = ?`), proof.SessionID).Scan(&raw); err != nil {
		return err
	}
	metadata, err := decodeAgentDeliveryMetadata(raw)
	if err != nil {
		return err
	}
	if !nativeErrorMatchesRetiredSubmission(metadata[models.SessionMetaKeyLastAgentError], proof) {
		return nil
	}
	// Explicit null also clears a connected client's legacy error merge when
	// no recovery notice exists to carry a newer restored revision.
	metadata[models.SessionMetaKeyLastAgentError] = json.RawMessage("null")
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET metadata = ?, updated_at = ? WHERE id = ? AND COALESCE(metadata, '') = ?`), string(encoded), completedAt, proof.SessionID, nullableMetadataValue(raw))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("native recovery error metadata changed")
	}
	return nil
}

func nativeErrorMatchesRetiredSubmission(raw json.RawMessage, proof *models.NativeResumeDeliveryEvidence) bool {
	var record map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &record) != nil {
		return false
	}
	last, exists := models.LoadLastAgentError(map[string]any{models.SessionMetaKeyLastAgentError: record})
	if !exists || last.Code != durableDeliveryUncertainErrorCode {
		return false
	}
	submissionID := last.Details
	if structured, present := record["delivery_submission_id"]; present {
		submissionID, _ = structured.(string)
	}
	for _, row := range proof.Submissions {
		if row.ID == submissionID {
			return true
		}
	}
	return false
}
