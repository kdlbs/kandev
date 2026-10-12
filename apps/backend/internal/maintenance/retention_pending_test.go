package maintenance

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionMaintenanceRemainingJobs(t *testing.T) {
	fixture := newTestFixture(t)
	for _, sessionID := range []string{"deleted-a", "deleted-b"} {
		_, err := fixture.repo.DB().Exec(`INSERT INTO agent_delivery_cleanup_jobs
			(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
			VALUES (?, ?, ?, 'pending', ?, ?)`, sessionID, "inc-"+sessionID, time.Now(), time.Now(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, wantPartial := range []bool{true, false} {
		result, err := executeDeliveryRetention(context.Background(), fixture.repo, models.AgentDeliveryRetentionBatchOptions{})
		if err != nil || result.DeliveryRetirementJobsCompleted != 1 || result.PurgedRetiredDeliveryRows != 0 || result.Partial != wantPartial {
			t.Fatalf("remaining deletion jobs require accurate partial result=%+v wantPartial=%v err=%v", result, wantPartial, err)
		}
	}
}
