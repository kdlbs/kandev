package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// withSubscriptionClaudeHome points the host home at a temporary Claude
// subscription credential so the agent reports subscription billing without
// reading the real user profile. No usage request is made by these tests.
func withSubscriptionClaudeHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	creds := `{"claudeAiOauth":{"accessToken":"test-token","expiresAt":4102444800000}}`
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
}

func newClaudeUsageAdapterForTest(t *testing.T, profile *settingsmodels.AgentProfile) *usageProviderAdapter {
	t.Helper()
	registry := agentregistry.NewRegistry(newTestLogger())
	if err := registry.Register(agents.NewClaudeACP()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return newUsageProviderAdapter(&antigravityUsageSettings{profile: profile}, registry)
}

// With provider usage limits enabled, a profile whose credentials or home do
// not provably belong to the host account must not read the host account's
// usage: the reading would be attributed to the wrong allowance account.
func TestUsageAdapterGetUsageRefusesUnprovedHostBindingWithTierLimits(t *testing.T) {
	cases := map[string]*settingsmodels.AgentProfile{
		"credential override": {
			ID: "claude-api-key", AgentID: claudeACPAgentID,
			EnvVars: []settingsmodels.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", Value: "test-key"}},
		},
		"remote executor": {
			ID: "claude-remote", AgentID: claudeACPAgentID, ExecutorPreference: "docker",
		},
	}
	withSubscriptionClaudeHome(t)
	for name, profile := range cases {
		t.Run(name, func(t *testing.T) {
			adapter := newClaudeUsageAdapterForTest(t, profile)
			if ag, ok := adapter.agentRegistry.Get(claudeACPAgentID); !ok || ag.BillingType() != agentusage.BillingTypeSubscription {
				t.Fatal("test setup: the host Claude account must report subscription billing")
			}
			adapter.tierLimits = &providerTierLimitService{}

			usage, err := adapter.GetUsage(context.Background(), profile.ID)
			if err != nil {
				t.Fatalf("GetUsage: %v", err)
			}
			if usage != nil {
				t.Fatalf("GetUsage = %#v, want nil for an unproved host binding", usage)
			}
			if adapter.svc.IsRegistered(profile.ID) {
				t.Fatal("an unproved host binding registered the host usage client")
			}
		})
	}
}
