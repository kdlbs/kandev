package routingerr

import (
	"net/http"
	"reflect"
	"strings"
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

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.1
func TestClassifyClaudeSessionLimit(t *testing.T) {
	resetInjection()
	for _, tc := range []struct {
		name  string
		phase Phase
		text  string
	}{
		{
			name:  "observed notice during streaming",
			phase: PhaseStreaming,
			text:  "Internal error: You've hit your session limit · resets 11:10am (Europe/Helsinki)",
		},
		{
			name:  "curly apostrophe during prompt send",
			phase: PhasePromptSend,
			text:  "You've hit your session limit",
		},
		{
			name:  "you have with case and whitespace differences",
			phase: PhaseStreaming,
			text:  "YOU   HAVE hit\tyour session   limit. Resets 11am (Europe/Helsinki)",
		},
		{
			name:  "curly apostrophe and case differences",
			phase: PhasePromptSend,
			text:  "YOU’VE HIT YOUR SESSION LIMIT",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{Phase: tc.phase, ProviderID: "claude-acp", Stderr: tc.text})
			if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh || e.Class != ClassHard {
				t.Fatalf("classification = %+v, want high-confidence hard quota_limited", e)
			}
			if e.ClassifierRule != "claude.stderr.session_limit.v1" {
				t.Fatalf("classifier rule = %q, want claude.stderr.session_limit.v1", e.ClassifierRule)
			}
			if !e.FallbackAllowed || !e.AutoRetryable || e.UserAction {
				t.Fatalf("session-limit recovery flags violated: %+v", e)
			}
		})
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.7
func TestClassifyClaudeSessionLimitRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	for _, tc := range []struct {
		name     string
		provider string
		text     string
		want     Code
	}{
		{name: "different provider", provider: "codex-acp", text: "You've hit your session limit", want: CodeAgentRuntime},
		{name: "bare phrase", provider: "claude-acp", text: "session limit", want: CodeAgentRuntime},
		{name: "approaching limit", provider: "claude-acp", text: "You're approaching your session limit", want: CodeAgentRuntime},
		{name: "limit suffix", provider: "claude-acp", text: "You've hit your session limitless", want: CodeAgentRuntime},
		{name: "rate limit remains transient", provider: "claude-acp", text: "You've hit your rate limit", want: CodeRateLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{Phase: PhaseStreaming, ProviderID: tc.provider, Stderr: tc.text})
			if e.Code != tc.want {
				t.Fatalf("classification = %+v, want %s", e, tc.want)
			}
			if tc.want == CodeRateLimited && e.Class != ClassTransient {
				t.Fatalf("rate-limit class = %s, want transient", e.Class)
			}
		})
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.5
func TestClassifyClaudeResetHintPrecedence(t *testing.T) {
	resetInjection()
	structured := time.Date(2030, time.January, 2, 4, 5, 0, 0, time.UTC)
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "claude-acp",
		Stderr:     "You've hit your session limit · resets 11:10am (Europe/Helsinki)",
		ResetHint:  &structured,
	})
	if e.Code != CodeQuotaLimited || e.ResetHint == nil || !e.ResetHint.Equal(structured) {
		t.Fatalf("classification = %+v, want quota with structured reset hint %v", e, structured)
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.4
func TestClassifyClaudeInvalidResetClockKeepsQuotaClassification(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "claude-acp",
		Stderr:     "You've hit your session limit · resets 13:70am (Europe/Helsinki)",
	})
	if e.Code != CodeQuotaLimited || e.Class != ClassHard || e.Confidence != ConfHigh || !e.FallbackAllowed {
		t.Fatalf("classification = %+v, want hard quota despite invalid clock", e)
	}
	if e.ResetHint != nil {
		t.Fatalf("ResetHint = %v, want no hint for invalid clock", e.ResetHint)
	}
}

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.5
func TestClassifyClaudeStructuredHTTPStatusPrecedence(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "claude-acp",
		HTTPStatus: http.StatusPaymentRequired,
		Stderr:     "You've hit your session limit · resets 11:10am (Europe/Helsinki)",
	})
	if e.Code != CodeSubscriptionRequired || e.ClassifierRule != "http.402" {
		t.Fatalf("classification = %+v, want structured HTTP 402 precedence", e)
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

func TestClassify_OpenCodeWeeklyResetClockDoesNotBecomeDailyHint(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "opencode-acp",
		Stderr:     "AI_APICallError: Weekly usage limit reached · resets 11am (America/New_York)",
	})
	if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh {
		t.Fatalf("classification = %+v, want high-confidence quota_limited", e)
	}
	if e.ResetHint != nil {
		t.Fatalf("ResetHint = %v, want no daily hint for a weekly reset notice", e.ResetHint)
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

func TestClassifyKeywordMixedSeverity(t *testing.T) {
	for _, text := range []string{
		"Error: account suspended, usage limit exceeded",
		"Error: forbidden access suspended, usage quota limit exceeded",
		"Error: model retired, usage limit exceeded",
	} {
		t.Run(text, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: "unknown-provider", Stderr: text,
				KeywordContext: KeywordContext{Enabled: true, Diagnostic: true}})
			if got.Confidence != ConfLow || got.Keyword != nil || got.FallbackAllowed || got.AutoRetryable {
				t.Fatalf("mixed unresolved severity enabled automatic recovery: %+v", got)
			}
		})
	}
}

