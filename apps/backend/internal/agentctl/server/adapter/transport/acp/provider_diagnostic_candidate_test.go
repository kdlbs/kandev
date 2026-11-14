package acp

import (
	"testing"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func TestProviderDiagnosticCandidateMatchesClassify(t *testing.T) {
	const claudeAgentID = "claude-acp"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	marked := 0
	for _, text := range []string{
		"I'll read the scheduler next.",
		"API Error: 429 rate_limit_error",
		"API Error: 529 overloaded_error",
		"The tests pass.",
	} {
		classified := routingerr.Classify(routingerr.Input{
			Phase: routingerr.PhasePromptSend, ProviderID: claudeAgentID, Stderr: text,
		})
		want := classified.Confidence == routingerr.ConfHigh && classified.FallbackAllowed
		ev := a.convertMessageChunk("session-1", sdk.TextBlock(text), "assistant")
		if ev == nil {
			t.Fatalf("convertMessageChunk(%q) = nil", text)
		}
		if ev.ProviderDiagnosticCandidate != want {
			t.Errorf("candidate(%q) = %v, want %v", text, ev.ProviderDiagnosticCandidate, want)
		}
		if ev.ProviderDiagnosticCandidate {
			marked++
		}
	}
	if marked < 2 {
		t.Fatalf("marked %d provider diagnostics, want the rate-limit and overload lines", marked)
	}
}
