package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionPublicWorkBudget(t *testing.T) {
	for _, budget := range []time.Duration{0, 5 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, "SQLite", "budget", 1, 0)
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point != "before_payload" {
					return nil
				}
				// Model work that outlives the public budget before any payload mutation.
				timer := time.NewTimer(2 * agentDeliveryRetentionWorkTimeout)
				defer timer.Stop()
				select {
				case <-timer.C:
					return nil
				case <-t.Context().Done():
					return t.Context().Err()
				}
			}
			_, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{WorkTimeout: budget})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("public retention budget was not enforced: %v", err)
			}
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), events[0].StreamID)
			if err != nil || cursor.RetiredSequence != 0 {
				t.Fatalf("expired collector advanced checkpoint: %+v %v", cursor, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ? AND payload_pruned_at IS NULL", events[0].StreamID, 1)
		})
	}
}
