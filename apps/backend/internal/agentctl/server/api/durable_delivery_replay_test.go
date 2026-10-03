package api

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestReplayKeepsJournaledEventType(t *testing.T) {
	payload, err := json.Marshal(streams.AgentEvent{
		Type: "agent_link.offline_budget_exhausted",
		Data: map[string]any{"outcome": "cancelled"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got adapter.AgentEvent
	err = writeAgentStreamReplayEvent(journal.Event{Sequence: 3, Payload: payload}, func(e adapter.AgentEvent) error {
		got = e
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "agent_link.offline_budget_exhausted" || got.DeliverySequence != 3 || got.Data["outcome"] != "cancelled" {
		t.Fatalf("replayed event = %+v", got)
	}
}

func TestReplayFillsMissingTypeFromJournalEvent(t *testing.T) {
	var got adapter.AgentEvent
	err := writeAgentStreamReplayEvent(journal.Event{Type: "legacy_type", Payload: []byte(`{"outcome":"x"}`)}, func(e adapter.AgentEvent) error {
		got = e
		return nil
	})
	if err != nil || got.Type != "legacy_type" {
		t.Fatalf("type = %q, err = %v", got.Type, err)
	}
}
