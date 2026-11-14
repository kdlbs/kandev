package acp

import (
	"encoding/json"
	"testing"
)

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-004.1, AC-AGENTS-SUBAGENT-LIFECYCLE-004.2
func TestNormalizeSubagentControlLabels(t *testing.T) {
	codexInput := map[string]any{
		"senderThreadId":    "parent-thread",
		"receiverThreadIds": []any{"child-thread"},
		"agentsStates":      map[string]any{},
	}
	claudeMeta := map[string]any{"claudeCode": map[string]any{"toolName": "SendMessage"}}
	tests := []struct {
		name     string
		provider string
		title    string
		meta     map[string]any
		input    any
		want     string
	}{
		{name: "Codex wait", provider: codexAgentID, title: "wait", input: codexInput, want: "wait"},
		{name: "Codex close", provider: codexAgentID, title: "closeAgent", input: codexInput, want: "close"},
		{name: "Codex send input", provider: codexAgentID, title: "sendInput", input: codexInput, want: "message"},
		{name: "Codex followup", provider: codexAgentID, title: "followupTask", input: codexInput, want: "message"},
		{name: "Codex resume", provider: codexAgentID, title: "resumeAgent", input: codexInput, want: "resume"},
		{name: "Claude SendMessage metadata", provider: claudeAgentID, title: "SendMessage", meta: claudeMeta, want: "message"},
		{name: "non-Codex wait retains generic title", provider: "cursor-acp", title: "wait", input: codexInput},
		{name: "Codex wait without collaboration fields", provider: codexAgentID, title: "wait", input: map[string]any{"senderThreadId": "parent-thread"}},
		{name: "Claude title without trusted metadata", provider: claudeAgentID, title: "SendMessage"},
		{name: "unknown Codex tool", provider: codexAgentID, title: "execute", input: codexInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"title": tt.title, "raw_input": tt.input}
			if tt.meta != nil {
				args["meta"] = tt.meta
			}
			payload := NewNormalizer(tt.provider).NormalizeToolCall("other", args)
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Generic map[string]any `json:"generic"`
			}
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Generic == nil {
				t.Fatalf("normalized payload has no generic tool: %s", encoded)
			}
			got, _ := wire.Generic["subagent_control"].(string)
			if got != tt.want {
				t.Fatalf("subagent_control = %q, want %q; payload %s", got, tt.want, encoded)
			}
		})
	}
}
