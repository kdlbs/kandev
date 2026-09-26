package streams

import (
	"encoding/json"
	"testing"
)

func TestPromptUsageJSONIncludesOutputTokensPresentFalse(t *testing.T) {
	payload, err := json.Marshal(PromptUsage{})
	if err != nil {
		t.Fatalf("marshal prompt usage: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("unmarshal prompt usage: %v", err)
	}
	value, ok := fields["output_tokens_present"]
	if !ok {
		t.Fatal("output_tokens_present is absent, want explicit false")
	}
	if value != false {
		t.Fatalf("output_tokens_present = %v, want false", value)
	}
}
