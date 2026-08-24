package usage

import (
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
)

// TestDecodePayload_ValidEvent_PopulatesAllFields pins that decodePayload's
// locally-declared struct mirrors lifecycle.SessionPromptUsageEventPayload's
// JSON tags (AC-25) well enough to round-trip every field the writer needs.
func TestDecodePayload_ValidEvent_PopulatesAllFields(t *testing.T) {
	event := &bus.Event{
		Data: map[string]any{
			"task_id":          "task-1",
			"session_id":       "session-1",
			"agent_id":         "claude-acp",
			"agent_profile_id": "profile-1",
			"agent_type":       "claude-acp",
			"model":            "claude-sonnet-5",
			"turn_id":          "turn-1",
			"usage_event_id":   "evt-1",
			"timestamp":        "2026-08-23T12:00:00Z",
			"usage": map[string]any{
				"input_tokens":                   int64(100),
				"output_tokens":                  int64(30),
				"output_tokens_present":          true,
				"cached_read_tokens":             int64(25),
				"cached_write_tokens":            int64(5),
				"thought_tokens":                 int64(10),
				"total_tokens":                   int64(170),
				"provider_reported_cost_present": false,
				"estimated":                      false,
			},
		},
	}

	payload, err := decodePayload(event)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}

	if payload.TaskID != "task-1" || payload.SessionID != "session-1" ||
		payload.AgentID != "claude-acp" || payload.AgentProfileID != "profile-1" ||
		payload.AgentType != "claude-acp" || payload.Model != "claude-sonnet-5" ||
		payload.TurnID != "turn-1" || payload.UsageEventID != "evt-1" {
		t.Fatalf("payload = %+v, fields did not round-trip", payload)
	}

	if payload.Usage == nil {
		t.Fatal("payload.Usage = nil, want populated")
	}
	u := payload.Usage
	if u.InputTokens != 100 || u.OutputTokens != 30 || !u.OutputTokensPresent ||
		u.CachedReadTokens != 25 || u.CachedWriteTokens != 5 || u.ThoughtTokens != 10 {
		t.Errorf("usage = %+v, fields did not round-trip", u)
	}
}

// TestDecodePayload_OutputTokensPresentFalse_PinsAbsenceDistinctFromZero
// pins AC-4/AC-30's NULL-vs-zero distinction for output tokens: when the
// producer omits an output-token sample, output_tokens_present decodes to
// false even though output_tokens itself defaults to the JSON zero value.
func TestDecodePayload_OutputTokensPresentFalse_PinsAbsenceDistinctFromZero(t *testing.T) {
	event := &bus.Event{
		Data: map[string]any{
			"task_id":        "task-1",
			"usage_event_id": "evt-2",
			"usage": map[string]any{
				"input_tokens": int64(50),
				// output_tokens_present omitted entirely - never sampled.
			},
		},
	}

	payload, err := decodePayload(event)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}
	if payload.Usage.OutputTokensPresent {
		t.Error("OutputTokensPresent = true, want false when the field is absent from the wire payload")
	}
	if payload.Usage.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0 (JSON zero value, not meaningful when Present is false)", payload.Usage.OutputTokens)
	}
}

// TestDecodePayload_UndecodableData_ReturnsError pins the dropped:decode_error
// state-machine transition's precondition: a payload that cannot round-trip
// through JSON at all is a decode error, not a validation error.
func TestDecodePayload_UndecodableData_ReturnsError(t *testing.T) {
	event := &bus.Event{Data: func() {}}

	if _, err := decodePayload(event); err == nil {
		t.Fatal("decodePayload: got nil error, want an error for unmarshalable Data")
	}
}
