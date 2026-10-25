package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionPreparedHash(t *testing.T) {
	candidates := []agentDeliveryRetentionCandidate{{
		event:         models.AgentDeliveryEvent{StreamID: "prepared", Sequence: 1, EventType: "message_chunk", Payload: []byte(`{"text":"retained output","type":"message_chunk"}`)},
		payloadLoaded: true, cursorIdentityValid: true, canPrune: true,
	}}
	compact, err := agentDeliveryCandidatesForCompaction(context.Background(), candidates, 4<<20, &models.AgentDeliveryRetentionBatchResult{})
	if err != nil || len(compact) != 1 {
		t.Fatalf("prepare legacy compaction: candidates=%d error=%v", len(compact), err)
	}
	want := agentDeliveryEventHash(&compact[0].event)
	if compact[0].event.ContentHash != want {
		t.Fatal("prepared legacy candidate must carry its validated digest into the bulk writer")
	}
	_, args := agentDeliveryCompactionValues(compact)
	if args[2] != want {
		t.Fatal("bulk writer must use the prepared digest")
	}
}
