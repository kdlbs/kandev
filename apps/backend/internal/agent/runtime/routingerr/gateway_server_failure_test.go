package routingerr

import "testing"

const teamClaudeGateway500 = `{"code":-32603,"message":"Internal error: API Error: 500 Internal server error. This is a server-side issue, usually temporary - try again in a moment.","data":{"errorKind":"server_error"}}`

func TestClassifyACPWrappedGatewayServerFailures(t *testing.T) {
	resetInjection()
	cases := []struct {
		name string
		msg  string
	}{
		{"teamclaude 500", teamClaudeGateway500},
		{"bad gateway", `API Error: 502 Bad Gateway`},
		{"gateway timeout", `API Error: 504 Gateway Timeout`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: tc.msg})
			if got.Code != CodeProviderUnavailable || got.Class != ClassTransient {
				t.Fatalf("classification = %+v, want transient provider_unavailable", got)
			}
			if got.ClassifierRule != gatewayServerFailureRuleID || got.Confidence != ConfHigh {
				t.Fatalf("rule/confidence = %q/%q, want %q/high", got.ClassifierRule, got.Confidence, gatewayServerFailureRuleID)
			}
			if !got.AutoRetryable || !got.FallbackAllowed || got.UserAction {
				t.Fatalf("transient invariants violated: %+v", got)
			}
		})
	}
}

func TestClassifyACPWrappedGatewayServerFailuresRejectsUntrustedProse(t *testing.T) {
	resetInjection()
	for _, msg := range []string{
		"internal server error",
		"API Error: 500 invalid request",
		"the task mentioned status 500 Internal server error",
		"received status 502 from a local test server",
	} {
		got := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: msg})
		if got.Code == CodeProviderUnavailable {
			t.Fatalf("%q classified as provider unavailable: %+v", msg, got)
		}
	}
}

// TestClassifyACPWrappedGatewayServerFailuresProseQuotingWrapperFormClassifies
// pins the true, unfixed behaviour of gatewayServerFailureRe (R3-F3):
// requirements .17 calls the rule "anchored to the exact ACP wrapper form", but
// it is only word-boundaried, so narrative prose that merely quotes the wrapper
// form still matches and classifies. The requirement forbids tightening the
// regex, so this records the false-positive surface rather than asserting a
// rejection that does not hold.
func TestClassifyACPWrappedGatewayServerFailuresProseQuotingWrapperFormClassifies(t *testing.T) {
	resetInjection()
	for _, msg := range []string{
		"the agent said: API Error: 500 Internal Server Error, please retry",
		"summary: encountered API Error: 502 Bad Gateway while building",
	} {
		got := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: msg})
		if got.Code != CodeProviderUnavailable || got.Class != ClassTransient {
			t.Fatalf("%q classification = %+v, want transient provider_unavailable (unanchored regex still matches prose quoting the wrapper form)", msg, got)
		}
	}
}

func TestClassifyProxyCredentialsRefusedIsHard(t *testing.T) {
	resetInjection()
	got := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "claude-acp",
		Stderr:     `{"type":"error","error":{"type":"proxy_error","message":"All account credentials were refused by the upstream provider. Check your OAuth entitlement."}}`,
	})
	if got.Code != CodeMissingCredentials || got.Class != ClassHard {
		t.Fatalf("classification = %+v, want hard missing_credentials", got)
	}
	if !got.UserAction || got.AutoRetryable || !got.FallbackAllowed {
		t.Fatalf("hard-error invariants violated: %+v", got)
	}
}
