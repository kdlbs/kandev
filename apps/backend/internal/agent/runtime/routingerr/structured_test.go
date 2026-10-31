package routingerr

import (
	"fmt"
	"testing"
)

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.32
func TestClassifyStructuredProviderSignals(t *testing.T) {
	for _, test := range []struct {
		name     string
		in       Input
		wantCode Code
		wantRule string
		notCode  Code // for negative cases: the structured signal must not imply this code
	}{
		{
			name:     "acp authentication required",
			in:       Input{ProviderID: "codex-acp", ACPCode: -32000, Stderr: "Internal error"},
			wantCode: CodeAuthRequired, wantRule: "acp.authentication_required",
		},
		{
			name:    "json-rpc internal error is not authentication",
			in:      Input{ProviderID: "codex-acp", ACPCode: -32603, Stderr: "Internal error"},
			notCode: CodeAuthRequired,
		},
		{
			name:     "claude namespaced rate limit kind",
			in:       Input{ProviderID: "claude-acp", ErrorKind: "rate_limit_error", Stderr: "Internal error"},
			wantCode: CodeRateLimited, wantRule: "anthropic.kind.rate_limit",
		},
		{
			name:    "kind from another namespace is ignored",
			in:      Input{ProviderID: "codex-acp", ErrorKind: "rate_limit_error", Stderr: "Internal error"},
			notCode: CodeRateLimited,
		},
		{
			name:     "opencode provider auth kind",
			in:       Input{ProviderID: "opencode-acp", ErrorKind: "ProviderAuthError", Stderr: "Internal error"},
			wantCode: CodeAuthRequired, wantRule: "opencode.kind.provider_auth",
		},
		{
			name:     "unmapped http error is not relabelled by prose",
			in:       Input{ProviderID: "claude-acp", HTTPStatus: 418, Stderr: "Invalid API key"},
			wantCode: CodeUnknownProvider, wantRule: "http.418",
		},
		{
			name:    "opaque provider code carries no meaning",
			in:      Input{ProviderID: "claude-acp", ProviderCode: "invalid_api_key", Stderr: "Internal error"},
			notCode: CodeAuthRequired,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Classify(test.in)
			if test.notCode != "" {
				if got.Code == test.notCode {
					t.Fatalf("code = %q (rule %q), structured signal must not imply it", got.Code, got.ClassifierRule)
				}
				return
			}
			if got.Code != test.wantCode {
				t.Fatalf("code = %q (rule %q), want %q", got.Code, got.ClassifierRule, test.wantCode)
			}
			if test.wantRule != "" && got.ClassifierRule != test.wantRule {
				t.Fatalf("rule = %q, want %q", got.ClassifierRule, test.wantRule)
			}
		})
	}
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.32
func TestClassifyKeepsAlreadyNormalizedError(t *testing.T) {
	normalized := &Error{Code: CodeQuotaLimited, Confidence: ConfHigh, ClassifierRule: "upstream.rule"}
	got := Classify(Input{StructuredErr: fmt.Errorf("wrapped: %w", normalized), Stderr: "Rate limit exceeded"})
	if got != normalized {
		t.Fatalf("Classify = %+v, want the wrapped normalized error unchanged", got)
	}
}
