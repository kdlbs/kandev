package sqlite

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionHashComparison(t *testing.T) {
	for _, payload := range []string{`{"type":"message_chunk","text":"original"}`, `{"broken":`} {
		t.Run(payload, func(t *testing.T) {
			stored := models.AgentDeliveryEvent{
				StreamID: "hash-comparison-stream", Sequence: 1, SessionID: "session",
				IncarnationID: "incarnation", HarnessGeneration: 1,
				EventType: "message_chunk", Payload: []byte(payload), HashVersion: 1,
			}
			stored.ContentHash = agentDeliveryEventHash(&stored)
			incoming := stored
			incoming.ContentHash = "caller-controlled"
			if same, err := sameAgentDeliveryEvent(&stored, &incoming); err != nil || !same {
				t.Fatalf("identical bytes with caller hash: %v %v", same, err)
			}
			incoming.Payload = []byte("changed bytes")
			incoming.ContentHash = stored.ContentHash
			if same, err := sameAgentDeliveryEvent(&stored, &incoming); err != nil || same {
				t.Fatalf("forged incoming hash accepted: %v %v", same, err)
			}
			incoming = stored
			stored.ContentHash = "corrupt retained hash"
			if same, err := sameAgentDeliveryEvent(&stored, &incoming); !errors.Is(err, repoerrors.ErrAgentDeliveryReceiptCorrupt) || same {
				t.Fatalf("identical bytes bypassed retained hash validation: %v %v", same, err)
			}
		})
	}
}
