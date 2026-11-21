package deliveryretention

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/maintenance"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.1, AC-PLATFORM-DELIVERY-RETENTION-005.5
func TestWorkerBatchCeilingRecoversAfterCommittedBatches(t *testing.T) {
	repo := &batchOptionsRepository{retentionErrors: []error{
		context.DeadlineExceeded, context.DeadlineExceeded, context.DeadlineExceeded,
		context.DeadlineExceeded, context.DeadlineExceeded,
	}}
	worker := New(repo, db.NewPool(nil, nil), Options{BatchRows: 50})
	var snapshot Snapshot
	for range 5 {
		if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
			t.Fatal("retention batch stopped the pass")
		}
	}
	if got := worker.batchRowLimit("retention"); got != 1 {
		t.Fatalf("retention ceiling after deadlines = %d, want 1", got)
	}
	worker.recordBatchFailure(context.Background(), "orphans", failureDeadline, context.DeadlineExceeded)
	if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
		t.Fatal("successful retention batch stopped the pass")
	}

	start := len(repo.retentionOptions)
	for range 6 {
		if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
			t.Fatal("successful retention batch stopped the pass")
		}
	}
	var recovered []int
	for _, options := range repo.retentionOptions[start:] {
		recovered = append(recovered, options.MaxCandidates)
	}
	if want := []int{2, 4, 8, 16, 32, 50}; !reflect.DeepEqual(recovered, want) {
		t.Fatalf("recovered retention candidate limits = %v, want %v", recovered, want)
	}
	if got := worker.batchRowLimit("retention"); got != 50 {
		t.Fatalf("retention ceiling after recovery = %d, want 50", got)
	}
	if got := worker.batchRowLimit("orphans"); got != 25 {
		t.Fatalf("orphan ceiling changed during retention recovery: got %d, want 25", got)
	}

	repo.retentionErrors = append(repo.retentionErrors, context.DeadlineExceeded)
	if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
		t.Fatal("later retention deadline stopped the pass")
	}
	if got := worker.batchRowLimit("retention"); got != 25 {
		t.Fatalf("retention ceiling after later deadline = %d, want 25", got)
	}

	release, acquired := maintenance.ForPool(worker.pool).TryAcquire()
	if !acquired {
		t.Fatal("failed to acquire maintenance gate for busy-batch check")
	}
	if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
		t.Fatal("busy retention batch stopped the pass")
	}
	release()
	if got := worker.batchRowLimit("retention"); got != 25 {
		t.Fatalf("busy retention batch raised ceiling to %d, want 25", got)
	}

	repo.retentionErrors = append(repo.retentionErrors, errors.New("storage unavailable"))
	if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
		t.Fatal("failed retention batch stopped the pass")
	}
	if got := worker.batchRowLimit("retention"); got != 25 {
		t.Fatalf("failed retention batch raised ceiling to %d, want 25", got)
	}
}
