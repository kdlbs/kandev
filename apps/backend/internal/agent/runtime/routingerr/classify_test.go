package routingerr

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

func resetInjection() {
	injectOnce = sync.Once{}
	injectMap = nil
}

func TestClassify_HTTPStatusMapping(t *testing.T) {
	resetInjection()
	cases := []struct {
		status int
		want   Code
	}{
		{http.StatusUnauthorized, CodeAuthRequired},
		{http.StatusForbidden, CodePermissionDeniedByUser},
		{http.StatusPaymentRequired, CodeSubscriptionRequired},
		{http.StatusTooManyRequests, CodeRateLimited},
		{http.StatusInternalServerError, CodeProviderUnavailable},
		{http.StatusBadGateway, CodeProviderUnavailable},
		{http.StatusGatewayTimeout, CodeProviderUnavailable},
		{http.StatusServiceUnavailable, CodeProviderUnavailable},
	}
	for _, c := range cases {
		got := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", HTTPStatus: c.status})
		if got.Code != c.want {
			t.Fatalf("status %d → %s, want %s", c.status, got.Code, c.want)
		}
		if got.Confidence != ConfHigh {
			t.Fatalf("status %d confidence %s want high", c.status, got.Confidence)
		}
	}
}

func TestClassify_ExitCode127(t *testing.T) {
	resetInjection()
	exit := 127
	e := Classify(Input{Phase: PhaseProcessStart, ProviderID: "claude-acp", ExitCode: &exit})
	if e.Code != CodeProviderNotConfigured {
		t.Fatalf("got %s, want provider_not_configured", e.Code)
	}
	if !e.UserAction || !e.FallbackAllowed || e.AutoRetryable {
		t.Fatalf("invariants violated: %+v", e)
	}
}

func TestClassify_ProviderRules(t *testing.T) {
	resetInjection()
	cases := []struct {
		name       string
		providerID string
		stderr     string
		wantCode   Code
	}{
		{"claude quota", "claude-acp", "Error: anthropic_quota_exceeded for user", CodeQuotaLimited},
		{"claude rate", "claude-acp", "you hit the rate-limit", CodeRateLimited},
		{
			"claude proxy credentials refused",
			"claude-acp",
			`{"type":"error","error":{"type":"proxy_error","message":"All account credentials were refused by the upstream provider. Check your OAuth entitlement."}}`,
			CodeMissingCredentials,
		},
		{"claude auth", "claude-acp", "you are not authenticated", CodeAuthRequired},
		{"claude model", "claude-acp", "model claude-foo not found here", CodeModelUnavailable},
		{"codex quota", "codex-acp", "insufficient_quota for project", CodeQuotaLimited},
		{
			"codex usage limit",
			"codex-acp",
			`{"code":-32603,"message":"Internal error","data":{"codexErrorInfo":"usageLimitExceeded","message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 1st, 2026 3:14 PM."}}`,
			CodeQuotaLimited,
		},
		{"codex rate", "codex-acp", "rate_limit_exceeded", CodeRateLimited},
		{"codex apikey", "codex-acp", "invalid api key provided", CodeMissingCredentials},
		{"opencode auth", "opencode-acp", "Unauthorized request", CodeAuthRequired},
		{"copilot subscription", "copilot-acp", "user is not entitled to Copilot", CodeSubscriptionRequired},
		{"copilot signin", "copilot-acp", "please sign in via gh auth login", CodeAuthRequired},
		{"amp auth", "amp-acp", "Invalid Token returned by server", CodeAuthRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Classify(Input{Phase: PhaseSessionInit, ProviderID: c.providerID, Stderr: c.stderr})
			if e.Code != c.wantCode {
				t.Fatalf("%s → %s, want %s (rule=%s)", c.name, e.Code, c.wantCode, e.ClassifierRule)
			}
		})
	}
}

