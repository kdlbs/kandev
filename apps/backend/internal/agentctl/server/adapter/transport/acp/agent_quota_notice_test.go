package acp

import (
	"context"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// A quota notice whose relative reset is still arriving must not settle the
// prompt: the first queued notice ends the turn, so it has to carry the reset.
func TestUsageLimitNoticeWaitsForCompleteResetSentence(t *testing.T) {
	a := newTestAdapterForAgent("codex-acp")
	a.sessionID = "session"
	_, turn := newPromptTurnState(context.Background(), 1, false)
	a.promptTurn = turn

	first := a.convertMessageChunkWithProtocolID("session",
		acp.TextBlock("You have reached your current quota for this period. Your limit will reset in 1 hour"), acpAssistantRole, "", 1)
	if first == nil || !first.ProviderDiagnosticCandidate {
		t.Fatalf("opening notice fragment was not marked as diagnostic: %+v", first)
	}
	select {
	case diagnostic := <-turn.providerErrorCh:
		t.Fatalf("partial notice settled the prompt: %+v", diagnostic)
	default:
	}

	before := time.Now()
	rest := a.convertMessageChunkWithProtocolID("session", acp.TextBlock(", 30 minutes."), acpAssistantRole, "", 1)
	if rest == nil || !rest.ProviderDiagnosticCandidate {
		t.Fatalf("completing notice fragment was recorded as output: %+v", rest)
	}
	select {
	case diagnostic := <-turn.providerErrorCh:
		reset := diagnostic.ProviderError.ResetAt
		if reset == nil || reset.Before(before.Add(89*time.Minute)) || reset.After(time.Now().Add(91*time.Minute)) {
			t.Fatalf("complete notice reset = %v, want about 90 minutes from now", reset)
		}
	default:
		t.Fatal("complete notice was not queued")
	}
	after := a.convertMessageChunkWithProtocolID("session", acp.TextBlock(" I will continue with the task."), acpAssistantRole, "", 1)
	if after == nil || after.ProviderDiagnosticCandidate {
		t.Fatalf("text after the delivered notice was marked as diagnostic: %+v", after)
	}
}
