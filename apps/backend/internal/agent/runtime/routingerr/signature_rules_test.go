package routingerr

import (
	"net/http"
	"testing"
)

// @covers AC-AGENTS-CODEX-RATE-SIGNATURE-001.1 AC-AGENTS-CODEX-RATE-SIGNATURE-001.2 AC-AGENTS-CODEX-RATE-SIGNATURE-001.3
func TestCodexRetryExhausted429Classification(t *testing.T) {
	resetInjection()
	const signature = "exceeded retry limit, last status: 429 Too Many Requests"
	for _, text := range []string{
		signature,
		"Internal error: " + signature,
		"API Error: " + signature,
	} {
		got := Classify(Input{Phase: PhasePromptSend, ProviderID: "codex-acp", Stderr: text})
		if got.Code != CodeRateLimited || got.Confidence != ConfHigh {
			t.Errorf("Codex message %q classified as %s/%s, want high-confidence rate limit", text, got.Code, got.Confidence)
		}
	}

	for _, tc := range []struct {
		name      string
		provider  string
		text      string
		status    int
		wantOther Code
	}{
		{name: "mid-sentence quote", provider: "codex-acp", text: "Request ended after " + signature},
		{name: "other status", provider: "codex-acp", text: "exceeded retry limit, last status: 503 Service Unavailable"},
		{name: "incomplete signature", provider: "codex-acp", text: "exceeded retry limit, last status: 429"},
		{name: "unrelated provider", provider: "opencode-acp", text: signature},
		{name: "structured authentication precedence", provider: "codex-acp", text: signature, status: http.StatusUnauthorized, wantOther: CodeAuthRequired},
		{name: "quota signature precedence", provider: "codex-acp", text: "insufficient_quota\n" + signature, wantOther: CodeQuotaLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: tc.provider, Stderr: tc.text, HTTPStatus: tc.status})
			if tc.wantOther != "" {
				if got.Code != tc.wantOther {
					t.Fatalf("classification = %s (%s), want %s", got.Code, got.ClassifierRule, tc.wantOther)
				}
				return
			}
			if got.Code == CodeRateLimited {
				t.Fatalf("message %q from %s classified as rate limit by %s", tc.text, tc.provider, got.ClassifierRule)
			}
		})
	}
}

// agentProseAboutLimits is agent output that talks about limits it read
// elsewhere. None of it is the reporting agent's own provider failing.
var agentProseAboutLimits = []string{
	"I started a new session for the MCP server work; it hit a rate limit right after it started.",
	"Both alerts are old turns that were already handled (08:39Z and 08:52Z, rate limit).",
	"Now I have a clear picture. Other tasks are stuck on technical issues (rate limits, model not found).",
	"The child's sessions are all persistently rate-limited free models.",
	"The newest session (05:48) died on rate limit, so I moved the card to hold.",
	"The plan document hit its size quota earlier today; I trimmed old sections.",
	"The plan document ran out of quota earlier today.",
	"This step does not need a subscription; it only reads the board.",
	"I checked the billing page: the credit balance is fine for this month.",
	"The child reports that its subscription has expired.",
	"The OpenCode task quoted `payment required`.",
	"The Copilot task quoted `subscription required`.",
	"The child reports that it is not entitled to Copilot.",
	"The child reports that its other provider has insufficient credits.",
	"The Claude child says, `You've hit your session limit`.",
	"The OpenCode child reports that a daily usage limit was reached.",
	"The OpenCode child reports insufficient credits.",
}

