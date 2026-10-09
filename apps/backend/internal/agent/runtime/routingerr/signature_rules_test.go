package routingerr

import "testing"

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