func TestClassifyKeywordEnvelopeAndEvidence(t *testing.T) {
	for _, tc := range []struct {
		text, group string
	}{
		{"Error: CAPACITY, busy!", "capacity"},
		{"Error: credits balances", "limit"},
		{"Error: unauthorized invalid-api-key", "account"},
		{"Error: retired deprecated", "gone"},
		{"Error: invalid_tools schema", "request"},
		{"Error: " + strings.Repeat("busy ", 3), ""},
		{"Error: suspended", ""},
		{"Error: capacity busy quota renewal", ""},
		{"Error: not overloaded or busy", ""},
		{"Error: without capacity or busy", ""},
		{`Error: "capacity busy"`, ""},
		{"An example says capacity busy", ""},
		{"Error: capacity\nbusy", ""},
		{"Error: capacity busy " + strings.Repeat("x", 1024), ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, Stderr: tc.text,
				KeywordContext: KeywordContext{Enabled: true, Diagnostic: true}})
			if tc.group == "" {
				if got.Keyword != nil || got.Confidence != ConfLow || got.AutoRetryable || got.FallbackAllowed {
					t.Fatalf("unsafe evidence admitted: %+v", got)
				}
			} else if got.Keyword == nil || got.Keyword.Group != tc.group || got.Confidence != ConfHigh {
				t.Fatalf("keyword evidence = %+v, want group %s", got, tc.group)
			}
		})
	}
}

func TestClassifyKeywordDisabledAndKnownPriority(t *testing.T) {
	for _, in := range []Input{
		{Phase: PhasePromptSend, Stderr: "Error: capacity busy"},
		{Phase: PhasePromptSend, Stderr: "Selected model is at capacity. Please try a different model."},
		{Phase: PhasePromptSend, HTTPStatus: 403, Stderr: "Error: capacity busy"},
		{Phase: PhasePromptSend, Stderr: `Error from provider gateway: 403 Forbidden {"code":"user_blocked"} usage limit exceeded`},
	} {
		legacy := Classify(in)
		in.KeywordContext.Diagnostic = true
		if got := Classify(in); !reflect.DeepEqual(got, legacy) {
			t.Fatalf("disabled classifier changed legacy result: %+v versus %+v", got, legacy)
		}
		if legacy.Confidence == ConfLow {
			continue
		}
		in.KeywordContext.Enabled, in.KeywordContext.HTTPStatus = true, 429
		if got := Classify(in); !reflect.DeepEqual(got, legacy) {
			t.Fatalf("keyword fallback displaced existing rule: %+v versus %+v", got, legacy)
		}
	}
}

func TestClassifyKeywordStructuralStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		text   string
		code   Code
	}{
		{401, "", CodeAuthRequired}, {403, "", CodePermissionDeniedByUser},
		{404, "Error: model retired", CodeModelUnavailable},
		{408, "", CodeProviderUnavailable}, {504, "", CodeProviderUnavailable},
		{429, "", CodeRateLimited}, {500, "", CodeProviderUnavailable},
		{502, "", CodeProviderUnavailable}, {503, "", CodeProviderUnavailable},
		{529, "", CodeProviderOverloaded},
		{400, "Error: context tokens length", CodeAgentRuntime},
		{400, "Error: tools schema", CodeAgentRuntime},
	} {
		got := Classify(Input{Phase: PhasePromptSend, Stderr: tc.text,
			KeywordContext: KeywordContext{Enabled: true, Diagnostic: true, HTTPStatus: tc.status}})
		if got.Code != tc.code || got.Keyword == nil || got.Confidence != ConfHigh {
			t.Fatalf("status %d: %+v, want %s", tc.status, got, tc.code)
		}
	}
	got := Classify(Input{Phase: PhasePromptSend,
		KeywordContext: KeywordContext{Enabled: true, Diagnostic: true, HTTPStatus: 404}})
	if got.Keyword != nil || got.Confidence != ConfLow {
		t.Fatalf("unrelated 404 inferred model removal: %+v", got)
	}
}

func TestClassifyKeywordScopeTimingAndAction(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		text, scope, action string
		context             KeywordContext
		phase               Phase
		backoff             time.Duration
	}{
		{"Error: capacity busy", "path_model", "try_next", KeywordContext{}, PhaseProcessStart, 5 * time.Minute},
		{"Error: capacity busy", "model", "retry_same", KeywordContext{ModelID: "m", OtherPaths: 1}, PhaseStreaming, 5 * time.Minute},
		{"Error: capacity busy", "path_model", "try_next", KeywordContext{ConsecutiveFailures: 2, OtherModels: 1}, PhaseStreaming, 15 * time.Minute},
		{"Error: renewal balance", "account", "try_next", KeywordContext{AccountID: "known", Tier: "subscription", ConsecutiveFailures: 99}, PhasePromptSend, 30 * time.Minute},
		{"Error: renewal balance", "path_model", "try_next", KeywordContext{Tier: "subscription"}, PhasePromptSend, 5 * time.Minute},
		{"Error: forbidden suspended", "path_model", "stop", KeywordContext{}, PhasePromptSend, 0},
	} {
		c := tc.context
		c.Enabled, c.Diagnostic = true, true
		got := Classify(Input{Phase: tc.phase, Stderr: tc.text, OccurredAt: now, KeywordContext: c})
		if got.Keyword == nil || got.Keyword.Scope != tc.scope || got.Keyword.Action != tc.action || got.Keyword.Backoff != tc.backoff || HoldsUntilRelease(got.Code) {
			t.Fatalf("scope/timing/action violated: %+v / %+v", got, got.Keyword)
		}
	}
	reset := now.Add(2 * time.Minute)
	got := Classify(Input{Phase: PhasePromptSend, Stderr: "Error: renewal balance", OccurredAt: now, ResetHint: &reset,
		KeywordContext: KeywordContext{Enabled: true, Diagnostic: true, FreeDaily: true}})
	if got.ResetHint == nil || !got.ResetHint.Equal(reset) {
		t.Fatalf("explicit reset lost: %+v", got)
	}
	got = Classify(Input{Phase: PhasePromptSend, Stderr: "Error: renewal balance", OccurredAt: now,
		KeywordContext: KeywordContext{Enabled: true, Diagnostic: true, FreeDaily: true}})
	if got.ResetHint == nil || !got.ResetHint.Equal(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("explicit free-daily context lost UTC reset: %+v", got)
	}
}
