package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// TestDynamicAttemptEvidenceRequiresDiagnosticTextContainment pins
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.23: a recorded diagnostic that
// classifies to the terminal failure's code but whose normalized text is not
// contained in the terminal message must keep the output fence. This is the
// "prose-matched" scenario from the peer-triaged defect: an assistant
// sentence narrating a 529 classifies identically to the terminal failure but
// is not itself the transport diagnostic.
func TestDynamicAttemptEvidenceRequiresDiagnosticTextContainment(t *testing.T) {
	var service Service
	const prose = "I ran the build and the upstream returned 529 because the provider is overloaded; retrying now."
	const terminal = "API Error: Repeated 529 Overloaded errors. The API is at capacity."

	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	service.observeProviderDiagnostic("session-1", "execution-1", 1, prose)

	got := service.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 1,
		ErrorMessage:     terminal,
	})
	if !got.OutputObserved {
		t.Fatal("prose narrating the same code without containment was treated as a transport diagnostic")
	}
	if service.promptAttemptPreResultSafe(got) {
		t.Fatal("prose without containment was incorrectly treated as pre-result safe")
	}
}

// TestDynamicAttemptEvidenceContainmentIsCaseSensitive pins AC.23's
// case-sensitive comparison rule. Both catalogue rules feeding this path are
// case-insensitive, so containment is the only remaining discriminator: a
// title-cased diagnostic chunk classifies identically to the terminal
// failure's lower-cased text but must not satisfy containment.
func TestDynamicAttemptEvidenceContainmentIsCaseSensitive(t *testing.T) {
	var service Service
	const chunk = "API Error: 500 Internal Server Error."
	const terminal = "Internal error: API Error: 500 Internal server error. This is a server-side issue, usually temporary - try again in a moment."

	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	service.observeProviderDiagnostic("session-1", "execution-1", 1, chunk)

	got := service.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 1,
		ErrorMessage:     terminal,
	})
	if !got.OutputObserved {
		t.Fatal("case-mismatched diagnostic was incorrectly treated as contained in the terminal message")
	}
	if service.promptAttemptPreResultSafe(got) {
		t.Fatal("case-mismatched diagnostic was incorrectly treated as pre-result safe")
	}
}

// TestDynamicAttemptEvidenceContainmentSurvivesSanitizedTrailingPunctuation
// pins the flagship "matched" scenario from the system design's input
// inventory. The diagnostic chunk arrives raw from the ACP stream and keeps
// its trailing period, but by the time the terminal provider failure reaches
// this fence its message has already been through sanitizeProviderMessage,
// which trims trailing punctuation. Containment must still hold for text
// that is otherwise identical, or the exact scenario the design is built
// around would never authorize automatic recovery.
func TestDynamicAttemptEvidenceContainmentSurvivesSanitizedTrailingPunctuation(t *testing.T) {
	var service Service
	const rawDiagnostic = "API Error: Repeated 529 Overloaded errors. The API is at capacity."
	const sanitizedTerminal = "API Error: Repeated 529 Overloaded errors. The API is at capacity"

	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	service.observeProviderDiagnostic("session-1", "execution-1", 1, rawDiagnostic)

	got := service.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 1,
		ErrorMessage:     sanitizedTerminal,
	})
	if got.OutputObserved {
		t.Fatal("diagnostic differing from the terminal message only by sanitized trailing punctuation was treated as generated output")
	}
	if !service.promptAttemptPreResultSafe(got) {
		t.Fatal("diagnostic differing from the terminal message only by sanitized trailing punctuation was not pre-result safe")
	}
}

// TestDynamicAttemptEvidenceContainmentSatisfiedByGatewaySubstring pins the
// gateway-500 sample from the system design's input inventory: the chunk text
// is a strict substring of the sanitized terminal message, and containment
// must hold for that case exactly as it holds for the text-identical
// overloaded pair (already covered by
// TestDynamicAttemptEvidenceTreatsMatchingACPProviderDiagnosticAsPreResult).
func TestDynamicAttemptEvidenceContainmentSatisfiedByGatewaySubstring(t *testing.T) {
	var service Service
	const chunk = "API Error: 500 Internal server error."
	const terminal = "Internal error: API Error: 500 Internal server error. This is a server-side issue, usually temporary - try again in a moment."

	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	service.observeProviderDiagnostic("session-1", "execution-1", 1, chunk)

	got := service.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 1,
		ErrorMessage:     terminal,
	})
	if got.OutputObserved {
		t.Fatal("gateway diagnostic contained in the terminal message was treated as generated output")
	}
	if !service.promptAttemptPreResultSafe(got) {
		t.Fatal("gateway diagnostic contained in the terminal message was not pre-result safe")
	}
}
