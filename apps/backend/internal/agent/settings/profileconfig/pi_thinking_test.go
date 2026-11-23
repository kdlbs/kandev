package profileconfig

import "testing"

func TestPiThinkingIntent(t *testing.T) {
	tests := []struct {
		name, mode string
		options    map[string]string
		want       string
	}{
		{"legacy", "medium", nil, "medium"},
		{"canonical wins", "medium", map[string]string{"thought_level": "off", "unrelated": "keep"}, "off"},
		{"default", "", map[string]string{"unrelated": "keep"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, rest := PiThinkingIntent(tt.mode, tt.options, "thought_level")
			if mode != tt.want {
				t.Fatalf("thinking = %q, want %q", mode, tt.want)
			}
			if _, ok := rest["thought_level"]; ok {
				t.Fatal("alias will be applied twice")
			}
			if rest["unrelated"] != tt.options["unrelated"] {
				t.Fatal("lost unrelated option")
			}
			if tt.name == "canonical wins" && tt.options["thought_level"] != "off" {
				t.Fatal("mutated saved intent")
			}
		})
	}
}

func TestPiThinkingIntentResolvesSavedIDToAdvertisedOption(t *testing.T) {
	options := map[string]string{"thought_level": "medium", "unrelated": "keep"}
	mode, rest := PiThinkingIntent("off", options, "pi_reasoning")
	if mode != "medium" || len(rest) != 1 || rest["unrelated"] != "keep" {
		t.Fatalf("stored thinking intent = %q, remaining options = %v", mode, rest)
	}
	options["pi_reasoning"] = "off"
	mode, rest = PiThinkingIntent("high", options, "pi_reasoning")
	if mode != "off" || len(rest) != 1 || rest["unrelated"] != "keep" {
		t.Fatalf("advertised thinking intent = %q, remaining options = %v", mode, rest)
	}
}
