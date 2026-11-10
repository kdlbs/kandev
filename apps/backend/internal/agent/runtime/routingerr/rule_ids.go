package routingerr

const (
	ruleClaudeSessionLimit = "claude.stderr.session_limit.v1"
	ruleClaudeWeeklyLimit  = "claude.stderr.weekly_limit.v1"
	ruleOpenCodeUsageLimit = "opencode.stderr.usage_limit.v1"
	ruleOpenCodeCredit     = "opencode.stderr.credit.v1"
	providerClaudeACP      = "claude-acp"
	providerOpenCodeACP    = "opencode-acp"
	providerAntigravityACP = "antigravity-acp"
	errKindRateLimitError  = "rate_limit_error"
)
