package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// gatewayServerFailureSample is a text sample that classifies as a
// high-confidence, fallback-allowed provider diagnostic on its own — used to
// prove that handleAgentStreamEvent's message_streaming dispatch trusts the
// carried ProviderDiagnosticCandidate marker rather than re-deriving that
// classification itself from the chunk text
// (AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20).
const gatewayServerFailureSample = "API Error: 500 Internal server error."

// TestHandleAgentStreamEvent_MessageStreamingHonorsUnmarkedProviderDiagnosticText
// proves the orchestrator no longer re-derives the provider-diagnostic
// classification from message_streaming text: a chunk whose text would
// classify as a high-confidence diagnostic on its own, but which arrives
// without the carried marker (as a non-assistant chunk would from the ACP
// conversion boundary), must be treated as ordinary output.
func TestHandleAgentStreamEvent_MessageStreamingHonorsUnmarkedProviderDiagnosticText(t *testing.T) {
	svc, _ := newTransientTestService(t)
	armTransientPromptEvidence(svc)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                        "message_streaming",
			MessageID:                   "msg-1",
			Text:                        gatewayServerFailureSample,
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: false,
		},
	})

	got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     gatewayServerFailureSample,
	})
	if !got.OutputObserved {
		t.Fatal("unmarked chunk was treated as a transport diagnostic by re-deriving classification from text")
	}
}

// TestHandleAgentStreamEvent_MessageStreamingTracksMarkedProviderDiagnosticForCorrelation
// proves a marked chunk still feeds the AC.23 diagnostic-code/text correlation
// fence: once handleAgentStreamEvent reads the carried marker, a terminal
// failure whose normalized message contains the recorded diagnostic text stays
// safe to automatically retry.
func TestHandleAgentStreamEvent_MessageStreamingTracksMarkedProviderDiagnosticForCorrelation(t *testing.T) {
	svc, _ := newTransientTestService(t)
	armTransientPromptEvidence(svc)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                        "message_streaming",
			MessageID:                   "msg-1",
			Text:                        gatewayServerFailureSample,
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: true,
		},
	})

	got := svc.withPromptAttemptEvidence(watcher.AgentEventData{
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     "Internal error: " + gatewayServerFailureSample + " This is a server-side issue, usually temporary.",
	})
	if got.OutputObserved {
		t.Fatal("marked provider-diagnostic chunk contained in the terminal message was treated as generated output")
	}
	if !svc.promptAttemptPreResultSafe(got) {
		t.Fatal("marked provider-diagnostic chunk contained in the terminal message was not pre-result safe")
	}
}
