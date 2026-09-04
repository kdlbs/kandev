package backendapp

import (
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestTeamClaudeStatusURL(t *testing.T) {
	cases := []struct {
		name   string
		base   string
		vendor string
		want   string
		ok     bool
	}{
		{"localhost", "http://localhost:3456", "teamclaude", "http://localhost:3456/teamclaude/status", true},
		{"ipv6 loopback", "http://[::1]:3456/v1", "teamclaude", "http://[::1]:3456/teamclaude/status", true},
		{"selection required", "http://localhost:3456", "", "", false},
		{"remote denied", "https://proxy.example.test", "teamclaude", "", false},
		{"credentials denied", "http://user:pass@localhost:3456", "teamclaude", "", false},
		{"not a URL", "localhost:3456", "teamclaude", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := teamClaudeStatusURL(&settingsmodels.AgentProfile{
				AgentID: "claude-acp",
				EnvVars: []settingsmodels.ProfileEnvVar{
					{Key: teamClaudeBaseURLEnv, Value: tc.base},
					{Key: usageProxyVendorEnv, Value: tc.vendor},
				},
			})
			if got != tc.want || ok != tc.ok {
				t.Fatalf("teamClaudeStatusURL = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
