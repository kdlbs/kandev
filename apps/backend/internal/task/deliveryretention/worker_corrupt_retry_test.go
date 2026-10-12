package deliveryretention

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionPersistentCorruptionBackoff(t *testing.T) {
	clock := newFakeClock()
	repo := &workerRepository{calls: make(chan string, 32), retentionErrors: []error{
		repoerrors.ErrAgentDeliveryReceiptCorrupt, repoerrors.ErrAgentDeliveryReceiptCorrupt, nil,
	}}
	snapshots := make(chan Snapshot, 32)
	worker := New(repo, db.NewPool(nil, nil), Options{
		After: clock.After, RetryDelay: func(attempt int) time.Duration { return time.Duration(attempt) * time.Second },
		Readiness:  func(context.Context) error { return nil },
		OnSnapshot: func(s Snapshot) { snapshots <- s },
	})
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()
	if failed := receiveSnapshot(t, snapshots); !failed.StructuralFailure {
		t.Fatalf("initial corruption: %+v", failed)
	}
	fireAfter(t, clock, time.Second)
	failed := receiveSnapshot(t, snapshots)
	if failed.ReadinessRecovered || !failed.StructuralFailure {
		t.Fatalf("corrupt receipt cannot report recovery: %+v", failed)
	}
	fireAfter(t, clock, 2*time.Second)
	if recovered := receiveSnapshot(t, snapshots); !recovered.ReadinessRecovered || recovered.FailedBatches != 0 {
		t.Fatalf("successful cleanup must report recovery: %+v", recovered)
	}
	if delay := receiveDelay(t, clock); delay != defaultInterval {
		t.Fatalf("successful cleanup must reset retry delay: %s", delay)
	}
}
