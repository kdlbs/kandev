package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/kandev/kandev/internal/agent/usage"
)

// defaultBillingType returns BillingTypeAPIKey. Used by agents that do not
// override BillingType().
func defaultBillingType() usage.BillingType {
	return usage.BillingTypeAPIKey
}

// claudeBillingType detects whether the Claude agent is using OAuth
// subscription credentials. It reads ~/.claude/.credentials.json once and
// caches the result for the process lifetime.
var claudeBillingType = sync.OnceValue(func() usage.BillingType {
	home, err := os.UserHomeDir()
	if err != nil {
		return usage.BillingTypeAPIKey
	}
	path := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return usage.BillingTypeAPIKey
	}
	var creds struct {
		ClaudeAiOauth *struct{} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return usage.BillingTypeAPIKey
	}
	if creds.ClaudeAiOauth != nil {
		return usage.BillingTypeSubscription
	}
	return usage.BillingTypeAPIKey
})

// codexBillingType detects whether the Codex agent is using subscription
// credentials. It checks for ~/.codex/auth.json once per process — that
// path matches the SourceFiles / Runtime mounts in codex_acp.go, where
// the real Codex CLI persists OAuth tokens. The earlier ~/.config/codex
// path was an XDG-style guess and never matched a real install, so
// subscription billing was undetectable.
var codexBillingType = sync.OnceValue(func() usage.BillingType {
	home, err := os.UserHomeDir()
	if err != nil {
		return usage.BillingTypeAPIKey
	}
	path := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(path); err == nil {
		return usage.BillingTypeSubscription
	}
	return usage.BillingTypeAPIKey
})
