package deliveryretention

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.6
func TestDeliveryRetentionWorkerDrainsBeforeWaiting(t *testing.T) {
	for _, discoveryOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "discovery"}[discoveryOnly], func(t *testing.T) {
			now := time.Now()
			repo := &drainingRepository{productivePhaseRepository: productivePhaseRepository{calls: map[string]int{}, onCall: func() { now = now.Add(100 * time.Millisecond) }}, discoveryOnly: discoveryOnly}
			waited := make(chan int, 1)
			worker := New(repo, db.NewPool(nil, nil), Options{Now: func() time.Time { return now }, After: func(time.Duration) <-chan time.Time {
				waited <- repo.calls["retention"]
				return make(chan time.Time)
			}})
			if err := worker.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			defer worker.Stop()
			select {
			case calls := <-waited:
				// Seventy productive batches followed by one exhausted probe.
				if calls != 71 {
					t.Fatalf("worker slept before draining backlog: %d of 71 calls", calls)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not return to idle after exhaustion")
			}
		})
	}
}

type drainingRepository struct {
	productivePhaseRepository
	discoveryOnly bool
}

func (r *drainingRepository) RunAgentDeliveryRetentionBatch(context.Context, models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.calls["retention"]++
	r.onCall()
	result := models.AgentDeliveryRetentionBatchResult{}
	if r.calls["retention"] <= 70 {
		if r.discoveryOnly {
			result.DiscoveryAdvanced = true
		} else {
			result.EventPayloadsPruned = 1
		}
	}
	return result, nil
}