// agentProseQuotingOtherProviders is agent prose that quotes another
// provider's exact error signatures mid-sentence. A prompt error can carry
// the agent's final text, and this text does not name its own provider failure.
var agentProseQuotingOtherProviders = []string{
	"Blocked: six child tasks failed when resuming their sessions (`AI_APICallError: Not Found`, `Rate limit exceeded`) and stopped without a completion signal.",
	"Child session log: Agent encountered an error: AI_APICallError: Rate limit exceeded. Please try again later",
	"The OpenCode child reported `AI_APICallError: Go usage limit exceeded` and another child reported `credit limit reached`.",
	"The child sent too many requests while it retried.",
	"The child reported rate_limit_exceeded and RESOURCE_EXHAUSTED.",
	"The child reported insufficient_quota for its account.",
	`The child reported {"code":-32603,"message":"Internal error","data":{"codexErrorInfo":"usageLimitExceeded"}}.`,
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.4
func TestClassify_AgentProseAboutLimitsIsNotAProviderLimit(t *testing.T) {
	resetInjection()
	for _, provider := range []string{"claude-acp", "codex-acp", "opencode-acp", "copilot-acp", "amp-acp"} {
		for _, text := range agentProseAboutLimits {
			for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
				got := Classify(Input{Phase: phase, ProviderID: provider, Stderr: text})
				switch got.Code {
				case CodeRateLimited, CodeQuotaLimited, CodeSubscriptionRequired:
					t.Fatalf("%s prose %q classified as %s by %s", provider, text, got.Code, got.ClassifierRule)
				}
			}
		}
		for _, text := range agentProseQuotingOtherProviders {
			for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
				got := Classify(Input{Phase: phase, ProviderID: provider, Stderr: text})
				switch got.Code {
				case CodeRateLimited, CodeQuotaLimited, CodeSubscriptionRequired:
					t.Fatalf("%s quoted prose %q classified as %s by %s", provider, text, got.Code, got.ClassifierRule)
				}
			}
		}
	}
}

