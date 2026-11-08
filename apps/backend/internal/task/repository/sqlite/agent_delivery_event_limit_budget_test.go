package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
func TestDeliveryEventLimitRetention(t *testing.T) {
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
			// The public 250 ms work budget has separate coverage. Give the
			// maximum-payload case a generous deadline so it tests retention
			// behavior without depending on database and CI host speed.
			options := normalizeAgentDeliveryRetentionOptions(models.AgentDeliveryRetentionBatchOptions{})
			options.WorkTimeout = 5 * time.Second
			ctx, cancel := context.WithTimeout(context.Background(), options.WorkTimeout)
			defer cancel()
			result, err := repo.runAgentDeliveryRetentionBatch(ctx, options)
			if err != nil {
				t.Fatalf("run retention batch: %v", err)
			}
			if result.EventPayloadsPruned < 1 || result.PayloadBytesPruned < int64(len(payload)) {
				t.Fatalf("maximum admitted event was skipped by retention: %+v", result)
			}
		})
	}
}
