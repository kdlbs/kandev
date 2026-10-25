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

// TestConvertMessageChunk_ProviderDiagnosticCandidateUsesAdapterProviderID pins
// that classification is scoped to the adapter's own agent ID: a
// provider-specific high-confidence rule (claude-acp's
// claude.proxy.credentials_refused.v1) only fires when that identity reaches
// routingerr.Classify. Without it, providerRules["claude-acp"] is never
// consulted and the chunk is left unmarked even though the text matches.
func TestConvertMessageChunk_ProviderDiagnosticCandidateUsesAdapterProviderID(t *testing.T) {
	const credentialsRefusedText = `{"type":"error","error":{"type":"proxy_error","message":` +
		`"All account credentials were refused by the upstream provider. Check your OAuth entitlement."}}`

	claudeAdapter := newTestAdapterForAgent("claude-acp")
	claudeEvent := claudeAdapter.convertMessageChunk("session-1", acp.TextBlock(credentialsRefusedText), "assistant")
	if claudeEvent == nil {
		t.Fatal("expected converted assistant event")
	}
	if !claudeEvent.ProviderDiagnosticCandidate {
		t.Fatal("claude-acp adapter must classify its own provider-scoped credentials-refused rule as a diagnostic candidate")
	}

	unscoped := newTestAdapterForAgent("")
	unscopedEvent := unscoped.convertMessageChunk("session-1", acp.TextBlock(credentialsRefusedText), "assistant")
	if unscopedEvent == nil {
		t.Fatal("expected converted assistant event")
	}
	if unscopedEvent.ProviderDiagnosticCandidate {
		t.Fatal("an adapter with no provider identity must not match claude-acp's provider-scoped rule")
	}
}
