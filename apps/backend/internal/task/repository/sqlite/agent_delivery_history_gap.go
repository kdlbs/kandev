package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) validateHistoryGapCursorTx(ctx context.Context, tx *sqlx.Tx, recovery models.AgentDeliveryRecovery) (bool, error) {
	gap := recovery.HistoryGap
	if gap == nil {
		return true, nil
	}
	if gap.ReceivedSequence < 0 || gap.ReceivedSequence != gap.ProjectedSequence ||
		gap.FirstSequence != gap.ReceivedSequence+1 || gap.LastSequence < gap.FirstSequence {
		return false, nil
	}
	query := `SELECT session_id, incarnation_id, harness_generation, received_sequence, projected_sequence
		FROM agent_delivery_cursors WHERE stream_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var sessionID, incarnationID string
	var generation, received, projected int64
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), recovery.StreamID).Scan(&sessionID, &incarnationID, &generation, &received, &projected)
	if errors.Is(err, sql.ErrNoRows) {
		return gap.ReceivedSequence == 0 && gap.ProjectedSequence == 0, nil
	}
	if err != nil {
		return false, err
	}
	return sessionID == recovery.SessionID && incarnationID == recovery.IncarnationID &&
		generation == recovery.HarnessGeneration && received == gap.ReceivedSequence && projected == gap.ProjectedSequence, nil
}

func retainDeliveryHistoryGap(metadata map[string]json.RawMessage, recovery models.AgentDeliveryRecovery, nativeSessionID string) error {
	if recovery.HistoryGap == nil {
		return nil
	}
	if recovery.Reconstruction == nil || recovery.Reconstruction.PayloadHash == "" || nativeSessionID == "" {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	records := map[string]models.AgentDeliveryHistoryGapRecord{}
	if raw, exists := metadata[models.SessionMetaKeyAgentDeliveryHistoryGaps]; exists {
		if err := json.Unmarshal(raw, &records); err != nil || records == nil {
			return repoerrors.ErrAgentDeliveryReconstructionConflict
		}
	}
	record := models.AgentDeliveryHistoryGapRecord{StreamID: recovery.StreamID, IncarnationID: recovery.IncarnationID,
		HarnessGeneration: recovery.HarnessGeneration, SubmissionID: recovery.SubmissionID,
		PayloadHash: recovery.Reconstruction.PayloadHash, NativeSessionID: nativeSessionID, Gap: *recovery.HistoryGap}
	if previous, exists := records[recovery.StreamID]; exists && previous != record {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	records[recovery.StreamID] = record
	encoded, err := json.Marshal(records)
	if err != nil {
		return err
	}
	metadata[models.SessionMetaKeyAgentDeliveryHistoryGaps] = encoded
	return nil
}
