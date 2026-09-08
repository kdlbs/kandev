package streams

import (
	"strings"
	"testing"
)

// TestSanitizeProviderMessageRedactsMixedCaseURLsAndIdentifiers pins the
// case-insensitivity gap closed alongside sanitisation.go's fixture-lint
// regexes (commit d816b55c4): a gateway that emits an upper- or mixed-case
// URL scheme, or an upper-/mixed-case workspace/session/run identifier
// prefix, must be redacted exactly like the lower-case form. Checked against
// literal substrings rather than the production patterns themselves, so the
// assertion cannot pass merely because the same blind spot is on both sides.
func TestSanitizeProviderMessageRedactsMixedCaseURLsAndIdentifiers(t *testing.T) {
	cases := []struct {
		name           string
		message        string
		forbiddenLower string
	}{
		{"uppercase scheme", "Upstream failed: HTTPS://gateway.example/incident and retry.", "gateway.example"},
		{"mixed case scheme", "Upstream failed: HttpS://gateway.example/incident and retry.", "gateway.example"},
		{"uppercase wrk identifier", "Workspace WRK_abc123 could not reach the provider.", "wrk_abc123"},
		{"uppercase ses identifier", "Session SES_abc123 could not reach the provider.", "ses_abc123"},
		{"uppercase run identifier", "Run RUN_abc123 could not reach the provider.", "run_abc123"},
		{"mixed case identifier", "Session Ses_abc123 could not reach the provider.", "ses_abc123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.ToLower(SanitizeProviderMessage(tc.message))
			if strings.Contains(got, tc.forbiddenLower) {
				t.Fatalf("sanitized message still contains unredacted content %q: %q", tc.forbiddenLower, got)
			}
		})
	}
}

// TestSanitizeProviderMessageRedactsCredentials pins that a raw ACP
// RequestError.Message routed through providerErrorFromACPPrompt cannot leak
// a credential embedded by an adapter-defined error shape: the message text
// gets the same routingerr.Redact pass as error.data, not just URL/identifier
// stripping.
func TestSanitizeProviderMessageRedactsCredentials(t *testing.T) {
	cases := []struct {
		name           string
		message        string
		forbiddenLower string
	}{
		{"bearer token", "Request failed: Bearer sk-abcdEFGH12345678ijklMNOPqrstUVWX rejected.", "sk-abcdEFGH12345678ijklMNOPqrstUVWX"},
		{"api key literal", "sk-ABCDEFGHIJKL invalid", "sk-ABCDEFGHIJKL"},
		{"token key value", "auth failed token: supersecrettokenvalue", "supersecrettokenvalue"},
		{"password key value", "login rejected password=hunter2verylong", "hunter2verylong"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.ToLower(SanitizeProviderMessage(tc.message))
			if strings.Contains(got, strings.ToLower(tc.forbiddenLower)) {
				t.Fatalf("sanitized message still contains unredacted credential %q: %q", tc.forbiddenLower, got)
			}
		})
	}
}
