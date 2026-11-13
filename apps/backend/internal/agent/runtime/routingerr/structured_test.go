package routingerr

import (
	"fmt"
	"testing"
	"time"
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
// @covers AC-PLATFORM-RATE-THROTTLE-RETRY-001.3
func TestClassifyHTTP429PreservesKnownHardQuota(t *testing.T) {
	quota := Classify(Input{
		ProviderID: "codex-acp",
		HTTPStatus: 429,
		Stderr:     "You've hit your usage limit.",
	})
	if quota.Code != CodeQuotaLimited || quota.Class != ClassHard {
		t.Fatalf("known usage quota = %q/%q, want %q/%q", quota.Code, quota.Class, CodeQuotaLimited, ClassHard)
	}
	if quota.ResetHint != nil {
		t.Fatalf("known usage quota reset hint = %v, want none", quota.ResetHint)
	}
	if got := Decide(ContextKanban, quota, time.Now()); got != DecisionManual {
		t.Fatalf("known usage quota decision = %q, want %q", got, DecisionManual)
	}

	throttle := Classify(Input{ProviderID: "codex-acp", HTTPStatus: 429})
	if throttle.Code != CodeRateLimited {
		t.Fatalf("metadata-only HTTP 429 = %q, want %q", throttle.Code, CodeRateLimited)
	}
	if got := Decide(ContextKanban, throttle, time.Now()); got != DecisionShortRetry {
		t.Fatalf("metadata-only HTTP 429 decision = %q, want %q", got, DecisionShortRetry)
	}
}

func TestClassifyHTTP429KeepsFreeUsageNoticeAsRateLimit(t *testing.T) {
	got := Classify(Input{
		ProviderID: "opencode-acp",
		HTTPStatus: 429,
		Stderr:     "AI_APICallError: free usage limit reached",
	})
	if got.Code != CodeRateLimited || got.ClassifierRule != "http.429" {
		t.Fatalf("free usage with HTTP 429 = %q/%q, want rate_limited/http.429", got.Code, got.ClassifierRule)
	}
}

func TestHardQuotaRuleRateStatusAllowlist(t *testing.T) {
	for _, tc := range []struct {
		rule string
		want bool
	}{
		{rule: "codex.stderr.quota.v1", want: true},
		{rule: "claude.stderr.weekly_limit.v1", want: true},
		{rule: "opencode.stderr.free_usage_prefixed.v1", want: false},
		{rule: "opencode.stderr.free_usage_unlabelled.v1", want: false},
		{rule: "opencode.stderr.quota.v1", want: false},
	} {
		if got := hardQuotaRuleOutranksRateStatus(tc.rule); got != tc.want {
			t.Errorf("hardQuotaRuleOutranksRateStatus(%q) = %t, want %t", tc.rule, got, tc.want)
		}
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
