package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-005.5 AC-PLATFORM-DELIVERY-RETENTION-006.6
func TestDeliveryRetentionIntegrationHistoricalSweep(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, terminal, advance := seedRetentionPromptSubmission(t, engine, "historical-sweep")
			seedHistoricalCanonicalOutputs(t, repo, terminal)
			advance(26 * time.Hour)
			options := models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1}
			rollback := errors.New("rollback protected discovery")
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "after_progress" {
					return rollback
				}
				return nil
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), options)
			if !errors.Is(err, rollback) || result.DiscoveryAdvanced {
				t.Fatalf("rolled back scan claimed progress: %+v %v", result, err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			result, err = repo.RunAgentDeliveryRetentionBatch(t.Context(), options)
			if err != nil || !result.DiscoveryAdvanced || result.EventPayloadsPruned != 0 {
				t.Fatalf("protected first page must commit discovery progress: %+v %v", result, err)
			}
			// Reopening the initialized repository keeps the persisted frontier.
			reopened := NewWithInitializedDB(repo.db, repo.ro, repo.log)
			reopened.clockNow = repo.clockNow
			t.Cleanup(func() {
				if err := reopened.Close(); err != nil {
					t.Error(err)
				}
			})
			pruned := 0
			for attempt := 0; attempt < 20; attempt++ {
				result, err = reopened.RunAgentDeliveryRetentionBatch(t.Context(), options)
				if err != nil {
					t.Fatal(err)
				}
				pruned += result.EventPayloadsPruned
				if !result.DiscoveryAdvanced && result.EventPayloadsPruned == 0 {
					break
				}
			}
			if pruned != 2 {
				t.Fatalf("historical traversal pruned %d bodies, want 2", pruned)
			}
			// Once only protected rows remain, wrap must stop forward discovery.
			advancedOnly := true
			for attempt := 0; attempt < 20; attempt++ {
				result, err = reopened.RunAgentDeliveryRetentionBatch(t.Context(), options)
				if err != nil {
					t.Fatal(err)
				}
				advancedOnly = advancedOnly && result.DiscoveryAdvanced
			}
			if advancedOnly {
				t.Fatal("protected frontier wrap reported endless forward progress")
			}
		})
	}
}
