package routingerr

import "regexp"

type rule struct {
	id         string
	pattern    *regexp.Regexp
	code       Code
	confidence Confidence
}

var providerRules = map[string][]rule{
	"claude-acp": {
		mustRule("claude.stderr.quota.v1", `(?i)anthropic_quota_exceeded|credit balance|insufficient credits`, CodeQuotaLimited, ConfHigh),
		mustRule("claude.stderr.rate.v1", `(?i)rate.?limit`, CodeRateLimited, ConfHigh),
		mustRule("claude.stderr.auth.v1", `(?i)not authenticated|please log in|run `+"`"+`claude`+"`"+` to authenticate`, CodeAuthRequired, ConfHigh),
		mustRule("claude.stderr.subscription.v1", `(?i)subscription`, CodeSubscriptionRequired, ConfMedium),
		mustRule("claude.stderr.model.v1", `(?i)model.*not found|unknown model`, CodeModelUnavailable, ConfHigh),
		mustRule("claude.stderr.notinstalled.v1", `(?i)command not found|no such file`, CodeProviderNotConfigured, ConfMedium),
	},
	"codex-acp": {
		mustRule("codex.stderr.quota.v1", `(?i)insufficient_quota|quota_exceeded`, CodeQuotaLimited, ConfHigh),
		mustRule("codex.stderr.rate.v1", `(?i)rate_limit_exceeded|too many requests`, CodeRateLimited, ConfHigh),
		mustRule("codex.stderr.auth.v1", `(?i)invalid api key|incorrect api key|missing api key`, CodeMissingCredentials, ConfHigh),
		mustRule("codex.stderr.model.v1", `(?i)model_not_found`, CodeModelUnavailable, ConfHigh),
	},
	"opencode-acp": {
		mustRule("opencode.stderr.quota.v1", `(?i)quota`, CodeQuotaLimited, ConfMedium),
		mustRule("opencode.stderr.rate.v1", `(?i)rate.?limit`, CodeRateLimited, ConfHigh),
		mustRule("opencode.stderr.auth.v1", `(?i)unauthorized|invalid token`, CodeAuthRequired, ConfHigh),
	},
	"copilot-acp": {
		mustRule("copilot.stderr.subscription.v1", `(?i)not entitled|subscription required|copilot is disabled`, CodeSubscriptionRequired, ConfHigh),
		mustRule("copilot.stderr.auth.v1", `(?i)please.*sign in|gh auth login`, CodeAuthRequired, ConfHigh),
		mustRule("copilot.stderr.rate.v1", `(?i)rate.?limit`, CodeRateLimited, ConfHigh),
	},
	"amp-acp": {
		mustRule("amp.stderr.auth.v1", `(?i)unauthorized|invalid token`, CodeAuthRequired, ConfHigh),
		mustRule("amp.stderr.rate.v1", `(?i)rate.?limit`, CodeRateLimited, ConfHigh),
		mustRule("amp.stderr.quota.v1", `(?i)quota`, CodeQuotaLimited, ConfMedium),
	},
}

func mustRule(id, pat string, code Code, conf Confidence) rule {
	return rule{id: id, pattern: regexp.MustCompile(pat), code: code, confidence: conf}
}

func matchProviderRules(providerID, text string) (*Error, bool) {
	rules, ok := providerRules[providerID]
	if !ok || text == "" {
		return nil, false
	}
	for _, r := range rules {
		if r.pattern.MatchString(text) {
			return &Error{
				Code:           r.code,
				Confidence:     r.confidence,
				ClassifierRule: r.id,
			}, true
		}
	}
	return nil, false
}
