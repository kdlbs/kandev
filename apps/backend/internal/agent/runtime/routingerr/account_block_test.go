package routingerr

import "testing"

// accountBlockedDiagnostic is the complete error a gateway returned for an
// execution profile whose account it had restricted.
const accountBlockedDiagnostic = `Error from provider example-gateway: 403 Forbidden {"error":{"code":"user_blocked","message":"Your access has been restricted due to repeated policy violations."}}`

func TestAccountBlockedClassification(t *testing.T) {
	for name, text := range map[string]string{
		"observed diagnostic":                    accountBlockedDiagnostic,
		"internal error label":                   "Internal error: " + accountBlockedDiagnostic,
		"json message field with escaped quotes": `Internal error: {"message":"Error from provider example-gateway: 403 Forbidden {\"error\":{\"code\":\"user_blocked\",\"message\":\"Your access has been restricted due to repeated policy violations.\"}}"}`,
		"message without code":                   `Error from provider example-gateway: 403 Forbidden {"error":{"message":"Your access has been restricted due to repeated policy violations."}}`,
		"code without message":                   `AI_APICallError: Error from provider gateway-x: 403 Forbidden {"error":{"code":"user_blocked"}}`,
		"after earlier output":                   "partial answer\n" + accountBlockedDiagnostic,
	} {
		for _, provider := range []string{"", "opencode-acp", "claude-acp", "codex-acp"} {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: provider, Stderr: text})
			if got.Code != Code("account_blocked") || got.Confidence != ConfHigh || got.Class != ClassHard ||
				!got.FallbackAllowed || got.AutoRetryable || got.UserAction {
				t.Fatalf("%s/%q: code=%s rule=%s class=%s confidence=%s fallback=%v retry=%v user=%v, want account_blocked/hard/high/fallback",
					name, provider, got.Code, got.ClassifierRule, got.Class, got.Confidence,
					got.FallbackAllowed, got.AutoRetryable, got.UserAction)
			}

		}
	}
}

func TestAccountBlockedRejectsProseAndPlain403(t *testing.T) {
	for name, text := range map[string]string{
		"plain 403":           `Error from provider example-gateway: 403 Forbidden {"error":{"code":"forbidden","message":"Access denied."}}`,
		"non-403 with code":   `Error from provider example-gateway: 401 Unauthorized {"error":{"code":"user_blocked"}}`,
		"non-403 message":     `Error from provider example-gateway: 429 Too Many Requests {"error":{"message":"Your access has been restricted for a minute."}}`,
		"prose quoting error": `The other session failed with Error from provider example-gateway: 403 Forbidden {"error":{"code":"user_blocked"}} so I switched models.`,
		"prose about code":    "The gateway returns user_blocked when an account is restricted; your access has been restricted is the message.",
		"bare 403 forbidden":  "403 Forbidden user_blocked",
		"code on next line":   "Error from provider example-gateway: 403 Forbidden\n{\"error\":{\"code\":\"user_blocked\"}}",
	} {
		got := Classify(Input{Phase: PhasePromptSend, Stderr: text})
		if got.Code == Code("account_blocked") {
			t.Fatalf("%s: %q classified as account_blocked (%s)", name, text, got.ClassifierRule)
		}
	}
	if got := Classify(Input{Phase: PhasePromptSend, HTTPStatus: 403, Stderr: accountBlockedDiagnostic}); got.Code != CodePermissionDeniedByUser {
		t.Fatalf("structured 403 = %s, want %s", got.Code, CodePermissionDeniedByUser)
	}
}
