package streams

import (
	"encoding/json"
	"testing"
)

func TestPromptFailureDispositionWireValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		want      PromptFailureDisposition
		wantValid bool
	}{
		{name: "retained runtime", input: `{"type":"error","prompt_failure_disposition":"retain_runtime"}`, want: PromptFailureDispositionRetainRuntime, wantValid: true},
		{name: "omitted", input: `{"type":"error"}`, wantValid: true},
		{name: "unknown", input: `{"type":"error","prompt_failure_disposition":"restart_runtime"}`, want: "restart_runtime", wantValid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var event AgentEvent
			if err := json.Unmarshal([]byte(test.input), &event); err != nil {
				t.Fatalf("unmarshal event: %v", err)
			}
			if event.PromptFailureDisposition != test.want {
				t.Fatalf("disposition = %q, want %q", event.PromptFailureDisposition, test.want)
			}
			if got := event.PromptFailureDisposition.Valid(); got != test.wantValid {
				t.Fatalf("Valid() = %v, want %v", got, test.wantValid)
			}
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("marshal event: %v", err)
			}
			var wire map[string]any
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatalf("unmarshal encoded event: %v", err)
			}
			if got := wire["prompt_failure_disposition"]; test.want == "" {
				if _, exists := wire["prompt_failure_disposition"]; exists {
					t.Fatalf("omitted disposition unexpectedly encoded as %v", got)
				}
			} else if got != string(test.want) {
				t.Fatalf("wire disposition = %v, want %q", got, test.want)
			}
		})
	}
}
