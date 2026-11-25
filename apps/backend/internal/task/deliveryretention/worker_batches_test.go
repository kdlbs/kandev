package deliveryretention

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionWorkerDeadlineBatchLimit(t *testing.T) {
	deadline := context.DeadlineExceeded
	repo := &batchOptionsRepository{retentionErrors: []error{
		deadline,
		fmt.Errorf("wrapped query deadline: %w", deadline),
		deadline,
		deadline,
		deadline,
		deadline,
	}}
	worker := New(repo, db.NewPool(nil, nil), Options{BatchRows: 180})
	var snapshot Snapshot

	for range 7 {
		if !worker.runPassBatch(t.Context(), "retention", batchBudget, &snapshot) {
			t.Fatal("retention batch stopped the pass")
		}
	}

	var got []int
	for _, options := range repo.retentionOptions {
		got = append(got, options.MaxCandidates)
	}
	want := []int{50, 25, 12, 6, 3, 1, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retention batch candidate limits = %v, want %v", got, want)
	}
	if snapshot.FailedBatches != 6 || snapshot.CommittedBatches != 1 {
		t.Fatalf("snapshot after repeated deadlines = %+v, want six failures and one commit", snapshot)
	}
	if got := worker.batchRowLimit("orphans"); got != 180 {
		t.Fatalf("unaffected orphan ceiling = %d, want 180", got)
	}
}

func TestDeliveryRetentionWorkerPhaseBatchLimitsAreIndependent(t *testing.T) {
	repo := &batchOptionsRepository{
		retentionErrors: []error{context.DeadlineExceeded},
		orphanErrors:    []error{context.DeadlineExceeded},
	}
	worker := New(repo, db.NewPool(nil, nil), Options{BatchRows: 180})
	var snapshot Snapshot

	for _, phase := range []string{"retention", "orphans", "retention", "orphans", "retirements"} {
		if !worker.runPassBatch(t.Context(), phase, batchBudget, &snapshot) {
			t.Fatalf("%s batch stopped the pass", phase)
		}
	}

	var retentionRows []int
	for _, options := range repo.retentionOptions {
		retentionRows = append(retentionRows, options.MaxCandidates)
	}
	if want := []int{50, 25}; !reflect.DeepEqual(retentionRows, want) {
		t.Fatalf("retention candidate limits = %v, want %v", retentionRows, want)
	}
	var orphanRows []int
	for _, options := range repo.orphanOptions {
		orphanRows = append(orphanRows, options.MaxCandidates)
	}
	if want := []int{180, 90}; !reflect.DeepEqual(orphanRows, want) {
		t.Fatalf("orphan candidate limits = %v, want %v", orphanRows, want)
	}
	if len(repo.retirementOptions) != 1 || repo.retirementOptions[0].MaxCandidates != 180 {
		t.Fatalf("retirement candidate limits = %+v, want one limit of 180", repo.retirementOptions)
	}
	if snapshot.FailedBatches != 2 || snapshot.CommittedBatches != 3 {
		t.Fatalf("snapshot after independent phase failures = %+v, want two failures and three commits", snapshot)
	}
}

func TestDeliveryRetentionWorkerBatchLimitIgnoresOtherErrorsAndParentCancellation(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	deadlineCtx, cancelDeadline := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancelDeadline()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
	}{
		{name: "unrelated error", ctx: context.Background(), err: errors.New("storage unavailable")},
		{name: "canceled parent", ctx: canceledCtx, err: context.DeadlineExceeded},
		{name: "expired parent deadline", ctx: deadlineCtx, err: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker := New(&batchOptionsRepository{}, db.NewPool(nil, nil), Options{BatchRows: 160})
			worker.recordBatchFailure(test.ctx, "orphans", classifyError(test.err), test.err)
			if got := worker.batchRowLimit("orphans"); got != 160 {
				t.Fatalf("orphan candidate limit after %s = %d, want 160", test.name, got)
			}
		})
	}
}

func TestDeliveryRetentionWorkerBatchRetryBackoffIncludesFailedPhase(t *testing.T) {
	clock := newFakeClock()
	repo := &batchOptionsRepository{retentionErrors: []error{context.DeadlineExceeded}}
	snapshots := make(chan Snapshot, 1)
	worker := New(repo, db.NewPool(nil, nil), Options{
		After:      clock.After,
		RetryDelay: func(int) time.Duration { return 7 * time.Second },
		OnSnapshot: func(snapshot Snapshot) { snapshots <- snapshot },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()

	snapshot := receiveSnapshot(t, snapshots)
	if snapshot.FailedBatches != 1 || snapshot.CommittedBatches != 2 {
		t.Fatalf("pass with failed retention and successful later phases = %+v", snapshot)
	}
	if timer := receiveTimer(t, clock); timer.delay != 7*time.Second {
		t.Fatalf("retry delay after a partial pass = %s, want 7s", timer.delay)
	}
}

func TestDeliveryRetentionWorkerNewConstructionResetsBatchLimits(t *testing.T) {
	options := Options{BatchRows: 160}
	first := New(&batchOptionsRepository{}, db.NewPool(nil, nil), options)
	first.recordBatchFailure(context.Background(), "orphans", failureDeadline, context.DeadlineExceeded)
	if got := first.batchRowLimit("orphans"); got != 80 {
		t.Fatalf("first worker orphan candidate limit = %d, want 80", got)
	}

	second := New(&batchOptionsRepository{}, db.NewPool(nil, nil), options)
	if got := second.batchRowLimit("orphans"); got != 160 {
		t.Fatalf("new worker orphan candidate limit = %d, want configured limit 160", got)
	}
	if got := second.batchRowLimit("retention"); got != BackgroundBatchRows {
		t.Fatalf("new worker retention candidate limit = %d, want ceiling %d", got, BackgroundBatchRows)
	}
}

type batchOptionsRepository struct {
	retentionOptions  []models.AgentDeliveryRetentionBatchOptions
	orphanOptions     []models.AgentDeliveryOrphanBatchOptions
	retirementOptions []models.AgentDeliveryRetirementBatchOptions
	retentionErrors   []error
	orphanErrors      []error
	retirementErrors  []error
}

func (r *batchOptionsRepository) RunAgentDeliveryRetentionBatch(_ context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.retentionOptions = append(r.retentionOptions, options)
	return models.AgentDeliveryRetentionBatchResult{}, nextBatchError(&r.retentionErrors)
}

func (r *batchOptionsRepository) RunAgentDeliveryOrphanBatch(_ context.Context, options models.AgentDeliveryOrphanBatchOptions) (models.AgentDeliveryOrphanBatchResult, error) {
	r.orphanOptions = append(r.orphanOptions, options)
	return models.AgentDeliveryOrphanBatchResult{}, nextBatchError(&r.orphanErrors)
}

func (r *batchOptionsRepository) RunAgentDeliveryRetirementBatch(_ context.Context, options models.AgentDeliveryRetirementBatchOptions) (models.AgentDeliveryRetirementBatchResult, error) {
	r.retirementOptions = append(r.retirementOptions, options)
	return models.AgentDeliveryRetirementBatchResult{}, nextBatchError(&r.retirementErrors)
}

func nextBatchError(queue *[]error) error {
	if len(*queue) == 0 {
		return nil
	}
	err := (*queue)[0]
	*queue = (*queue)[1:]
	return err
}
