package streams

import (
	"strings"
	"testing"
)

func TestDiagnosticIdentityCompleteRequiresUnchangedBoundedUTF8(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{name: "unchanged", text: "provider overloaded", want: true},
		{name: "empty", text: "", want: false},
		{name: "exact byte limit", text: strings.Repeat("é", 512), want: true},
		{name: "over byte limit", text: strings.Repeat("é", 513), want: false},
		{name: "invalid UTF-8", text: string([]byte{0xff}), want: false},
		{name: "sanitizer changes URL", text: "provider failed https://private.example/token", want: false},
		{name: "sanitizer changes whitespace", text: "provider  overloaded", want: false},
		{name: "sanitizer changes trailing punctuation", text: "provider overloaded.", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsCompleteProviderDiagnostic(tc.text); got != tc.want {
				t.Fatalf("IsCompleteProviderDiagnostic() = %t, want %t", got, tc.want)
			}
		})
	}
}

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
// gets the same routingerr.SanitizeFullUnbounded pass as error.data, not just
// URL/identifier stripping.
func TestSanitizeProviderMessageRedactsCredentials(t *testing.T) {
	cases := []struct {
		name           string
		message        string
		forbiddenLower string
	}{
		{"bearer token", "Request failed: Bearer zzzzzzzzzzzzzzzzzzzzzzzz rejected.", "zzzzzzzzzzzzzzzzzzzzzzzz"},
		{"api key literal", "--api-key zzzzzzzzzzzzzzzzzzzzzzzz invalid", "zzzzzzzzzzzzzzzzzzzzzzzz"},
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
