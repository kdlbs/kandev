package models

import (
	"encoding/json"
	"testing"
)

func TestCostEventJSONIncludesUnknownTokensOutAsNull(t *testing.T) {
	payload, err := json.Marshal(CostEvent{})
	if err != nil {
		t.Fatalf("marshal cost event: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("unmarshal cost event: %v", err)
	}
	value, ok := fields["tokens_out"]
	if !ok {
		t.Fatal("tokens_out is absent, want explicit null for an unknown value")
	}
	if value != nil {
		t.Fatalf("tokens_out = %v, want null", value)
	}
}