func TestClassify_ProviderLimitSignaturesStillClassify(t *testing.T) {
	resetInjection()
	cases := []struct {
		provider string
		text     string
		want     Code
	}{
		{"claude-acp", `Internal error: API Error: 429 {"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed the rate limit for your organization"}}`, CodeRateLimited},
		{"claude-acp", "API Error: Request rejected (429) · This request would exceed your account's rate limit.", CodeRateLimited},
		{"claude-acp", "HTTP 429 Too Many Requests", CodeRateLimited},
		{"claude-acp", "You've hit your rate limit", CodeRateLimited},
		{"claude-acp", "rate limit exceeded", CodeRateLimited},
		{"claude-acp", "Internal error: Rate limit exceeded. Please retry shortly.", CodeRateLimited},
		{"claude-acp", "request failed\nRate limit reached for this organization", CodeRateLimited},
		{"claude-acp", "Your credit balance is too low to access the Anthropic API.", CodeQuotaLimited},
		{"claude-acp", "anthropic_quota_exceeded", CodeQuotaLimited},
		{"claude-acp", "Claude Code requires an active Claude Pro or Max subscription.", CodeSubscriptionRequired},
		{"claude-acp", "Your subscription has expired.", CodeSubscriptionRequired},
		{"opencode-acp", "AI_APICallError: Rate limit exceeded. Please try again later", CodeRateLimited},
		{"opencode-acp", "AI_APICallError: Rate limit exceeded: free-models-per-min", CodeRateLimited},
		{"opencode-acp", "rate_limit_exceeded", CodeRateLimited},
		{"opencode-acp", "AI_APICallError: Too Many Requests", CodeRateLimited},
		{"opencode-acp", "AI_APICallError: You exceeded your current quota, please check your plan and billing details.", CodeQuotaLimited},
		{"opencode-acp", "AI_APICallError: Quota exceeded for quota metric 'Generate Content API requests per minute'", CodeQuotaLimited},
		{"opencode-acp", "AI_APICallError: RESOURCE_EXHAUSTED", CodeQuotaLimited},
		{"copilot-acp", "Error: rate limit exceeded", CodeRateLimited},
		{"amp-acp", "rate limited: rate limit reached for this account", CodeRateLimited},
		{"amp-acp", "insufficient_quota", CodeQuotaLimited},
	}
	for _, tc := range cases {
		got := Classify(Input{Phase: PhaseStreaming, ProviderID: tc.provider, Stderr: tc.text})
		if got.Code != tc.want {
			t.Fatalf("%s %q → %s (%s), want %s", tc.provider, tc.text, got.Code, got.ClassifierRule, tc.want)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.6
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.8
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.9
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.10
func TestClassify_ProviderCatalogueSignatures(t *testing.T) {
	resetInjection()
	cases := []struct {
		name, provider, stderr, stdout string
		wantCode                       Code
		wantRule                       string
	}{
		{"JSON field first", "opencode-acp", "AI_APICallError: {\"error_type\":\"provider_unavailable\",\"message\":\"temporary\"}", "provider diagnostic tail", CodeProviderUnavailable, "provider.structured_error_type.v1"},
		{"JSON field after message", "opencode-acp", "AI_APICallError: {\"message\":\"temporary\",\"error_type\":\"provider_unavailable\"}", "provider diagnostic tail", CodeProviderUnavailable, "provider.structured_error_type.v1"},
		{"Agent status and JSON field first", "opencode-acp", "Agent encountered an error: AI_APICallError: {\"error_type\":\"provider_unavailable\",\"message\":\"temporary\"}", "provider diagnostic tail", CodeProviderUnavailable, "provider.structured_error_type.v1"},
		{"Agent status and JSON field after message", "opencode-acp", "Agent encountered an error: AI_APICallError: {\"message\":\"temporary\",\"error_type\":\"provider_unavailable\"}", "provider diagnostic tail", CodeProviderUnavailable, "provider.structured_error_type.v1"},
		{"bare free models daily", "opencode-acp", "Rate limit exceeded: free-models-per-day-high-balance", "provider diagnostic tail", CodeRateLimited, "opencode.rate.free_models_per_day.v1"},
		{"AI prefix free models daily", "opencode-acp", "AI_APICallError: Rate limit exceeded: free-models-per-day-high-balance", "provider diagnostic tail", CodeRateLimited, "opencode.rate.free_models_per_day.v1"},
		{"bare model daily", "opencode-acp", "Rate limit exceeded: limit_rpd/gpt-test", "provider diagnostic tail", CodeRateLimited, "opencode.rate.model_per_day.v1"},
		{"AI prefix model daily", "opencode-acp", "AI_APICallError: Rate limit exceeded: limit_rpd/gpt-test", "provider diagnostic tail", CodeRateLimited, "opencode.rate.model_per_day.v1"},
		{"bare retry later", "opencode-acp", "Rate limit exceeded. Please try again later", "provider diagnostic tail", CodeRateLimited, "opencode.rate.try_later.v1"},
		{"AI prefix retry later", "opencode-acp", "AI_APICallError: Rate limit exceeded. Please try again later", "provider diagnostic tail", CodeRateLimited, "opencode.rate.try_later.v1"},
		{"provider label retry later", "opencode-acp", "Error from provider (zen): Rate limit exceeded. Please try again later", "provider diagnostic tail", CodeRateLimited, "opencode.rate.try_later.v1"},
		{"AI prefix and provider label retry later", "opencode-acp", "AI_APICallError: Error from provider (zen): Rate limit exceeded. Please try again later", "provider diagnostic tail", CodeRateLimited, "opencode.rate.try_later.v1"},
		{"unlabelled free usage", "opencode-acp", "free usage limit reached", "provider diagnostic tail", CodeQuotaLimited, "opencode.stderr.free_usage_unlabelled.v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{
				Phase: PhasePromptSend, ProviderID: tc.provider,
				Stderr: tc.stderr, Stdout: tc.stdout,
			})
			if got.Code != tc.wantCode || got.ClassifierRule != tc.wantRule {
				t.Fatalf("got %s/%s, want %s/%s", got.Code, got.ClassifierRule, tc.wantCode, tc.wantRule)
			}
			if want := Sanitize(tc.stderr + "\n" + tc.stdout); got.RawExcerpt != want {
				t.Fatalf("RawExcerpt = %q, want %q", got.RawExcerpt, want)
			}
			if tc.wantRule == "opencode.rate.try_later.v1" && got.ResetHint != nil {
				t.Fatalf("retry-later wording invented a reset: %v", got.ResetHint)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.4
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.5
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.8
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.10
func TestClassify_ProviderCatalogueBoundariesAndPriority(t *testing.T) {
	resetInjection()
	cases := []struct {
		name, provider string
		status         int
		stderr, stdout string
		wantCode       Code
		wantRule       string
	}{
		{"bare JSON is not a diagnostic", "opencode-acp", 0, "{\"error_type\":\"provider_unavailable\"}", "provider diagnostic tail", CodeAgentRuntime, "phase.poststart.unknown"},
		{"mid-prose JSON label is not a diagnostic", "opencode-acp", 0, "Assistant prose quotes Error: {\"error_type\":\"provider_unavailable\"}", "provider diagnostic tail", CodeAgentRuntime, "phase.poststart.unknown"},
		{"wrong structured error type", "opencode-acp", 0, "AI_APICallError: {\"error_type\":\"task_failure\"}", "provider diagnostic tail", CodeAgentRuntime, "phase.poststart.unknown"},
		{"mid-prose retry later is not a diagnostic", "opencode-acp", 0, "The log said: Rate limit exceeded. Please try again later", "provider diagnostic tail", CodeAgentRuntime, "phase.poststart.unknown"},
		{"mid-prose free usage is not a diagnostic", "opencode-acp", 0, "The child reported free usage limit reached", "provider diagnostic tail", CodeAgentRuntime, "phase.poststart.unknown"},
		{"daily rule stays provider scoped", "copilot-acp", 0, "AI_APICallError: Rate limit exceeded: free-models-per-day-high-balance", "provider diagnostic tail", CodeRateLimited, "copilot.stderr.rate.v1"},
		{"HTTP payment precedes daily", "opencode-acp", http.StatusPaymentRequired, "AI_APICallError: Rate limit exceeded: free-models-per-day-high-balance", "provider diagnostic tail", CodeSubscriptionRequired, "http.402"},
		{"HTTP rate precedes structured field", "opencode-acp", http.StatusTooManyRequests, "AI_APICallError: {\"message\":\"temporary\",\"error_type\":\"provider_unavailable\"}", "provider diagnostic tail", CodeRateLimited, "http.429"},
		{"HTTP unavailable precedes structured field", "opencode-acp", http.StatusServiceUnavailable, "AI_APICallError: {\"error_type\":\"provider_unavailable\",\"message\":\"temporary\"}", "provider diagnostic tail", CodeProviderUnavailable, "http.503"},
		{"credit precedes daily", "opencode-acp", 0, "AI_APICallError: Rate limit exceeded: free-models-per-day-high-balance\nError: Out of credits", "provider diagnostic tail", CodeQuotaLimited, "opencode.stderr.credit.v1"},
		{"payment precedes daily", "opencode-acp", 0, "AI_APICallError: Rate limit exceeded: free-models-per-day-high-balance\nError: payment required to continue", "provider diagnostic tail", CodeSubscriptionRequired, "opencode.stderr.subscription.v1"},
		{"quota precedes daily", "opencode-acp", 0, "AI_APICallError: Rate limit exceeded: limit_rpd/gpt-test\nAI_APICallError: Quota exceeded for quota metric GenerateContent", "provider diagnostic tail", CodeQuotaLimited, "opencode.stderr.quota.v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{
				Phase: PhasePromptSend, ProviderID: tc.provider,
				HTTPStatus: tc.status, Stderr: tc.stderr, Stdout: tc.stdout,
			})
			if got.Code != tc.wantCode || got.ClassifierRule != tc.wantRule {
				t.Fatalf("got %s/%s, want %s/%s", got.Code, got.ClassifierRule, tc.wantCode, tc.wantRule)
			}
			if want := Sanitize(tc.stderr + "\n" + tc.stdout); got.RawExcerpt != want {
				t.Fatalf("RawExcerpt = %q, want %q", got.RawExcerpt, want)
			}
		})
	}
}
