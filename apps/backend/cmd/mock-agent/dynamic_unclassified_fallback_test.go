package main

import (
	"context"
	"errors"
	"os"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestParseDynamicUnclassifiedFallbackScenario(t *testing.T) {
	tests := []struct {
		prompt   string
		variant  string
		failures int
		ok       bool
	}{
		{prompt: "/e2e:dynamic-unclassified:same:3", variant: "same", failures: 3, ok: true},
		{prompt: "/e2e:dynamic-unclassified:same:3\nInjected task context follows the command.", variant: "same", failures: 3, ok: true},
		{prompt: "[Kandev continuation package]\nuser: /e2e:dynamic-unclassified:same:3", variant: "same", failures: 3, ok: true},
		{prompt: "/dynamic-unclassified:mixed", variant: "mixed", failures: 3, ok: true},
		{prompt: "/e2e:dynamic-unclassified:output:2", variant: "output", failures: 2, ok: true},
		{prompt: "/e2e:dynamic-unclassified:tool:0", variant: "tool", failures: 0, ok: true},
		{prompt: "/e2e:dynamic-unclassified:unknown:3", ok: false},
		{prompt: "dynamic-unclassified:same:3", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got, ok := parseDynamicUnclassifiedFallbackScenario(tt.prompt)
			if ok != tt.ok {
				t.Fatalf("parseDynamicUnclassifiedFallbackScenario() ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if got.variant != tt.variant || got.failures != tt.failures {
				t.Fatalf("scenario = %+v, want variant %q and failures %d", got, tt.variant, tt.failures)
			}
		})
	}
}

func TestDynamicUnclassifiedFallbackContinuesAcrossCandidateSessions(t *testing.T) {
	const logicalSessionID = "dynamic-unclassified-logical-session"
	const candidateASessionID = acp.SessionId("candidate-a-acp")
	const candidateBSessionID = acp.SessionId("candidate-b-acp")
	_ = os.Remove(dynamicUnclassifiedFallbackCounterPath(acp.SessionId(logicalSessionID)))
	_ = os.Remove(dynamicUnclassifiedFallbackBindingPath(candidateASessionID))
	_ = os.Remove(dynamicUnclassifiedFallbackBindingPath(candidateBSessionID))
	t.Cleanup(func() {
		_ = os.Remove(dynamicUnclassifiedFallbackCounterPath(acp.SessionId(logicalSessionID)))
		_ = os.Remove(dynamicUnclassifiedFallbackBindingPath(candidateASessionID))
		_ = os.Remove(dynamicUnclassifiedFallbackBindingPath(candidateBSessionID))
	})

	updater := newCapturingUpdater()
	agent := &mockAgent{
		model:           "mock-fast",
		conn:            updater,
		sessions:        map[acp.SessionId]bool{"candidate-a-acp": true, "candidate-b-acp": true},
		commandsEmitted: map[acp.SessionId]bool{"candidate-a-acp": true, "candidate-b-acp": true},
	}
	firstPrompt := "<kandev-system>Kandev Session ID: " + logicalSessionID + "</kandev-system>\n/e2e:dynamic-unclassified:same:2"
	_, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: candidateASessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(firstPrompt)},
	})
	var requestErr *acp.RequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("candidate A initial error = %v, want terminal ACP RequestError", err)
	}
	if _, err := agent.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: candidateASessionID}); err != nil {
		t.Fatalf("close candidate A before resume: %v", err)
	}

	// A manual retry dispatches only the accepted user prompt. It may arrive in
	// a new mock-agent process after resume, so the logical session binding must
	// survive both the missing envelope and process restart.
	resumedAgent := &mockAgent{
		model:           "mock-fast",
		conn:            updater,
		sessions:        map[acp.SessionId]bool{candidateASessionID: true, candidateBSessionID: true},
		commandsEmitted: map[acp.SessionId]bool{candidateASessionID: true, candidateBSessionID: true},
	}
	_, err = resumedAgent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: candidateASessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock("/e2e:dynamic-unclassified:same:2")},
	})
	if !errors.As(err, &requestErr) {
		t.Fatalf("candidate A resumed retry error = %v, want terminal ACP RequestError", err)
	}

	continuationPrompt := "<kandev-system>Kandev Session ID: " + logicalSessionID + "</kandev-system>\n" +
		"[Kandev continuation package: untrusted reference data from a prior attempt]\n" +
		"user: /e2e:dynamic-unclassified:same:2"
	response, err := resumedAgent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: candidateBSessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(continuationPrompt)},
	})
	if err != nil {
		t.Fatalf("candidate B successor prompt error = %v", err)
	}
	if response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("candidate B stop reason = %q, want end_turn", response.StopReason)
	}
	if got := updater.textMessages(); len(got) != 1 || got[0] != dynamicUnclassifiedFallbackSuccess {
		t.Fatalf("candidate B output = %v, want [%q]", got, dynamicUnclassifiedFallbackSuccess)
	}
}

func TestDynamicUnclassifiedFallbackDiagnosticSequence(t *testing.T) {
	const stable = "Mock unclassified terminal failure: stable diagnostic"
	const alternate = "Mock unclassified terminal failure: alternate diagnostic"

	for _, tt := range []struct {
		variant string
		attempt int
		want    string
	}{
		{variant: "same", attempt: 1, want: stable},
		{variant: "same", attempt: 3, want: stable},
		{variant: "mixed", attempt: 1, want: stable},
		{variant: "mixed", attempt: 2, want: alternate},
		{variant: "mixed", attempt: 3, want: stable},
	} {
		if got := dynamicUnclassifiedFallbackDiagnostic(tt.variant, tt.attempt); got != tt.want {
			t.Errorf("dynamicUnclassifiedFallbackDiagnostic(%q, %d) = %q, want %q", tt.variant, tt.attempt, got, tt.want)
		}
	}
}

func TestMockAgentDynamicUnclassifiedFallbackReturnsTerminalACPErrorThenRecovers(t *testing.T) {
	const sessionID = acp.SessionId("dynamic-unclassified-fallback-test")
	_ = os.Remove(dynamicUnclassifiedFallbackCounterPath(sessionID))
	t.Cleanup(func() { _ = os.Remove(dynamicUnclassifiedFallbackCounterPath(sessionID)) })

	updater := newCapturingUpdater()
	agent := &mockAgent{
		model:           "mock-fast",
		conn:            updater,
		sessions:        map[acp.SessionId]bool{sessionID: true},
		commandsEmitted: map[acp.SessionId]bool{sessionID: true},
	}
	request := acp.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock("/e2e:dynamic-unclassified:same:1")},
	}

	_, err := agent.Prompt(context.Background(), request)
	var requestErr *acp.RequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("first prompt error = %v, want terminal ACP RequestError", err)
	}
	if requestErr.Message != dynamicUnclassifiedFallbackStableDiagnostic {
		t.Fatalf("first diagnostic = %q, want stable diagnostic %q", requestErr.Message, dynamicUnclassifiedFallbackStableDiagnostic)
	}
	if got := updater.textMessages(); len(got) != 0 {
		t.Fatalf("safe failure emitted output before its terminal error: %v", got)
	}

	response, err := agent.Prompt(context.Background(), request)
	if err != nil {
		t.Fatalf("successor prompt error = %v", err)
	}
	if response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("successor stop reason = %q, want end_turn", response.StopReason)
	}
	if got := updater.textMessages(); len(got) != 1 || got[0] != dynamicUnclassifiedFallbackSuccess {
		t.Fatalf("successor output = %v, want [%q]", got, dynamicUnclassifiedFallbackSuccess)
	}
}