func TestClassify_ProviderNeutralTransientSignals(t *testing.T) {
	resetInjection()
	cases := []struct {
		name     string
		provider string
		stderr   string
		want     Code
	}{
		{
			name:     "model capacity",
			provider: "codex-acp",
			stderr:   "Selected model is at capacity. Please try a different model.",
			want:     Code("model_capacity"),
		},
		{
			name:     "network unavailable",
			provider: "claude-acp",
			stderr:   "connect: network is unreachable",
			want:     Code("network_unavailable"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{
				Phase:      PhasePromptSend,
				ProviderID: tc.provider,
				Stderr:     tc.stderr,
			})
			if got.Code != tc.want {
				t.Fatalf("classification = %q, want %q (rule=%s)", got.Code, tc.want, got.ClassifierRule)
			}
			if !got.AutoRetryable || !got.FallbackAllowed || got.UserAction {
				t.Fatalf("transient invariants violated: %+v", got)
			}
		})
	}
}

func TestClassify_PrestartPhaseFallback(t *testing.T) {
	resetInjection()
	for _, p := range []Phase{PhaseAuthCheck, PhaseProcessStart, PhaseSessionInit} {
		e := Classify(Input{Phase: p, ProviderID: "unknown-provider", Stderr: "weird gibberish"})
		if e.Code != CodeUnknownProvider {
			t.Fatalf("phase %s → %s, want unknown_provider_error", p, e.Code)
		}
		if !e.FallbackAllowed || !e.AutoRetryable {
			t.Fatalf("phase %s invariants violated: %+v", p, e)
		}
		if e.Confidence != ConfLow {
			t.Fatalf("phase %s confidence %s want low", p, e.Confidence)
		}
	}
}

func TestClassify_PoststartAmbiguousNoFallback(t *testing.T) {
	resetInjection()
	for _, p := range []Phase{PhasePromptSend, PhaseStreaming, PhaseToolExecution, PhaseShutdown} {
		e := Classify(Input{Phase: p, ProviderID: "claude-acp", Stderr: "unexpected glitch"})
		if e.Code != CodeAgentRuntime {
			t.Fatalf("phase %s → %s, want agent_runtime_error", p, e.Code)
		}
		if e.FallbackAllowed {
			t.Fatalf("phase %s must not fall back", p)
		}
	}
}

func TestClassify_InvariantsAuthRequired(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "not authenticated"})
	if e.Code != CodeAuthRequired {
		t.Fatalf("got %s", e.Code)
	}
	if !e.FallbackAllowed || e.AutoRetryable || !e.UserAction {
		t.Fatalf("auth_required invariants violated: %+v", e)
	}
}

func TestClassify_InvariantsModelUnavailable(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "model claude-x not found"})
	if e.Code != CodeModelUnavailable {
		t.Fatalf("got %s", e.Code)
	}
	if !e.FallbackAllowed {
		t.Fatalf("model_unavailable must allow fallback: %+v", e)
	}
}

func TestClassify_InvariantsQuotaLimitedAutoRetryable(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "anthropic_quota_exceeded"})
	if e.Code != CodeQuotaLimited {
		t.Fatalf("got %s", e.Code)
	}
	if !e.AutoRetryable || !e.FallbackAllowed {
		t.Fatalf("quota_limited invariants violated: %+v", e)
	}
}

func TestClassify_OpenCodeUsageLimitIsHighConfidenceQuota(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "opencode-acp",
		Stderr:     "AI_APICallError: 5-hour usage limit reached. Resets in 4hr 19min.",
	})
	if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh {
		t.Fatalf("classification = %+v, want high-confidence quota_limited", e)
	}
}

func TestClassify_OpenCodePeriodUsageLimitsAreHighConfidenceQuota(t *testing.T) {
	resetInjection()
	for _, stderr := range []string{
		"AI_APICallError: Weekly usage limit reached. Resets in 3 days. To continue using this model now, enable usage from your available balance",
		"AI_APICallError: Daily usage limit reached. Resets in 4hr 19min.",
		"AI_APICallError: monthly usage limit reached.",
	} {
		e := Classify(Input{
			Phase:      PhaseStreaming,
			ProviderID: "opencode-acp",
			Stderr:     stderr,
		})
		if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh {
			t.Fatalf("classification of %q = %+v, want high-confidence quota_limited", stderr, e)
		}
	}
}

