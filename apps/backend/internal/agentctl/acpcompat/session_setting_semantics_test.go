package acpcompat

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"testing"
)

func TestSessionSettingSemantics(t *testing.T) {
	thought := streams.ConfigOption{ID: "thought_level", Type: "select", Category: "thought_level"}
	permission := streams.ConfigOption{ID: "approval", Type: "select", Category: "mode"}
	tests := []struct {
		name, agent, want string
		options           []streams.ConfigOption
	}{
		{"Pi aliases thinking", PiAgentID, "thought_level", []streams.ConfigOption{thought}},
		{"another agent does not alias thinking", "custom", "", []streams.ConfigOption{thought}},
		{"permission retained", "claude-code", "approval", []streams.ConfigOption{thought, permission}},
		{"ambiguous Pi options", "pi-acp", "", []streams.ConfigOption{thought, {ID: "other", Type: "select", Category: "thought_level"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			option, ok := SessionModeOption(tt.agent, tt.options)
			if ok != (tt.want != "") || (ok && option.ID != tt.want) {
				t.Fatalf("got %+v, %v", option, ok)
			}
		})
	}
}
