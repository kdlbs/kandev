package deliveryretention

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionWorkerBackgroundQuantum(t *testing.T) {
	for _, rows := range []int{0, 10, 50, 150, 200} {
		t.Run(fmt.Sprint(rows), func(t *testing.T) {
			repo := &productivePhaseRepository{calls: map[string]int{}}
			worker := New(repo, db.NewPool(nil, nil), Options{BatchRows: rows})
			var snapshot Snapshot
			if _, err := worker.runBatch(t.Context(), "retention", &snapshot); err != nil {
				t.Fatal(err)
			}
			want := 50
			if rows > 0 {
				want = min(rows, want)
			}
			if len(repo.rows) != 1 || repo.rows[0] != want || !snapshot.SampleLimitReached {
				t.Fatalf("background lease must honor the operator ceiling and report its effective limit: rows=%v want=%d snapshot=%+v", repo.rows, want, snapshot)
			}
		})
	}
}

func TestDeliveryRetentionWorkerProductivePhases(t *testing.T) {
	repo := &productivePhaseRepository{calls: map[string]int{}}
	now := time.Now()
	worker := New(repo, db.NewPool(nil, nil), Options{
		Now: func() time.Time { return now },
		After: func(delay time.Duration) <-chan time.Time {
			now = now.Add(delay)
			fire := make(chan time.Time, 1)
			fire <- now
			return fire
		},
	})
	for pass := 1; pass <= 2; pass++ {
		snapshot := worker.runPass(t.Context())
		if repo.calls["orphans"] != pass || repo.calls["retirements"] != pass {
			t.Fatalf("quiet phases must be checked once each pass: %+v", repo.calls)
		}
		if snapshot.EventPayloadsPruned < 40 || snapshot.Duration > passBudget {
			t.Fatalf("productive phase lost work to repeated quiet checks: %+v", snapshot)
		}
	}
}

func TestDeliveryRetentionWorkerRemainingYieldBudget(t *testing.T) {
	now := time.Now()
	repo := &productivePhaseRepository{calls: map[string]int{}, onCall: func() { now = now.Add(20 * time.Millisecond) }}
	worker := New(repo, db.NewPool(nil, nil), Options{
		Now: func() time.Time { return now },
		After: func(delay time.Duration) <-chan time.Time {
			now = now.Add(delay)
			fire := make(chan time.Time, 1)
			fire <- now
			return fire
		},
	})
	if snapshot := worker.runPass(t.Context()); snapshot.Duration > passBudget {
		t.Fatalf("yield must not extend a pass beyond its remaining budget: %+v", snapshot)
	}
}

type productivePhaseRepository struct {
	calls  map[string]int
	onCall func()
	rows   []int
}

func (r *productivePhaseRepository) RunAgentDeliveryRetentionBatch(_ context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.calls["retention"]++
	r.rows = append(r.rows, options.MaxCandidates)
	if r.onCall != nil {
		r.onCall()
	}
	return models.AgentDeliveryRetentionBatchResult{Candidates: options.MaxCandidates, EventPayloadsPruned: 1}, nil
}

func (r *productivePhaseRepository) RunAgentDeliveryOrphanBatch(context.Context, models.AgentDeliveryOrphanBatchOptions) (models.AgentDeliveryOrphanBatchResult, error) {
	r.calls["orphans"]++
	if r.onCall != nil {
		r.onCall()
	}
	return models.AgentDeliveryOrphanBatchResult{}, nil
}

func (r *productivePhaseRepository) RunAgentDeliveryRetirementBatch(context.Context, models.AgentDeliveryRetirementBatchOptions) (models.AgentDeliveryRetirementBatchResult, error) {
	r.calls["retirements"]++
	if r.onCall != nil {
		r.onCall()
	}
	return models.AgentDeliveryRetirementBatchResult{}, nil
}

func TestDeliveryRetentionWorkerRemainingBatchBudget(t *testing.T) {
	now := time.Now()
	deadline := now.Add(passBudget)
	var admitted []time.Duration
	repo := &productivePhaseRepository{calls: map[string]int{}, onCall: func() {
		admitted = append(admitted, deadline.Sub(now))
		now = now.Add(53 * time.Millisecond)
	}}
	worker := New(repo, db.NewPool(nil, nil), Options{
		Now: func() time.Time { return now },
		After: func(delay time.Duration) <-chan time.Time {
			now = now.Add(delay)
			fire := make(chan time.Time, 1)
			fire <- now
			return fire
		},
	})
	snapshot := worker.runPass(t.Context())
	for _, timeout := range admitted {
		if timeout < batchBudget {
			t.Fatalf("pass admitted a truncated batch lease %s; full lease %s must fit remaining pass budget: %+v", timeout, batchBudget, snapshot)
		}
	}
	if snapshot.Duration > passBudget || snapshot.EventPayloadsPruned < 20 {
		t.Fatalf("bounded pass lost productive service: %+v", snapshot)
	}
}