func TestClassify_OpenCodeCreditLimitReachedIsHighConfidenceQuota(t *testing.T) {
	resetInjection()
	cases := []struct {
		name   string
		stderr string
	}{
		{"observed DevPass message", "AI_APICallError: Dev Plan credit limit reached. Upgrade your plan or wait for renewal on 10/08/2026 Or enable pay-as-you-go overflow in your DevPass dashboard to keep going past your allowance"},
		{"plural credits exhausted", "AI_APICallError: out of credits"},
		{"singular credit exhausted", "AI_APICallError: out of credit"},
		{"insufficient plural credits", "AI_APICallError: insufficient credits for this request"},
		{"insufficient singular credit", "AI_APICallError: insufficient credit"},
		{"insufficient balance", "AI_APICallError: insufficient balance"},
		{"case insensitive", "AI_APICallError: CREDIT LIMIT REACHED"},
		{"variable whitespace", "AI_APICallError: out\t of  credits"},
		{"payment with exhausted allowance", "AI_APICallError: payment required: credit limit reached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{
				Phase:      PhaseStreaming,
				ProviderID: "opencode-acp",
				Stderr:     tc.stderr,
			})
			if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh || e.Class != ClassHard {
				t.Fatalf("classification = %+v, want high-confidence hard quota_limited", e)
			}
			if e.ClassifierRule != "opencode.stderr.credit.v1" {
				t.Fatalf("classifier rule = %s, want opencode.stderr.credit.v1", e.ClassifierRule)
			}
			if !e.FallbackAllowed || !e.AutoRetryable || e.UserAction {
				t.Fatalf("credit exhaustion recovery flags violated: %+v", e)
			}
		})
	}
}

func TestClassify_OpenCodeCreditRuleRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	cases := []struct {
		name     string
		provider string
		stderr   string
	}{
		{"bare credit", "opencode-acp", "credit"},
		{"bare credits", "opencode-acp", "credits"},
		{"purchase suggestion", "opencode-acp", "Visit the dashboard to purchase more credits"},
		{"allowance remains", "opencode-acp", "Your credit limit is approaching"},
		{"leading word boundary", "opencode-acp", "discredit limit reached"},
		{"trailing word boundary", "opencode-acp", "out of creditsuffix"},
		{"credit word suffix", "opencode-acp", "insufficient creditworthiness"},
		{"other provider", "codex-acp", "credit limit reached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{Phase: PhaseStreaming, ProviderID: tc.provider, Stderr: tc.stderr})
			if e.Code != CodeAgentRuntime || e.Confidence != ConfLow || e.FallbackAllowed {
				t.Fatalf("unrelated credit text must retain ambiguous post-start recovery: %+v", e)
			}
		})
	}
}

func TestClassify_OpenCodePaymentRequiredNeedsUserAction(t *testing.T) {
	resetInjection()
	cases := []struct {
		name       string
		stderr     string
		httpStatus int
		wantRule   string
	}{
		{"plain payment error", "AI_APICallError: payment required to continue", 0, "opencode.stderr.subscription.v1"},
		{"case and whitespace", "AI_APICallError: PAYMENT\tREQUIRED", 0, "opencode.stderr.subscription.v1"},
		{"unrelated credit mention", "payment required: update your credit card", 0, "opencode.stderr.subscription.v1"},
		{"HTTP status precedes credit text", "AI_APICallError: credit limit reached", http.StatusPaymentRequired, "http.402"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{
				Phase:      PhaseStreaming,
				ProviderID: "opencode-acp",
				Stderr:     tc.stderr,
				HTTPStatus: tc.httpStatus,
			})
			if e.Code != CodeSubscriptionRequired || e.Confidence != ConfHigh || e.ClassifierRule != tc.wantRule {
				t.Fatalf("payment classification = %+v, want high-confidence subscription_required via %s", e, tc.wantRule)
			}
			if !e.UserAction || e.AutoRetryable || !e.FallbackAllowed {
				t.Fatalf("payment recovery flags violated: %+v", e)
			}
		})
	}
}

func TestHasProviderRules(t *testing.T) {
	if !HasProviderRules("opencode-acp") {
		t.Fatal("opencode-acp should have provider rules")
	}
	if HasProviderRules("opencode-go") {
		t.Fatal("model-provider ID opencode-go must not resolve to provider rules")
	}
}

