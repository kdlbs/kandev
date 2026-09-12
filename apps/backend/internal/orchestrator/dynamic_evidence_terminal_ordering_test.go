package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// TestPromptAttemptPreResultSafeRepeatedTerminalEventOnlyFirstWins pins R3-F11:
// duplicate or repeated delivery of the same terminal failure event must not
// authorize recovery twice. There is no new synchronization for this — the
// existing identity fence plus clearPromptAttemptEvidence's first-write-wins
// deletion is the whole mechanism, so a redelivered event after the first
// consumer has cleared the record finds no evidence and is rejected.
func TestPromptAttemptPreResultSafeRepeatedTerminalEventOnlyFirstWins(t *testing.T) {
	var service Service
	service.beginPromptAttempt("session-1", "execution-1", 5, false)

	data := watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 5,
		ErrorMessage:     "some terminal provider failure",
	}

	first := service.withPromptAttemptEvidence(data)
	if !first.EvidenceKnown || first.OutputObserved || first.EffectObserved {
		t.Fatalf("first terminal event evidence = %+v, want known with no output/effect", first)
	}
	if !service.promptAttemptPreResultSafe(first) {
		t.Fatal("first delivery of the terminal event was not pre-result safe")
	}
	service.clearPromptAttemptEvidence("session-1", "execution-1", 5)

	second := service.withPromptAttemptEvidence(data)
	if second.EvidenceKnown {
		t.Fatal("repeated terminal event still reported known evidence after the first delivery cleared it")
	}
	if service.promptAttemptPreResultSafe(second) {
		t.Fatal("repeated terminal event was treated as pre-result safe a second time")
	}
}

// TestObserveProviderDiagnosticAfterTerminalFailureClearedIsNoOp pins R3-F11's
// other ordering case: a diagnostic notification that arrives after the
// terminal failure has already been observed and cleared must not resurrect
// or fabricate evidence for that execution/generation. observeProviderDiagnostic
// only writes through promptAttemptForSession's Load, never LoadOrStore, so a
// cleared record stays absent.
func TestObserveProviderDiagnosticAfterTerminalFailureClearedIsNoOp(t *testing.T) {
	var service Service
	const diagnostic = "API Error: Repeated 529 Overloaded errors. The API is at capacity."

	service.beginPromptAttempt("session-1", "execution-1", 5, false)
	service.clearPromptAttemptEvidence("session-1", "execution-1", 5)

	// Late-arriving diagnostic notification for the already-cleared attempt.
	service.observeProviderDiagnostic("session-1", "execution-1", 5, diagnostic)

	got := service.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 5,
		ErrorMessage:     diagnostic,
	})
	if got.EvidenceKnown {
		t.Fatal("late provider diagnostic notification resurrected evidence cleared by an earlier terminal failure")
	}
}
