package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

// These tests pin the shared run and run-event wire contract.
func TestRunJSONContract(t *testing.T) {
	run := Run{
		ID: "run-1", AgentProfileID: "agent-1", Status: RunStatusQueued,
		ActorKind: ActorKindAgent, ActorID: "agent-1", PriorityClass: PriorityClassEvent,
	}
	payload, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode run JSON: %v", err)
	}
	for key, want := range map[string]any{
		"id": "run-1", "status": "queued", "actor_kind": "agent", "priority_class": float64(2),
	} {
		if got[key] != want {
			t.Errorf("JSON %s = %#v, want %#v", key, got[key], want)
		}
	}
	if _, ok := got["routing_blocked_status"]; ok {
		t.Error("nil routing_blocked_status must remain omitted")
	}
	if field, ok := reflect.TypeOf(run).FieldByName("Status"); !ok || field.Tag.Get("db") != "status" {
		t.Errorf("Run.Status db tag = %q, want %q", field.Tag.Get("db"), "status")
	}
}

func TestRunEventJSONContract(t *testing.T) {
	event := RunEvent{RunID: "run-1", Seq: 3, EventType: RunEventTypeStep, Level: RunEventLevelInfo}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal run event: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode run event JSON: %v", err)
	}
	for key, want := range map[string]any{
		"run_id": "run-1", "seq": float64(3), "event_type": "step", "level": "info",
	} {
		if got[key] != want {
			t.Errorf("JSON %s = %#v, want %#v", key, got[key], want)
		}
	}
}

func TestRunEnumContractMethods(t *testing.T) {
	if !ActorKindAgent.Valid() || ActorKind("unknown").Valid() {
		t.Fatal("ActorKind.Valid contract changed")
	}
	if got := PriorityClassEvent.String(); got != "event" {
		t.Fatalf("PriorityClassEvent.String() = %q, want event", got)
	}
	if got := RoutingBlockedWaitingForCapacity.String(); got != "waiting_for_provider_capacity" {
		t.Fatalf("RoutingBlockedWaitingForCapacity.String() = %q", got)
	}
}