func TestClassify_InjectionShortCircuit(t *testing.T) {
	resetInjection()
	t.Setenv("KANDEV_PROVIDER_FAILURES", "claude-acp:quota_limited,codex-acp:auth_required")
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "completely unrelated text"})
	if e.Code != CodeQuotaLimited {
		t.Fatalf("injection ignored: got %s", e.Code)
	}
	if e.ClassifierRule != "inject.env" {
		t.Fatalf("expected inject.env rule, got %s", e.ClassifierRule)
	}
}

func TestClassify_SanitizesRawExcerpt(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "Bearer abcdefghijklmnopqrstuvwxyz1234567890"})
	if e.RawExcerpt == "" {
		t.Fatal("expected raw excerpt set")
	}
	if containsSubstring(e.RawExcerpt, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("raw excerpt not sanitized: %q", e.RawExcerpt)
	}
}

func TestClassify_SanitizesOpenCodeWorkspaceURL(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "opencode-acp",
		Stderr:     "stream error https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go",
	})
	for _, secret := range []string{"/workspace/", "wrk_01KQM7K5CYT715264YKKFB17ZY"} {
		if containsSubstring(e.RawExcerpt, secret) {
			t.Fatalf("raw excerpt contains %q: %q", secret, e.RawExcerpt)
		}
	}
	if !containsSubstring(e.RawExcerpt, "https://opencode.ai") {
		t.Fatalf("raw excerpt removed safe provider host: %q", e.RawExcerpt)
	}
}

func TestError_String(t *testing.T) {
	e := &Error{Code: CodeAuthRequired, ClassifierRule: "claude.stderr.auth.v1"}
	if got := e.Error(); got != "auth_required: claude.stderr.auth.v1" {
		t.Fatalf("got %q", got)
	}
}

// Codex sends the usage-limit notice as a plain agent message with a
// typographic apostrophe, and the terminal ACP error is only "Internal error".
// Classifying the notice text itself must yield a fallback-eligible quota error
// carrying the provider's retry time so dynamic routing can advance.
func TestClassify_CodexUsageLimitPlainText(t *testing.T) {
	resetInjection()
	cases := []struct {
		name string
		text string
	}{
		{
			"curly apostrophe",
			"You\u2019ve hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 27th, 2026 3:09 AM.",
		},
		{
			"straight apostrophe",
			"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 27th, 2026 3:09 AM.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{
				Phase:      PhaseStreaming,
				ProviderID: "codex-acp",
				Stderr:     tc.text,
			})
			if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh {
				t.Fatalf("classification = %s/%s, want quota_limited/high (rule=%s)",
					e.Code, e.Confidence, e.ClassifierRule)
			}
			if !e.FallbackAllowed {
				t.Fatalf("quota failure must allow fallback: %+v", e)
			}
			if e.ResetHint != nil {
				t.Fatalf("ResetHint = %v, want no hint for an unzoned provider time", e.ResetHint)
			}
		})
	}
}

func TestClassify_CodexUsageLimitExplicitTimezone(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "codex-acp",
		Stderr:     "You've hit your usage limit. try again at Sep 27th, 2026 3:09 AM UTC.",
	})
	want := time.Date(2026, time.September, 27, 3, 9, 0, 0, time.UTC)
	if e.ResetHint == nil || !e.ResetHint.Equal(want) {
		t.Fatalf("ResetHint = %v, want %v", e.ResetHint, want)
	}
}

// The structured reset hint from the adapter always wins over text parsing.
func TestClassify_ResetHintStructuredWins(t *testing.T) {
	resetInjection()
	hint := time.Date(2030, time.January, 2, 4, 5, 0, 0, time.UTC)
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "codex-acp",
		Stderr:     "You\u2019ve hit your usage limit. try again at Sep 27th, 2026 3:09 AM.",
		ResetHint:  &hint,
	})
	if e.ResetHint == nil || !e.ResetHint.Equal(hint) {
		t.Fatalf("ResetHint = %v, want the structured %v", e.ResetHint, hint)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
