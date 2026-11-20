package sqlite

import (
	"errors"
	"testing"
	"time"

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

// The existing v1 normalized JSON retains all four capability booleans,
// including their false defaults, when the input omits them.
const legacyToolOutputBytesReceiptHash = "324f614b4f3177419e58a9872c650ab46529ca828fdaf0eb7eff4fc33c4cd6ff"

func TestDeliveryRetentionLegacyToolOutputBytesHashFixtures(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantHash   string
		wantLegacy string
	}{
		{
			name: "absent", payload: `{"type":"tool_update"}`,
			wantHash: legacyToolOutputBytesReceiptHash, wantLegacy: legacyToolOutputBytesReceiptHash,
		},
		{
			name: "null", payload: `{"type":"tool_update","tool_output_bytes":null}`,
			wantHash: legacyToolOutputBytesReceiptHash, wantLegacy: legacyToolOutputBytesReceiptHash,
		},
		{
			name: "zero", payload: `{"type":"tool_update","tool_output_bytes":0}`,
			wantHash:   "347e7a75374456eefec096eb77b0c4ed0c91b24d6154ced32a10f74630e34c0c",
			wantLegacy: legacyToolOutputBytesReceiptHash,
		},
		{
			name: "nonzero", payload: `{"type":"tool_update","tool_output_bytes":5}`,
			wantHash:   "01507a787a355635857e423fa910cc80bc9082e89e2a2981665520550c4a0dd8",
			wantLegacy: legacyToolOutputBytesReceiptHash,
		},
		{
			name: "maximum", payload: `{"type":"tool_update","tool_output_bytes":18446744073709551615}`,
			wantHash:   "4d79098e479bdc428977234499f8f93d20f553d39e0e977441966c73bb5463ad",
			wantLegacy: legacyToolOutputBytesReceiptHash,
		},
		{
			name:       "invalid type remains raw",
			payload:    `{"type":"tool_update","tool_output_bytes":"invalid"}`,
			wantHash:   "0fee317190a85d91036e9fae9d51208be62c2e6eb57e484340462dbc5e93a977",
			wantLegacy: "0fee317190a85d91036e9fae9d51208be62c2e6eb57e484340462dbc5e93a977",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := legacyToolOutputBytesReceipt(test.payload)
			if got := agentDeliveryEventHash(&event); got != test.wantHash {
				t.Fatalf("v1 event hash = %s, want historical fixture %s", got, test.wantHash)
			}
			if got := agentDeliveryEventHashWithoutToolOutputBytes(&event); got != test.wantLegacy {
				t.Fatalf("field-less v1 event hash = %s, want historical fixture %s", got, test.wantLegacy)
			}
		})
	}
}

func TestDeliveryRetentionLegacyToolOutputBytesMixedReaderReplay(t *testing.T) {
	for _, value := range []struct {
		name    string
		payload string
	}{
		{name: "zero", payload: `{"type":"tool_update","tool_output_bytes":0}`},
		{name: "nonzero", payload: `{"type":"tool_update","tool_output_bytes":5}`},
		{name: "maximum", payload: `{"type":"tool_update","tool_output_bytes":18446744073709551615}`},
	} {
		t.Run(value.name, func(t *testing.T) {
			stored := legacyToolOutputBytesReceipt(`{"type":"tool_update"}`)
			stored.HashVersion = agentDeliveryPayloadHashVersion
			stored.ContentHash = legacyToolOutputBytesReceiptHash
			incoming := legacyToolOutputBytesReceipt(value.payload)
			if same, err := sameAgentDeliveryEvent(&stored, &incoming); err != nil || !same {
				t.Fatalf("new reader must accept the old reader's field-less receipt: same=%t err=%v", same, err)
			}

			stored.Payload = nil
			prunedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			stored.PayloadPrunedAt = &prunedAt
			if same, err := sameAgentDeliveryEvent(&stored, &incoming); err != nil || !same {
				t.Fatalf("new reader must accept the compacted old receipt: same=%t err=%v", same, err)
			}
		})
	}
}

