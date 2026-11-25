package streams

import (
	"encoding/json"
	"testing"
)

// TestPromptSuggestionCapabilitiesJSON pins the two independent capability
// fields: the legacy effective value and the support-only value. An explicit
// false must be present on the wire, while an absent producer stays absent.
func TestPromptSuggestionCapabilitiesJSON(t *testing.T) {
	trueValue := true
	falseValue := false
	cases := []struct {
		name    string
		event   AgentEvent
		wantRaw any // nil means the raw key must be absent
		wantEff any // nil means the effective key must be absent
	}{
		{
			name: "supported and enabled",
			event: AgentEvent{
				Type:                             EventTypeAgentCapabilities,
				SupportsPromptSuggestions:        true,
				NativePromptSuggestionsSupported: &trueValue,
			},
			wantRaw: true, wantEff: true,
		},
		{
			name: "supported but not requested",
			event: AgentEvent{
				Type:                             EventTypeAgentCapabilities,
				NativePromptSuggestionsSupported: &trueValue,
			},
			wantRaw: true, wantEff: nil,
		},
		{
			name: "explicitly unsupported",
			event: AgentEvent{
				Type:                             EventTypeAgentCapabilities,
				NativePromptSuggestionsSupported: &falseValue,
			},
			wantRaw: false, wantEff: nil,
		},
		{
			name:    "older producer without raw support",
			event:   AgentEvent{Type: EventTypeAgentCapabilities, SupportsPromptSuggestions: true},
			wantRaw: nil, wantEff: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			raw, rawPresent := decoded["native_prompt_suggestions_supported"]
			if tc.wantRaw == nil {
				if rawPresent {
					t.Fatalf("raw support = %#v, want the field absent", raw)
				}
			} else if !rawPresent || raw != tc.wantRaw {
				t.Fatalf("raw support = %#v (present=%t), want %#v", raw, rawPresent, tc.wantRaw)
			}
			effective, effectivePresent := decoded["supports_prompt_suggestions"]
			if tc.wantEff == nil {
				if effectivePresent {
					t.Fatalf("effective = %#v, want the field absent", effective)
				}
			} else if !effectivePresent || effective != tc.wantEff {
				t.Fatalf("effective = %#v (present=%t), want %#v", effective, effectivePresent, tc.wantEff)
			}
		})
	}
}
