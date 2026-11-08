package routingerr

import (
	"net/http"
	"testing"
)

func TestClassify_OpenCodePrefixedFreeUsageLimit(t *testing.T) {
	resetInjection()
	cases := []struct {
		name   string
		notice string
	}{
		{"API call error", "AI_APICallError: free usage limit reached"},
		{"error", "Error: free usage limit reached"},
		{"internal error", "Internal error: free usage limit reached"},
		{"API error", "API Error: free usage limit reached"},
		{"case insensitive", "api error: FREE USAGE LIMIT REACHED"},
		{"whitespace", "\tInternal\t error:\tfree  usage\tlimit reached"},
		{"later diagnostic line", "provider diagnostic\n  Error: free usage limit reached"},
	}
	for _, tc := range cases {
		for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
			t.Run(tc.name+"/"+string(phase), func(t *testing.T) {
				got := Classify(Input{Phase: phase, ProviderID: "opencode-acp", Stderr: tc.notice})
				if got.Code != CodeQuotaLimited || got.Class != ClassHard || got.Confidence != ConfHigh {
					t.Fatalf("classification = %+v, want high-confidence hard quota_limited", got)
				}
				if got.ClassifierRule != "opencode.stderr.free_usage_prefixed.v1" {
					t.Fatalf("classifier rule = %s, want opencode.stderr.free_usage_prefixed.v1", got.ClassifierRule)
				}
				if !got.FallbackAllowed || !got.AutoRetryable || got.UserAction {
					t.Fatalf("quota recovery flags violated: %+v", got)
				}
				if got.ResetHint != nil {
					t.Fatalf("ResetHint = %v, want none for a notice without a reset time", got.ResetHint)
				}
			})
		}
	}
}

func TestClassify_OpenCodePrefixedFreeUsageLimitRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	cases := []struct {
		name, provider, notice string
	}{
		{"quoted API call error", "opencode-acp", "The child reported AI_APICallError: free usage limit reached"},
		{"quoted error", "opencode-acp", "The child reported Error: free usage limit reached"},
		{"quoted internal error", "opencode-acp", "The child reported Internal error: free usage limit reached"},
		{"quoted API error", "opencode-acp", "The child reported API Error: free usage limit reached"},
		{"unknown prefix", "opencode-acp", "TypeError: free usage limit reached"},
		{"incomplete notice", "opencode-acp", "AI_APICallError: free usage limit"},
		{"missing word boundary", "opencode-acp", "AI_APICallError: free usage limit reachedness"},
		{"Claude provider", "claude-acp", "AI_APICallError: free usage limit reached"},
		{"Codex provider", "codex-acp", "AI_APICallError: free usage limit reached"},
		{"Copilot provider", "copilot-acp", "AI_APICallError: free usage limit reached"},
		{"Amp provider", "amp-acp", "AI_APICallError: free usage limit reached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: tc.provider, Stderr: tc.notice})
			if got.Code != CodeAgentRuntime || got.ClassifierRule != "phase.poststart.unknown" {
				t.Fatalf("classification = %s/%s, want unmatched runtime error", got.Code, got.ClassifierRule)
			}
		})
	}
}

func TestClassify_OpenCodePrefixedFreeUsageLimitPreservesPriority(t *testing.T) {
	resetInjection()
	const notice = "AI_APICallError: free usage limit reached"
	cases := []struct {
		name, stderr string
		status       int
		wantCode     Code
		wantRule     string
	}{
		{"HTTP auth", notice, http.StatusUnauthorized, CodeAuthRequired, "http.401"},
		{"HTTP rate limit", notice, http.StatusTooManyRequests, CodeRateLimited, "http.429"},
		{"unlabelled identity", notice + "\nfree usage limit reached", 0, CodeQuotaLimited, "opencode.stderr.free_usage_unlabelled.v1"},
		{"period quota", notice + "\nError: daily usage limit reached", 0, CodeQuotaLimited, "opencode.stderr.usage_limit.v1"},
		{"credit", notice + "\nError: out of credits", 0, CodeQuotaLimited, "opencode.stderr.credit.v1"},
		{"subscription", notice + "\nError: payment required", 0, CodeSubscriptionRequired, "opencode.stderr.subscription.v1"},
		{"generic quota", notice + "\nError: insufficient_quota", 0, CodeQuotaLimited, "opencode.stderr.quota.v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{
				Phase: PhasePromptSend, ProviderID: "opencode-acp", HTTPStatus: tc.status, Stderr: tc.stderr,
			})
			if got.Code != tc.wantCode || got.ClassifierRule != tc.wantRule {
				t.Fatalf("classification = %s/%s, want %s/%s", got.Code, got.ClassifierRule, tc.wantCode, tc.wantRule)
			}
		})
	}
}
