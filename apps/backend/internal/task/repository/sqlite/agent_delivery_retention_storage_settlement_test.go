package sqlite

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageDelayedCommitAccounting(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "Postgres", "delayed-commit", 1, 0)
	repo.db.SetMaxOpenConns(4)
	const rows, payloadSize = deliveryretention.BackgroundBatchRows, 1024
	seedRetentionStorageRows(t, repo, events[0], rows*4, payloadSize)
	canonical := retentionCanonicalContent(t, repo, events[0].SessionID)
	measured := &retentionStorageDelayedCommitRepository{Repository: repo, streamID: events[0].StreamID}
	t.Cleanup(func() {
		if measured.pending != nil {
			_ = measured.pending.Rollback()
		}
	})
	var removed int64
	for range 2 {
		_, _, bytes := runRetentionStorageAccountedBatch(t, measured, events[0], payloadSize, rows)
		removed += bytes
	}
	if want := int64((rows*3-1)*payloadSize + len(events[0].Payload)); removed != want {
		t.Fatalf("delayed commit accounting: bytes=%d want=%d", removed, want)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=?", events[0].StreamID, rows)
	if retentionCanonicalContent(t, repo, events[0].SessionID) != canonical {
		t.Fatal("delayed commit accounting changed canonical output")
	}
}

// Two deadline-returning passes can finish on the server at different times.
// The first pass becomes visible during the second call. The second pass keeps
// its state-row lease until the measurement synchronizes with settlement.
type retentionStorageDelayedCommitRepository struct {
	*Repository
	streamID string
	calls    int
	pending  *sqlx.Tx
}

func (r *retentionStorageDelayedCommitRepository) RunAgentDeliveryRetentionBatch(ctx context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.calls++
	if r.calls == 1 {
		return models.AgentDeliveryRetentionBatchResult{}, context.DeadlineExceeded
	}
	if err := r.finishPending(); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	result, err := r.Repository.RunAgentDeliveryRetentionBatch(ctx, options)
	if err != nil || r.calls != 2 {
		return result, err
	}
	if err := r.stagePending(ctx, options.MaxCandidates); err != nil {
		return models.AgentDeliveryRetentionBatchResult{}, err
	}
	return models.AgentDeliveryRetentionBatchResult{}, context.DeadlineExceeded
}

func (r *retentionStorageDelayedCommitRepository) stagePending(ctx context.Context, rows int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	r.pending = tx
	if _, err := tx.ExecContext(ctx, `UPDATE agent_delivery_retention_state SET updated_at=updated_at WHERE id=1`); err != nil {
		return err
	}
	// Stage only the durable removal boundary; ordinary passes still execute
	// through the actual retention method, including its preservation checks.
	for _, query := range []string{
		`DELETE FROM agent_delivery_inbox WHERE stream_id=$1 AND sequence > $2 AND sequence <= $3`,
		`DELETE FROM agent_delivery_effects WHERE stream_id=$1 AND sequence > $2 AND sequence <= $3 AND effect_type='agent_delivery.event'`,
	} {
		if _, err := tx.ExecContext(ctx, query, r.streamID, rows, rows*2); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_delivery_cursors SET retired_sequence=$1 WHERE stream_id=$2`, rows*2, r.streamID)
	return err
}

func (r *retentionStorageDelayedCommitRepository) finishPending() error {
	if r.pending == nil {
		return nil
	}
	err := r.pending.Commit()
	r.pending = nil
	return err
}

func (r *retentionStorageDelayedCommitRepository) awaitRetentionStorageSettlement(ctx context.Context) error {
	if err := r.finishPending(); err != nil {
		return err
	}
	return r.Repository.awaitRetentionStorageSettlement(ctx)
}
