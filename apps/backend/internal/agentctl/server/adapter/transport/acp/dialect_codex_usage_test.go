package acp

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/replayfixtures"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestCodexUsageLimitNoticeProjectsMatchingGenericPromptError(t *testing.T) {
	const notice = "You’ve hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase credits or try again at Sep 27th, 2026 3:09 AM."
	fixture := replayfixtures.Fixture{
		AgentID: codexAgentID,
		Identity: replayfixtures.Identity{
			SessionID:        "session-1",
			ExecutionID:      "execution-1",
			PromptGeneration: 1,
		},
		Frames: []replayfixtures.Frame{
			{Kind: replayfixtures.FrameMessageChunk, Role: "assistant", Text: notice},
			{Kind: replayfixtures.FramePromptError, Code: -32603, Message: "Internal error"},
		},
	}

	_, events, requestErr, promptErr := replayFixtureThroughAdapter(t, fixture)
	if promptErr != nil {
		t.Fatalf("Adapter.Prompt error = %v, want a retained turn event", promptErr)
	}
	tokens := tokenizeEvents(events)
	if len(tokens) != 2 || tokens[0] != "message_chunk:diagnostic" || tokens[1] != "error" {
		t.Fatalf("events = %v, want the marked Codex diagnostic before the retained error", tokens)
	}
	if requestErr == nil || requestErr.Code != -32603 {
		t.Fatalf("request error = %+v, want code -32603", requestErr)
	}
	terminal := lastErrorEvent(t, events)
	if terminal.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("disposition = %q, want retain_runtime", terminal.PromptFailureDisposition)
	}
	providerErr := terminal.ProviderError
	if providerErr == nil {
		t.Fatal("ProviderErrorFromError() = nil, want the correlated Codex usage-limit notice")
	}
	if providerErr.Source != streams.ProviderErrorSourceCodexACP || providerErr.ProviderID != codexAgentID {
		t.Fatalf("provider error identity = %+v, want Codex ACP", providerErr)
	}
	if providerErr.RPCCode != -32603 {
		t.Fatalf("provider error RPC code = %d, want -32603", providerErr.RPCCode)
	}
	if !strings.Contains(strings.ToLower(providerErr.Message), "you’ve hit your usage limit") {
		t.Fatalf("provider error message = %q, want the usage-limit notice", providerErr.Message)
	}
	if strings.Contains(providerErr.Message, "chatgpt.com") {
		t.Fatalf("provider error message leaked the URL: %q", providerErr.Message)
	}
	if providerErr.ResetAt != nil {
		t.Fatalf("provider error ResetAt = %v, want nil for an unzoned time", providerErr.ResetAt)
	}
	if terminal.Error != providerErr.Message {
		t.Fatalf("retained error text = %q, want the correlated quota message %q", terminal.Error, providerErr.Message)
	}
}

func lastErrorEvent(t *testing.T, events []AgentEvent) *AgentEvent {
	t.Helper()
	var terminal *AgentEvent
	for i := range events {
		if events[i].Type == streams.EventTypeError {
			terminal = &events[i]
		}
	}
	if terminal == nil {
		t.Fatal("no error event observed")
	}
	return terminal
}

func TestCodexUsageLimitNoticeDoesNotReplaceNonGenericPromptErrors(t *testing.T) {
	const notice = "You've hit your usage limit."
	for _, tc := range []struct {
		name    string
		code    int
		message string
	}{
		{name: "specific internal error", code: -32603, message: "Internal error: connection reset"},
		{name: "different RPC code", code: -32000, message: "Internal error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := replayfixtures.Fixture{
				AgentID: codexAgentID,
				Identity: replayfixtures.Identity{
					SessionID:        "session-1",
					ExecutionID:      "execution-1",
					PromptGeneration: 1,
				},
				Frames: []replayfixtures.Frame{
					{Kind: replayfixtures.FrameMessageChunk, Role: "assistant", Text: notice},
					{Kind: replayfixtures.FramePromptError, Code: tc.code, Message: tc.message},
				},
			}
			_, events, _, promptErr := replayFixtureThroughAdapter(t, fixture)
			if promptErr != nil {
				t.Fatalf("Adapter.Prompt error = %v, want a retained turn event", promptErr)
			}
			terminal := lastErrorEvent(t, events)
			if terminal.ProviderError == nil || terminal.ProviderError.Source != streams.ProviderErrorSourceACPPrompt {
				t.Fatalf("retained provider error = %+v, want the ordinary ACP projection", terminal.ProviderError)
			}
			if strings.Contains(strings.ToLower(terminal.ProviderError.Message), "usage limit") {
				t.Fatalf("retained provider message = %q, want the ordinary ACP projection", terminal.ProviderError.Message)
			}
		})
	}
}
