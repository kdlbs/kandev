// Package usage subscribes to the session-prompt-usage bus subject and
// records an append-only task_usage_events ledger row per observed event
// (docs/specs/task-cost-ledger/spec.md). It must not import any
// internal/office/** package (AC-25): Office records its own, separate
// cost_events ledger off the same bus subject, and this package is a
// sibling consumer, not a dependent.
package usage

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/events/bus"
)

// usageEventPayload mirrors lifecycle.SessionPromptUsageEventPayload's JSON
// shape field-for-field (AC-25). It is declared locally rather than
// importing that type because internal/agent/runtime/lifecycle imports
// internal/task/models, so importing it here would point a dependency edge
// back up the stack the wrong way; the wire contract is the JSON, not a
// shared Go type.
type usageEventPayload struct {
	TaskID         string              `json:"task_id"`
	SessionID      string              `json:"session_id"`
	AgentID        string              `json:"agent_id"`
	AgentProfileID string              `json:"agent_profile_id,omitempty"`
	AgentType      string              `json:"agent_type,omitempty"`
	Model          string              `json:"model,omitempty"`
	Usage          *promptUsagePayload `json:"usage"`
	Timestamp      string              `json:"timestamp"`
	TurnID         string              `json:"turn_id,omitempty"`
	UsageEventID   string              `json:"usage_event_id,omitempty"`
}

// promptUsagePayload mirrors streams.PromptUsage's JSON shape (AC-30).
// OutputTokensPresent is always serialized by the producer, which is what
// makes an unsampled output-token count distinguishable from a measured
// zero; CachedReadTokens, CachedWriteTokens and ThoughtTokens are plain
// omitempty ints, so for those three an absent field and a measured zero
// are indistinguishable on the wire by design (AC-30).
type promptUsagePayload struct {
	InputTokens                  int64 `json:"input_tokens"`
	OutputTokens                 int64 `json:"output_tokens"`
	OutputTokensPresent          bool  `json:"output_tokens_present"`
	CachedReadTokens             int64 `json:"cached_read_tokens,omitempty"`
	CachedWriteTokens            int64 `json:"cached_write_tokens,omitempty"`
	ThoughtTokens                int64 `json:"thought_tokens,omitempty"`
	TotalTokens                  int64 `json:"total_tokens,omitempty"`
	ProviderReportedCostSubcents int64 `json:"provider_reported_cost_subcents,omitempty"`
	ProviderReportedCostPresent  bool  `json:"provider_reported_cost_present,omitempty"`
	Estimated                    bool  `json:"estimated,omitempty"`
}

// decodePayload extracts a typed usageEventPayload from a bus event via a
// JSON marshal/unmarshal round trip (mirrors office/service.decodeEventData
// for the same wire event, without importing that unexported helper).
func decodePayload(event *bus.Event) (*usageEventPayload, error) {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	var payload usageEventPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
