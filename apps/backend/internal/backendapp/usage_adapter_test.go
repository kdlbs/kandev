package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// antigravityUsageSettings serves one Antigravity profile. Embedding the
// repository interface keeps the fake limited to the read GetUsage performs.
type antigravityUsageSettings struct {
	settingsstore.Repository
	profile *settingsmodels.AgentProfile
}

func (s *antigravityUsageSettings) GetAgentProfile(
	context.Context,
	string,
) (*settingsmodels.AgentProfile, error) {
	return s.profile, nil
}

// TestAntigravityDoesNotRegisterQuotaClient pins the accounting boundary for
// REQ-AGENTS-ANTIGRAVITY-ACP-014: the personal subscription path declares
// subscription billing metadata but registers no Google allowance client in the
// Office usage adapter.
func TestAntigravityDoesNotRegisterQuotaClient(t *testing.T) {
	registry := agentregistry.NewRegistry(newTestLogger())
	if err := registry.Register(agents.NewAntigravityACP()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	agent, ok := registry.Get(agents.AntigravityACPAgentID)
	if !ok {
		t.Fatal("Antigravity agent is not registered")
	}
	if agent.BillingType() != agentusage.BillingTypeSubscription {
		t.Fatalf("BillingType() = %q, want subscription", agent.BillingType())
	}

	const profileID = "antigravity-personal-profile"
	adapter := newUsageProviderAdapter(
		&antigravityUsageSettings{profile: &settingsmodels.AgentProfile{
			ID:      profileID,
			AgentID: agents.AntigravityACPAgentID,
		}},
		registry,
	)

	usage, err := adapter.GetUsage(context.Background(), profileID)
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if usage != nil {
		t.Fatalf("GetUsage = %#v, want nil without an allowance client", usage)
	}
	if adapter.svc.IsRegistered(profileID) {
		t.Fatal("Antigravity profile registered a usage client; no Google allowance client may be polled")
	}

	limited, _, err := adapter.svc.IsPotentiallyRateLimited(context.Background(), profileID)
	if err != nil {
		t.Fatalf("IsPotentiallyRateLimited: %v", err)
	}
	if limited {
		t.Fatal("Antigravity must not report allowance-driven rate limiting without a client")
	}
}
