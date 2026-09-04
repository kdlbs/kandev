package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
)

// TestConvertMessageChunk_ProviderDiagnosticCandidateAssistantRoleOnly pins
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.21: an unmarked non-empty user
// chunk cannot clear a recorded diagnostic, so the marker must never be set
// on a user-role chunk in the first place. Both roles carry the exact same
// gateway-failure text; only the assistant one may carry the marker.
func TestConvertMessageChunk_ProviderDiagnosticCandidateAssistantRoleOnly(t *testing.T) {
	const gatewayFailureText = "API Error: 500 Internal server error."

	a := newTestAdapter()

	assistantEvent := a.convertMessageChunk("session-1", acp.TextBlock(gatewayFailureText), "assistant")
	if assistantEvent == nil {
		t.Fatal("expected converted assistant event")
	}
	if !assistantEvent.ProviderDiagnosticCandidate {
		t.Fatal("assistant chunk classifying high-confidence/fallback-allowed must carry the marker")
	}

	userEvent := a.convertMessageChunk("session-1", acp.TextBlock(gatewayFailureText), "user")
	if userEvent == nil {
		t.Fatal("expected converted user event")
	}
	if userEvent.ProviderDiagnosticCandidate {
		t.Fatal("user chunk must never carry the provider diagnostic marker")
	}
}
