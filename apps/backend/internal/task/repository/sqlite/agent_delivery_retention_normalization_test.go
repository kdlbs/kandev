package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionStorageCompactionNormalization(t *testing.T) {
	base := retentionNormalizationCandidate(t)
	const formatOneHash = "4536fb021979c241745519d599bb2f8c764c86abb5d5267399582e9d1acf0b94"
	if got := agentDeliveryEventHash(&base.event); got != formatOneHash {
		t.Fatalf("format-one producer hash changed: %s", got)
	}
	for _, test := range []struct {
		name      string
		change    func(*agentDeliveryRetentionCandidate)
		eligible  bool
		wantError error
	}{
		{name: "producer_payload", eligible: true},
		{name: "version_one", eligible: true, change: func(c *agentDeliveryRetentionCandidate) {
			c.event.HashVersion = 1
			c.event.ContentHash = agentDeliveryEventHash(&c.event)
			c.event.PayloadBytes = int64(len(c.event.Payload))
		}},
		{name: "user_role", change: func(c *agentDeliveryRetentionCandidate) {
			c.event.Payload = []byte(`{"type":"message_chunk","role":"user","text":"retained"}`)
		}},
		{name: "type_mismatch", change: func(c *agentDeliveryRetentionCandidate) {
			c.event.Payload = []byte(`{"type":"reasoning","text":"retained"}`)
		}},
		{name: "typed_field_invalid", change: func(c *agentDeliveryRetentionCandidate) {
			c.event.Payload = []byte(`{"type":"message_chunk","text":42}`)
		}},
		{name: "malformed", change: func(c *agentDeliveryRetentionCandidate) {
			c.event.Payload = []byte(`{"type":"message_chunk","text":`)
		}},
		{name: "unsupported", change: func(c *agentDeliveryRetentionCandidate) {
			c.event.EventType = "adapter_extension"
			c.event.Payload = []byte(`{"type":"adapter_extension","text":"retained"}`)
		}},
		{name: "terminal_type_mismatch", change: func(c *agentDeliveryRetentionCandidate) { c.event.Terminal = true }},
		{name: "unsupported_hash", wantError: repoerrors.ErrAgentDeliveryHashVersion, change: func(c *agentDeliveryRetentionCandidate) { c.event.HashVersion = 99 }},
		{name: "corrupt_hash", wantError: repoerrors.ErrAgentDeliveryReceiptCorrupt, change: func(c *agentDeliveryRetentionCandidate) {
			c.event.HashVersion = 1
			c.event.ContentHash = "untrusted stored hash"
		}},
		{name: "wrong_byte_count", wantError: repoerrors.ErrAgentDeliveryReceiptCorrupt, change: func(c *agentDeliveryRetentionCandidate) { c.event.PayloadBytes = 1 }},
		{name: "identity_protected", change: func(c *agentDeliveryRetentionCandidate) { c.cursorIdentityValid = false }},
		{name: "proof_protected", change: func(c *agentDeliveryRetentionCandidate) { c.canPrune = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			if test.change != nil {
				test.change(&candidate)
			}
			candidate.payloadLength = int64(len(candidate.event.Payload))
			wantHash := agentDeliveryEventHash(&candidate.event)
			previewEvents := []agentDeliveryRetentionCandidate{candidate}
			var preview AgentDeliveryRetentionPreview
			previewEventPayloads(previewEvents, time.Unix(1, 0), 4<<20, &preview)
			if (preview.EventPayloadCandidates == 1) != test.eligible {
				t.Fatalf("preview eligibility: %+v", preview)
			}
			if test.eligible && previewEvents[0].event.ContentHash != wantHash {
				t.Fatal("preview changed the format-one normalized hash")
			}
			compact, err := agentDeliveryCandidatesForCompaction(context.Background(), []agentDeliveryRetentionCandidate{candidate}, 4<<20, &models.AgentDeliveryRetentionBatchResult{})
			if !errors.Is(err, test.wantError) || (len(compact) == 1) != test.eligible {
				t.Fatalf("compaction eligibility: count=%d error=%v", len(compact), err)
			}
			if test.eligible && compact[0].event.ContentHash != wantHash {
				t.Fatal("compaction changed the format-one normalized hash")
			}
		})
	}
}

func TestDeliveryRetentionStorageCompactionNormalizationAllocations(t *testing.T) {
	candidate := retentionNormalizationCandidate(t)
	candidate.event.HashVersion = 1
	candidate.event.ContentHash = agentDeliveryEventHash(&candidate.event)
	candidate.event.PayloadBytes = int64(len(candidate.event.Payload))
	// One independent normalized hash is the allocation baseline. Eligibility
	// must reuse its typed decode; collection requires only small fixed overhead.
	singleHash := testing.AllocsPerRun(50, func() { _ = agentDeliveryEventHash(&candidate.event) })
	compactAllocs := testing.AllocsPerRun(50, func() {
		_, err := agentDeliveryCandidatesForCompaction(context.Background(), []agentDeliveryRetentionCandidate{candidate}, 4<<20, &models.AgentDeliveryRetentionBatchResult{})
		if err != nil {
			t.Fatal(err)
		}
	})
	previewAllocs := testing.AllocsPerRun(50, func() {
		previewEventPayloads([]agentDeliveryRetentionCandidate{candidate}, time.Unix(1, 0), 4<<20, &AgentDeliveryRetentionPreview{})
	})
	t.Logf("normalized hash allocations=%g compact=%g preview=%g", singleHash, compactAllocs, previewAllocs)
	if compactAllocs > singleHash+6 || previewAllocs > singleHash+6 {
		t.Fatalf("compaction and preview must avoid repeated payload decode: hash=%g compact=%g preview=%g", singleHash, compactAllocs, previewAllocs)
	}
}

func retentionNormalizationCandidate(t *testing.T) agentDeliveryRetentionCandidate {
	t.Helper()
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: strings.Repeat("retained output", 80),
		Role: "assistant", PromptGeneration: 7, ControlTurnID: 9, TurnID: "local-turn",
		CanonicalMessageID: "local-message", CanonicalProjection: true, CanonicalMessageAppend: true,
		Data: map[string]any{"provider_diagnostic": "preserved", "nested": map[string]any{"count": 1.0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return agentDeliveryRetentionCandidate{
		event: models.AgentDeliveryEvent{StreamID: "normalize-stream", Sequence: 17, SessionID: "session",
			IncarnationID: "incarnation", HarnessGeneration: 3, SubmissionID: "prompt:normalize",
			EventType: streams.EventTypeMessageChunk, Payload: payload},
		payloadLoaded: true, payloadLength: int64(len(payload)), cursorIdentityValid: true, canPrune: true,
	}
}
