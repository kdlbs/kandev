package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
func TestDeliveryEventLimitRetentionBudget(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "event-limit", 3, 2)
			payload, err := json.Marshal(streams.AgentEvent{
				Type: streams.EventTypeMessageChunk,
				Text: strings.Repeat("x", int(deliverylimits.MaxEventBytes-1024)),
				Role: "assistant",
			})
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(payload)) > deliverylimits.MaxEventBytes {
				t.Fatalf("fixture payload %d exceeds the admitted event limit %d", len(payload), deliverylimits.MaxEventBytes)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`
				UPDATE agent_delivery_inbox SET payload = ?, payload_bytes = ?, content_hash = '', hash_version = 0
				WHERE stream_id = ? AND sequence = 1`),
				payload, int64(len(payload)), events[0].StreamID,
			); err != nil {
				t.Fatal(err)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(
				context.Background(), models.AgentDeliveryRetentionBatchOptions{},
			)
			if err != nil {
				t.Fatalf("run retention batch: %v", err)
			}
			if result.EventPayloadsPruned < 1 || result.PayloadBytesPruned < int64(len(payload)) {
				t.Fatalf("maximum admitted event was skipped by retention: %+v", result)
			}
		})
	}
}
