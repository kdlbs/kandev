package streams

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextCompactionPayloadRoundTripsAndSnapshots(t *testing.T) {
	preTokens := int64(31_300)
	payload := NewContextCompaction(ContextCompactionPayload{
		ID:         "compact-1",
		Status:     "completed",
		Summary:    "retained context",
		Trigger:    "manual",
		PreTokens:  &preTokens,
		DurationMs: compactionTestInt64Pointer(26800),
	})

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal context compaction: %v", err)
	}
	var decoded NormalizedPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal context compaction: %v", err)
	}
	if got := decoded.Kind(); got != ToolKindContextCompaction {
		t.Fatalf("kind = %q, want %q", got, ToolKindContextCompaction)
	}
	if got := decoded.ContextCompaction(); got == nil || got.Summary != "retained context" || got.Trigger != "manual" {
		t.Fatalf("decoded context compaction = %+v", got)
	}

	snapshot := decoded.Snapshot()
	decoded.ContextCompaction().Summary = "mutated"
	if got := snapshot.ContextCompaction().Summary; got != "retained context" {
		t.Fatalf("snapshot summary changed with source: %q", got)
	}
	if snapshot.ContextCompaction().PreTokens == decoded.ContextCompaction().PreTokens {
		t.Fatal("snapshot retained the source token-count pointer")
	}
}

func TestContextCompactionDisplayIsBoundedWithoutMutatingSource(t *testing.T) {
	longSummary := strings.Repeat("summary ", 20_000)
	payload := NewContextCompaction(ContextCompactionPayload{ID: "compact-1", Status: "completed", Summary: longSummary})
	event := AgentEvent{Type: EventTypeToolCall, ToolCallID: "compaction:s1:compact-1", NormalizedPayload: payload}

	prepared := PrepareToolEventWithLimit(event, 16<<10)
	if got := payload.ContextCompaction().Summary; got != longSummary {
		t.Fatal("display preparation mutated the source summary")
	}
	preparedSummary := prepared.NormalizedPayload.ContextCompaction().Summary
	if len(preparedSummary) >= len(longSummary) || !strings.Contains(preparedSummary, toolTruncationSuffix) {
		t.Fatalf("prepared summary was not visibly truncated: length=%d", len(preparedSummary))
	}
	if int64(len(marshalCompactionTestValue(t, prepared))) > 16<<10 {
		t.Fatalf("prepared event exceeds the configured display limit")
	}
}

func compactionTestInt64Pointer(value int64) *int64 { return &value }

func marshalCompactionTestValue(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return encoded
}
