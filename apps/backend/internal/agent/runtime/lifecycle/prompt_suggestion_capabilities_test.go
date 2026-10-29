package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/mcpmode"
)

// TestPromptSuggestionCapabilitiesForwarded pins the provider-neutral
// projection: the support-only value travels unchanged beside the effective
// enablement value, and an older producer's absence stays absent.
func TestPromptSuggestionCapabilitiesForwarded(t *testing.T) {
	trueValue := true
	falseValue := false
	cases := []struct {
		name      string
		event     agentctl.AgentEvent
		wantRaw   *bool
		wantEff   bool
		wantField bool
	}{
		{
			name:      "supported and enabled",
			event:     agentctl.AgentEvent{SupportsPromptSuggestions: true, NativePromptSuggestionsSupported: &trueValue},
			wantRaw:   &trueValue,
			wantEff:   true,
			wantField: true,
		},
		{
			name:      "supported but not requested",
			event:     agentctl.AgentEvent{NativePromptSuggestionsSupported: &trueValue},
			wantRaw:   &trueValue,
			wantField: true,
		},
		{
			name:      "explicitly unsupported",
			event:     agentctl.AgentEvent{NativePromptSuggestionsSupported: &falseValue},
			wantRaw:   &falseValue,
			wantField: true,
		},
		{
			name:    "older producer without raw support",
			event:   agentctl.AgentEvent{SupportsPromptSuggestions: true},
			wantEff: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildAgentStreamEventData(tc.event)
			if data.SupportsPromptSuggestions != tc.wantEff {
				t.Fatalf("effective = %v, want %v", data.SupportsPromptSuggestions, tc.wantEff)
			}
			if (data.NativePromptSuggestionsSupported != nil) != tc.wantField {
				t.Fatalf("raw support presence = %v, want %v",
					data.NativePromptSuggestionsSupported != nil, tc.wantField)
			}
			if tc.wantRaw != nil && *data.NativePromptSuggestionsSupported != *tc.wantRaw {
				t.Fatalf("raw support = %v, want %v", *data.NativePromptSuggestionsSupported, *tc.wantRaw)
			}

			// The browser payload must preserve the same distinction, including
			// an explicit false versus an absent field.
			encoded, err := json.Marshal(data)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			raw, present := decoded["native_prompt_suggestions_supported"]
			if present != tc.wantField {
				t.Fatalf("json raw presence = %v, want %v", present, tc.wantField)
			}
			if tc.wantRaw != nil && raw != *tc.wantRaw {
				t.Fatalf("json raw support = %#v, want %v", raw, *tc.wantRaw)
			}
		})
	}
}

// TestPromptSuggestionCapabilityRecreation verifies a recreated execution reads
// the current preference and keeps the interactive-session scope. Excluded
// modes never request suggestions even when the preference is on.
func TestPromptSuggestionCapabilityRecreation(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want bool
	}{
		{name: "interactive task", mode: mcpmode.Task, want: true},
		{name: "pending title", mode: mcpmode.TaskTitlePending, want: true},
		{name: "office", mode: mcpmode.Office},
		{name: "automation", mode: mcpmode.Automation},
		{name: "configuration chat", mode: mcpmode.Config},
		{name: "coordinator", mode: mcpmode.Coordinator},
		{name: "external passthrough", mode: mcpmode.External},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.dataDir = t.TempDir()
			mgr.SetPromptSuggestionsPreference(func(context.Context) bool { return true })
			prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", &WorkspaceInfo{
				TaskID:        "task-1",
				SessionID:     "session-1",
				AgentID:       "auggie",
				ExecutorType:  "local",
				WorkspacePath: t.TempDir(),
				McpMode:       tc.mode,
			}, "execution-1")
			if err != nil {
				t.Fatalf("prepareExecutionCreateRequest() error = %v", err)
			}
			if prepared.request.PromptSuggestions != tc.want {
				t.Fatalf("PromptSuggestions = %v, want %v", prepared.request.PromptSuggestions, tc.want)
			}
		})
	}
}

// TestPromptSuggestionCapabilityRecreationReadsCurrentPreference verifies a
// replacement execution does not inherit the prior execution's request.
func TestPromptSuggestionCapabilityRecreationReadsCurrentPreference(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	pref := false
	mgr.SetPromptSuggestionsPreference(func(context.Context) bool { return pref })
	recreate := func() bool {
		t.Helper()
		prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", &WorkspaceInfo{
			TaskID:        "task-1",
			SessionID:     "session-1",
			AgentID:       "auggie",
			ExecutorType:  "local",
			WorkspacePath: t.TempDir(),
			McpMode:       mcpmode.Task,
		}, "execution-1")
		if err != nil {
			t.Fatalf("prepareExecutionCreateRequest() error = %v", err)
		}
		return prepared.request.PromptSuggestions
	}
	if recreate() {
		t.Fatal("first execution requested suggestions while the preference was off")
	}
	pref = true
	if !recreate() {
		t.Fatal("recreated execution did not read the current preference")
	}
}
