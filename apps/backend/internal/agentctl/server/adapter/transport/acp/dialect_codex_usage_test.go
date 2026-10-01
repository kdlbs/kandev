package acp

import (
	"errors"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
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

	a, events, promptErr := replayFixtureThroughAdapter(t, fixture)
	if promptErr == nil {
		t.Fatal("Adapter.Prompt returned nil, want the generic ACP prompt error")
	}
	if len(events) != 1 || events[0] != "message_chunk:diagnostic" {
		t.Fatalf("events = %v, want the marked Codex diagnostic before the terminal error", events)
	}
	var requestErr *acp.RequestError
	if !errors.As(promptErr, &requestErr) {
		t.Fatalf("prompt error = %v, want the wrapped ACP request error to remain discoverable", promptErr)
	}
	if requestErr.Code != -32603 {
		t.Fatalf("wrapped request code = %d, want -32603", requestErr.Code)
	}
	if !strings.Contains(strings.ToLower(promptErr.Error()), "you’ve hit your usage limit") {
		t.Fatalf("prompt error text = %q, want the notice for queued-prompt recovery", promptErr.Error())
	}
	providerID, modelID := a.ProviderErrorContext()
	providerErr := ProviderErrorFromError(promptErr, providerID, modelID)
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
			a, _, promptErr := replayFixtureThroughAdapter(t, fixture)
			if promptErr == nil {
				t.Fatal("Adapter.Prompt returned nil, want the prompt error")
			}
			providerID, modelID := a.ProviderErrorContext()
			providerErr := ProviderErrorFromError(promptErr, providerID, modelID)
			if providerErr == nil || providerErr.Source == streams.ProviderErrorSourceCodexACP {
				t.Fatalf("ProviderErrorFromError() = %+v, want the ordinary ACP projection", providerErr)
			}
		})
	}
}
