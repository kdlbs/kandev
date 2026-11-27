package deliveryretention

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.5
func TestDeliveryRetentionWorkerProtectedDiscoveryProgress(t *testing.T) {
	repo := &discoveryProgressRepository{productivePhaseRepository: productivePhaseRepository{calls: map[string]int{}}}
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
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
	if snapshot.EventPayloadsPruned != 1 || repo.calls["retention"] != 5 {
		t.Fatalf("protected discovery must reach eligible output then stop at exhaustion: calls=%v snapshot=%+v", repo.calls, snapshot)
	}
	if repo.calls["orphans"] != 1 || repo.calls["retirements"] != 1 || snapshot.Duration > passBudget {
		t.Fatalf("discovery must preserve phase fairness and pass budget: %+v %+v", repo.calls, snapshot)
	}
}

type discoveryProgressRepository struct {
	productivePhaseRepository
}

func (r *discoveryProgressRepository) RunAgentDeliveryRetentionBatch(context.Context, models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	r.calls["retention"]++
	result := models.AgentDeliveryRetentionBatchResult{Candidates: 1}
	switch r.calls["retention"] {
	case 1, 2, 3:
		result.DiscoveryAdvanced = true
		result.DiscoveryAfter.SubmissionID = fmt.Sprintf("%03d", r.calls["retention"])
	case 4:
		result.EventPayloadsPruned = 1
	}
	return result, nil
}
