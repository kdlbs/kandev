package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageAccounting(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, outcome := range []string{"acknowledged", "rollback", "lost_commit_ack", "consecutive_lost_commit_acks"} {
			t.Run(engine+"/"+outcome, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "accounting", 1, 0)
				const payloadSize, rows = 1024, deliveryretention.BackgroundBatchRows
				seedRetentionStorageRows(t, repo, events[0], rows*4, payloadSize)
				before := retentionCanonicalContent(t, repo, events[0].SessionID)
				if outcome == "rollback" {
					repo.agentDeliveryRetentionFailpoint = func(point string) error {
						if point == "after_progress" {
							repo.agentDeliveryRetentionFailpoint = nil
							return context.DeadlineExceeded
						}
						return nil
					}
				}
				measured := &retentionStorageCommitAckRepository{Repository: repo, loseAck: outcome == "lost_commit_ack", loseEveryAck: outcome == "consecutive_lost_commit_acks"}
				result, retries, removedBytes := runRetentionStorageAccountedBatch(t, measured, events[0], payloadSize, rows)
				wantRows, wantRetries := rows, 0
				if outcome != "acknowledged" {
					wantRetries = 1
				}
				wantAcknowledgedRows := rows
				if outcome == "lost_commit_ack" || outcome == "consecutive_lost_commit_acks" {
					wantAcknowledgedRows = 0
				}
				wantBytes := int64((wantRows-1)*payloadSize + len(events[0].Payload))
				if removedBytes != wantBytes || retries != wantRetries || result.EventRowsRetired != wantAcknowledgedRows {
					t.Fatalf("durable removal bytes=%d want=%d retries=%d want=%d result=%+v", removedBytes, wantBytes, retries, wantRetries, result)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", events[0].StreamID, rows*4-wantRows)
				if retentionCanonicalContent(t, repo, events[0].SessionID) != before {
					t.Fatal("commit acknowledgement recovery changed canonical output")
				}
			})
		}
	}
}

type retentionStorageCommitAckRepository struct {
	*Repository
	loseAck      bool
	loseEveryAck bool
}

func (r *retentionStorageCommitAckRepository) RunAgentDeliveryRetentionBatch(ctx context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	result, err := r.Repository.RunAgentDeliveryRetentionBatch(ctx, options)
	if err == nil && (r.loseAck || r.loseEveryAck) {
		r.loseAck = false
		return models.AgentDeliveryRetentionBatchResult{}, fmt.Errorf("commit acknowledgement: %w", context.DeadlineExceeded)
	}
	return result, err
}

func TestDeliveryRetentionStorageRetryLockSettlement(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "Postgres", "retry-lock", 1, 0)
	repo.db.SetMaxOpenConns(4)
	const rows, payloadSize = deliveryretention.BackgroundBatchRows, 1024
	seedRetentionStorageRows(t, repo, events[0], rows*2, payloadSize)
	before := retentionCanonicalContent(t, repo, events[0].SessionID)
	locked, err := repo.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locked.Rollback() }()
	if _, err := locked.Exec(`UPDATE agent_delivery_retention_state SET updated_at=updated_at WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	firstDeadline, released := make(chan struct{}), make(chan error, 1)
	go func() {
		<-firstDeadline
		// A server-side lease can outlive the client's deadline. This bounded delay
		// models settlement within the worker's first retry-backoff window.
		timer := time.NewTimer(time.Second)
		<-timer.C
		released <- locked.Rollback()
	}()
	t.Cleanup(func() {
		if err := <-released; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	})
	measured := &retentionStorageLockRepository{Repository: repo, firstDeadline: firstDeadline}
	result, retries, removed := runRetentionStorageAccountedBatch(t, measured, events[0], payloadSize, rows)
	wantBytes := int64((rows-1)*payloadSize + len(events[0].Payload))
	if retries != 1 || result.EventRowsRetired != rows || removed != wantBytes {
		t.Fatalf("settled lock recovery: retries=%d result=%+v removed=%d want=%d", retries, result, removed, wantBytes)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=?", events[0].StreamID, rows)
	if retentionCanonicalContent(t, repo, events[0].SessionID) != before {
		t.Fatal("lock settlement changed canonical history")
	}
}

type retentionStorageLockRepository struct {
	*Repository
	firstDeadline chan struct{}
}

func (r *retentionStorageLockRepository) RunAgentDeliveryRetentionBatch(ctx context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	result, err := r.Repository.RunAgentDeliveryRetentionBatch(ctx, options)
	if errors.Is(err, context.DeadlineExceeded) && r.firstDeadline != nil {
		close(r.firstDeadline)
		r.firstDeadline = nil
	}
	return result, err
}