func TestDeliveryRetentionLegacyToolOutputBytesReceiptGuards(t *testing.T) {
	stored := legacyToolOutputBytesReceipt(`{"type":"tool_update"}`)
	stored.HashVersion = agentDeliveryPayloadHashVersion
	stored.ContentHash = legacyToolOutputBytesReceiptHash

	changedPayload := legacyToolOutputBytesReceipt(
		`{"type":"tool_update","text":"changed","tool_output_bytes":5}`,
	)
	if same, err := sameAgentDeliveryEvent(&stored, &changedPayload); err != nil || same {
		t.Fatalf("changed non-legacy content must conflict: same=%t err=%v", same, err)
	}
	for _, capability := range []string{"supports_image", "supports_audio", "supports_embedded_context", "supports_prompt_queueing"} {
		changed := legacyToolOutputBytesReceipt(`{"type":"tool_update","` + capability + `":true,"tool_output_bytes":5}`)
		if same, err := sameAgentDeliveryEvent(&stored, &changed); err != nil || same {
			t.Fatalf("changed capability %s must conflict: same=%t err=%v", capability, same, err)
		}
	}

	changedIdentity := legacyToolOutputBytesReceipt(`{"type":"tool_update","tool_output_bytes":5}`)
	changedIdentity.SessionID = "another-session"
	if same, err := sameAgentDeliveryEvent(&stored, &changedIdentity); err != nil || same {
		t.Fatalf("changed envelope must conflict: same=%t err=%v", same, err)
	}

	corrupt := stored
	corrupt.ContentHash = "untrusted alternate hash"
	if same, err := sameAgentDeliveryEvent(&corrupt, &changedPayload); same || err != repoerrors.ErrAgentDeliveryReceiptCorrupt {
		t.Fatalf("corrupt stored receipt must remain an error: same=%t err=%v", same, err)
	}

	fieldBearing := legacyToolOutputBytesReceipt(`{"type":"tool_update","tool_output_bytes":0}`)
	fieldBearing.HashVersion = agentDeliveryPayloadHashVersion
	fieldBearing.ContentHash = "347e7a75374456eefec096eb77b0c4ed0c91b24d6154ced32a10f74630e34c0c"
	changedField := legacyToolOutputBytesReceipt(`{"type":"tool_update","tool_output_bytes":5}`)
	if same, err := sameAgentDeliveryEvent(&fieldBearing, &changedField); err != nil || same {
		t.Fatalf("field-bearing receipt must keep its exact content identity: same=%t err=%v", same, err)
	}
}

func TestDeliveryRetentionLegacyToolOutputBytesCompactionReplay(t *testing.T) {
	old := legacyToolOutputBytesReceipt(`{"type":"tool_update"}`)
	old.HashVersion = agentDeliveryPayloadHashVersion
	old.ContentHash = legacyToolOutputBytesReceiptHash
	old.PayloadBytes = int64(len(old.Payload))
	candidate := agentDeliveryRetentionCandidate{
		event: old, cursorIdentityValid: true, canPrune: true,
		payloadLoaded: true, payloadLength: int64(len(old.Payload)),
	}
	hash, eligible, err := agentDeliveryCompactionHash(&candidate, 4<<20)
	if err != nil || !eligible || hash != legacyToolOutputBytesReceiptHash {
		t.Fatalf("legacy v1 compaction = %q, eligible=%t err=%v", hash, eligible, err)
	}

	compacted := old
	compacted.Payload = nil
	compacted.PayloadPrunedAt = &time.Time{}
	incoming := legacyToolOutputBytesReceipt(`{"type":"tool_update","tool_output_bytes":0}`)
	if same, err := sameAgentDeliveryEvent(&compacted, &incoming); err != nil || !same {
		t.Fatalf("replay after legacy receipt compaction = same:%t err:%v", same, err)
	}
}

func legacyToolOutputBytesReceipt(payload string) models.AgentDeliveryEvent {
	return models.AgentDeliveryEvent{
		StreamID: "legacy-stream", Sequence: 7, SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 3, SubmissionID: "prompt:legacy", EventType: "tool_update", Payload: []byte(payload),
	}
}
