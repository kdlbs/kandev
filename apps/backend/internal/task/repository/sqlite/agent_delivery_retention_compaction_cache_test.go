package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageCompactionStatementShape(t *testing.T) {
	candidates := make([]*agentDeliveryRetentionCandidate, 200)
	for n := range candidates {
		candidates[n] = &agentDeliveryRetentionCandidate{}
		candidates[n].event.StreamID = "compaction-shape"
		candidates[n].event.Sequence = int64(n + 1)
		candidates[n].event.ContentHash = fmt.Sprintf("digest-%d", n)
	}
	want, _ := agentDeliveryCompactionValues(candidates)
	for _, count := range []int{1, 17, 200} {
		got, args := agentDeliveryCompactionValues(candidates[:count])
		if got != want || len(args) != 800 {
			t.Fatalf("compaction must reuse one bounded SQL statement for %d rows: values bytes=%d args=%d", count, len(got), len(args))
		}
		for slot := count * 4; slot < 800; slot++ {
			if args[slot] != nil {
				t.Fatalf("unused compaction identity must be NULL: slot=%d value=%v", slot, args[slot])
			}
		}
	}
}

func TestDeliveryRetentionStorageCompactionExactSelection(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, count := range []int{1, 17, 200} {
			t.Run(fmt.Sprintf("%s/%d", engine, count), func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "compaction-exact", 1, 0)
				seedRetentionStorageRows(t, repo, events[0], 201, 1024)
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: count})
				if err != nil || result.EventPayloadsPruned != count || result.EventRowsRetired != count {
					t.Fatalf("exact prepared compaction of %d rows: %+v %v", count, result, err)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=?", events[0].StreamID, 201-count)
				hashes, payloads, err := repo.agentDeliveryCompactionStatements()
				if err != nil {
					t.Fatal(err)
				}
				result, err = repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
				if err != nil || result.EventRowsRetired != min(201-count, 200) {
					t.Fatalf("reused compaction must preserve remaining identities: %+v %v", result, err)
				}
				nextHashes, nextPayloads, err := repo.agentDeliveryCompactionStatements()
				if err != nil || hashes != nextHashes || payloads != nextPayloads {
					t.Fatalf("successful plans must be reused: %v", err)
				}
			})
		}
	}
}

func TestDeliveryRetentionStorageCompactionCancellationReuse(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "compaction-cancel", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 17, 1024)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "after_hash" {
					cancel()
				}
				return nil
			}
			if _, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel after actual hash statement: %v", err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND payload_pruned_at IS NULL", events[0].StreamID, 17)
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventPayloadsPruned != 17 || result.EventRowsRetired != 17 {
				t.Fatalf("prepared hash/payload plans must remain usable after cancellation rollback: %+v %v", result, err)
			}
		})
	}
}
