package controller

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/common/logger"
)

type availabilityTestAgent struct {
	id string

	discoveryResult *agents.DiscoveryResult
	modelList       *agents.ModelList

	mu                  sync.Mutex
	isInstalledCalls    int
	listModelsCallCount int
}

func (a *availabilityTestAgent) ID() string                             { return a.id }
func (a *availabilityTestAgent) Name() string                           { return a.id }
func (a *availabilityTestAgent) DisplayName() string                    { return a.id }
func (a *availabilityTestAgent) Description() string                    { return "" }
func (a *availabilityTestAgent) Enabled() bool                          { return true }
func (a *availabilityTestAgent) DisplayOrder() int                      { return 0 }
func (a *availabilityTestAgent) Logo(variant agents.LogoVariant) []byte { return nil }
func (a *availabilityTestAgent) DefaultModel() string                   { return "default-model" }
func (a *availabilityTestAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	return agents.NewCommand()
}
func (a *availabilityTestAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *availabilityTestAgent) Runtime() *agents.RuntimeConfig { return nil }
func (a *availabilityTestAgent) RemoteAuth() *agents.RemoteAuth { return nil }

func (a *availabilityTestAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	a.mu.Lock()
	a.isInstalledCalls++
	res := *a.discoveryResult
	a.mu.Unlock()
	return &res, nil
}

func (a *availabilityTestAgent) ListModels(ctx context.Context) (*agents.ModelList, error) {
	a.mu.Lock()
	a.listModelsCallCount++
	res := *a.modelList
	a.mu.Unlock()
	return &res, nil
}

func (a *availabilityTestAgent) resetInstalledCalls() {
	a.mu.Lock()
	a.isInstalledCalls = 0
	a.mu.Unlock()
}

func (a *availabilityTestAgent) installedCalls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isInstalledCalls
}

func TestListAvailableAgents_UsesDiscoveryCapabilitiesWithoutExtraIsInstalledCall(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	ag := &availabilityTestAgent{
		id: "codex",
		discoveryResult: &agents.DiscoveryResult{
			Available: true,
			Capabilities: agents.DiscoveryCapabilities{
				SupportsSessionResume: true,
				SupportsShell:         true,
				SupportsWorkspaceOnly: false,
			},
		},
		modelList: &agents.ModelList{SupportsDynamic: false},
	}

	agentRegistry := registry.NewRegistry(log)
	if err := agentRegistry.Register(ag); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), agentRegistry, log)
	if err != nil {
		t.Fatalf("load discovery registry: %v", err)
	}

	ag.resetInstalledCalls()
	ctrl := &Controller{
		discovery:     discoveryRegistry,
		agentRegistry: agentRegistry,
		modelCache:    modelfetcher.NewCache(),
		logger:        log,
	}

	resp, err := ctrl.ListAvailableAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAvailableAgents: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(resp.Agents))
	}

	if got := ag.installedCalls(); got != 1 {
		t.Fatalf("IsInstalled calls = %d, want 1", got)
	}

	caps := resp.Agents[0].Capabilities
	expected := dto.AgentCapabilitiesDTO{
		SupportsSessionResume: true,
		SupportsShell:         true,
		SupportsWorkspaceOnly: false,
	}
	if caps != expected {
		t.Fatalf("capabilities = %+v, want %+v", caps, expected)
	}
}
