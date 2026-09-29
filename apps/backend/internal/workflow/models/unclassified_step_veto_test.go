package models

import (
	"encoding/json"
	"testing"
)

func TestUnclassifiedStepVetoRoundTrip(t *testing.T) {
	const input = `{"name":"Working","position":1,"color":"blue","events":{},"disable_unclassified_fallback":true}`
	var step StepPortable
	if err := json.Unmarshal([]byte(input), &step); err != nil {
		t.Fatalf("unmarshal portable step: %v", err)
	}
	encoded, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal portable step: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal encoded step: %v", err)
	}
	raw, ok := fields["disable_unclassified_fallback"]
	if !ok {
		t.Fatal("portable step lost disable_unclassified_fallback")
	}
	var got bool
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal veto: %v", err)
	}
	if !got {
		t.Fatal("disable_unclassified_fallback = false, want true")
	}
}
