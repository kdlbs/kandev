package agents

import (
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agent/usage"
)

// defaultBillingType returns BillingTypeAPIKey. Used by agents that do not
// override BillingType().
func defaultBillingType() usage.BillingType {
	return usage.BillingTypeAPIKey
}

// claudeBillingType detects whether the Claude agent is using OAuth
// subscription credentials. Computed on every call (the read is a small
// local file) so logging in after the backend started is picked up without
// a restart.
func claudeBillingType() usage.BillingType {
	home, err := os.UserHomeDir()
	if err != nil {
		return usage.BillingTypeAPIKey
	}
	client := usage.NewClaudeUsageClientWithPath(filepath.Join(home, ".claude", ".credentials.json"))
	if client.HasSubscriptionCredentials() {
		return usage.BillingTypeSubscription
	}
	return usage.BillingTypeAPIKey
}

// codexBillingType detects whether the Codex agent is using subscription
// credentials. It reads ~/.codex/auth.json — that path matches the
// SourceFiles / Runtime mounts in codex_acp.go, where the real Codex CLI
// persists OAuth tokens. An auth.json holding only OPENAI_API_KEY (no
// ChatGPT OAuth tokens) is API-key billing. Computed on every call so
// `codex login` after backend start flips billing without a restart.
func codexBillingType() usage.BillingType {
	home, err := os.UserHomeDir()
	if err != nil {
		return usage.BillingTypeAPIKey
	}
	client := usage.NewCodexUsageClientWithPath(filepath.Join(home, ".codex", "auth.json"))
	if client.HasSubscriptionCredentials() {
		return usage.BillingTypeSubscription
	}
	return usage.BillingTypeAPIKey
}
