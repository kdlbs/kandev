package acp

import (
	"context"
	"errors"
	"strings"
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
	if err := turn.quotaNoticeOnNormalPromptReturn("session"); err != nil {
		t.Fatalf("output after the notice did not clear it for a normal return: %v", err)
	}
}

// Assistant output that follows a relative reset phrase, with or without a
// sentence terminator, is never held as part of the notice.
func TestRelativeQuotaNoticeDoesNotConsumeAssistantOutput(t *testing.T) {
	const opening = "You have reached your current quota for this period. Your limit will reset in 1 hour"
	const ending = ", 30 minutes."
	const answer = " Here is the requested answer."
	for _, tc := range []struct {
		name   string
		chunks []string
	}{
		{"mixed continuation", []string{opening, ending + answer}},
		{"mixed notice", []string{opening + ending + answer}},
		{"undelimited output", []string{opening, answer}},
		{"undelimited streaming output", []string{opening, strings.TrimSuffix(answer, ".")}},
		{"output after notice", []string{opening, ending, answer}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAdapterForAgent("codex-acp")
			a.sessionID = "session"
			_, turn := newPromptTurnState(context.Background(), 1, false)
			a.promptTurn = turn

			var event *AgentEvent
			for _, chunk := range tc.chunks {
				event = a.convertMessageChunkWithProtocolID("session", acp.TextBlock(chunk), acpAssistantRole, "", 1)
			}
			if event == nil || event.ProviderDiagnosticCandidate {
				t.Fatalf("ordinary output was marked as provider diagnostic evidence: %+v", event)
			}
			if err := turn.quotaNoticeOnNormalPromptReturn("session"); err != nil {
				t.Fatalf("quota notice overrode a prompt that produced output: %v", err)
			}
		})
	}
}

// A notice whose duration is split inside a unit stays diagnostic, and a prompt
// that returns before the delivered notice is consumed settles from it.
func TestRelativeQuotaNoticeSplitDurationSettlesNormalReturn(t *testing.T) {
	a := newTestAdapterForAgent("codex-acp")
	a.sessionID = "session"
	_, turn := newPromptTurnState(context.Background(), 1, false)
	a.promptTurn = turn

	for _, chunk := range []string{"You have reached your current quota for this period. Your limit will reset in 1 hour", ", 30 mi", "nutes."} {
		event := a.convertMessageChunkWithProtocolID("session", acp.TextBlock(chunk), acpAssistantRole, "", 1)
		if event == nil || !event.ProviderDiagnosticCandidate {
			t.Fatalf("valid duration fragment %q was marked as ordinary output", chunk)
		}
	}
	select {
	case diagnostic := <-turn.providerErrorCh:
		if diagnostic.ProviderError.ResetAt == nil ||
			diagnostic.ProviderError.ResetAt.Sub(diagnostic.ProviderError.OccurredAt).Round(time.Second) != 90*time.Minute {
			t.Fatalf("the diagnostic lost the complete reset duration: %+v", diagnostic.ProviderError)
		}
	default:
		t.Fatal("the completed notice did not produce a diagnostic")
	}
	var providerErr *providerPromptError
	if err := turn.quotaNoticeOnNormalPromptReturn("session"); !errors.As(err, &providerErr) {
		t.Fatalf("normal return after a completed notice = %v, want provider error", err)
	}
}

// A stream that ends on a whole duration without its terminator is a complete
// notice once the prompt returns.
func TestRelativeQuotaNoticeUnterminatedSettlesNormalReturn(t *testing.T) {
	a := newTestAdapterForAgent("codex-acp")
	a.sessionID = "session"
	_, turn := newPromptTurnState(context.Background(), 1, false)
	a.promptTurn = turn

	event := a.convertMessageChunkWithProtocolID("session",
		acp.TextBlock("You have reached your current quota for this period. Your limit will reset in 2 hours"), acpAssistantRole, "", 1)
	if event == nil || !event.ProviderDiagnosticCandidate {
		t.Fatalf("held notice was marked as ordinary output: %+v", event)
	}
	var providerErr *providerPromptError
	if err := turn.quotaNoticeOnNormalPromptReturn("session"); !errors.As(err, &providerErr) {
		t.Fatalf("normal return on a held notice = %v, want provider error", err)
	}
	reset := providerErr.ProviderError.ResetAt
	if reset == nil || reset.Sub(providerErr.ProviderError.OccurredAt).Round(time.Second) != 2*time.Hour {
		t.Fatalf("held notice reset = %v, want two hours after it was observed", reset)
	}
}
