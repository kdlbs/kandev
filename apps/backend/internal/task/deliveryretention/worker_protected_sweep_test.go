package deliveryretention

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.1 AC-PLATFORM-DELIVERY-RETENTION-005.5 AC-PLATFORM-DELIVERY-RETENTION-005.6
func TestDeliveryRetentionWorkerStaggeredWrapYields(t *testing.T) {
	for _, sources := range []string{"receipt-compaction", "prompt-compaction", "all"} {
		t.Run(sources, func(t *testing.T) {
			now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
			repo := &staggeredDiscoveryRepository{productivePhaseRepository: productivePhaseRepository{calls: map[string]int{}, onCall: func() { now = now.Add(300 * time.Millisecond) }}, sources: sources}
			timers := make(chan time.Duration, 1)
			snapshots := make(chan Snapshot, 16)
			fire := make(chan time.Time)
			worker := New(repo, db.NewPool(nil, nil), Options{Now: func() time.Time { return now }, After: func(d time.Duration) <-chan time.Time { timers <- d; return fire }, OnSnapshot: func(s Snapshot) { snapshots <- s }})
			if err := worker.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(worker.Stop)
			for cycle := 0; cycle < 2; cycle++ {
				select {
				case d := <-timers:
					if d != time.Minute {
						t.Fatalf("wrong idle delay: %s", d)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("staggered discovery did not yield")
				}
				if cycle == 0 {
					fire <- now
				}
			}
			worker.Stop()
			if repo.calls["retention"] > 75 {
				t.Fatalf("repeated old frontiers: %d batches", repo.calls["retention"])
			}
			if repo.calls["orphans"] < 3 || repo.calls["retirements"] < 3 {
				t.Fatal("test must span multiple passes with phase fairness")
			}
			for len(snapshots) > 0 {
				s := <-snapshots
				if s.EventPayloadsPruned != 0 || s.FailedBatches != 0 {
					t.Fatalf("unexpected work: %+v", s)
				}
			}
		})
	}
}

type staggeredDiscoveryRepository struct {
	productivePhaseRepository
	sources  string
	previous models.AgentDeliveryRetentionDiscovery
}

func (r *staggeredDiscoveryRepository) RunAgentDeliveryRetentionBatch(context.Context, models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.calls["retention"]++
	r.onCall()
	n := r.calls["retention"]
	// Seventy novel positions span several passes; then two sources wrap out of phase.
	a, b := n, n
	if n > 70 {
		a = 68 + n%2
		b = 69 - n%2
	}
	after := models.AgentDeliveryRetentionDiscovery{CompactionValid: true, CompactionAt: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), CompactionStreamID: "c", CompactionSequence: int64(a)}
	if r.sources != "prompt-compaction" {
		after.ReceiptStreamID = "r"
		after.ReceiptSequence = int64(b)
	}
	if r.sources != "receipt-compaction" {
		after.SubmissionID = fmt.Sprintf("%03d", b)
	}
	result := models.AgentDeliveryRetentionBatchResult{Candidates: 2, DiscoveryAdvanced: true, DiscoveryBefore: r.previous, DiscoveryAfter: after}
	r.previous = after
	return result, nil
}

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.5 AC-PLATFORM-DELIVERY-RETENTION-005.6
func TestDeliveryRetentionWorkerDiscoveryFrontiers(t *testing.T) {
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	before := models.AgentDeliveryRetentionDiscovery{CompactionValid: true, CompactionAt: at, CompactionStreamID: "c", CompactionSequence: 1, ReceiptStreamID: "r", ReceiptSequence: 2, SubmissionID: "b"}
	progress := discoveryProgress{}
	steps := []struct {
		name  string
		after models.AgentDeliveryRetentionDiscovery
		want  bool
	}{
		{"compaction advances as receipt wraps", models.AgentDeliveryRetentionDiscovery{CompactionValid: true, CompactionAt: at, CompactionStreamID: "c", CompactionSequence: 2, ReceiptStreamID: "r", ReceiptSequence: 1}, true},
		{"staggered wrap revisits old receipt", before, false},
		{"time precedes stream ordering", models.AgentDeliveryRetentionDiscovery{CompactionValid: true, CompactionAt: at.Add(time.Second), CompactionStreamID: "a", CompactionSequence: 1}, true},
		{"older projection cannot advance", models.AgentDeliveryRetentionDiscovery{CompactionValid: true, CompactionAt: at, CompactionStreamID: "z", CompactionSequence: 9}, false},
		{"new receipt stream", models.AgentDeliveryRetentionDiscovery{ReceiptStreamID: "s", ReceiptSequence: 1}, true},
		{"new prompt independently advances", models.AgentDeliveryRetentionDiscovery{SubmissionID: "c"}, true},
		{"empty resets cannot lower maxima", models.AgentDeliveryRetentionDiscovery{}, false},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			if got := progress.advance(before, step.after); got != step.want {
				t.Fatalf("discovery productive=%t, want %t", got, step.want)
			}
		})
	}
}
