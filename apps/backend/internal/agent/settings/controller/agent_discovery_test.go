package controller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

type managedTestAgent struct {
	testAgent
	spec agents.ManagedNPMRuntimeSpec
}

func (a *managedTestAgent) ManagedNPMRuntime() agents.ManagedNPMRuntimeSpec {
	return a.spec
}

func TestAvailableAgentIncludesManagedRuntimeMetadata(t *testing.T) {
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec:      agents.ManagedNPMRuntimeSpec{Package: "@example/managed-acp"},
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	ctrl.SetRuntimeUpdater(&fakeRuntimeUpdater{
		current:      hostutility.AgentCapabilities{AgentVersion: "1.2.3"},
		currentFound: true,
	})

	item := ctrl.buildAvailableAgentDTO(context.Background(), ag, discovery.Availability{
		Name:      ag.ID(),
		Available: true,
	}, time.Now())

	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal available agent: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal available agent: %v", err)
	}
	runtimeUpdate, ok := payload["runtime_update"].(map[string]any)
	if !ok {
		t.Fatal("runtime_update missing, want managed runtime metadata")
	}
	if runtimeUpdate["package"] != "@example/managed-acp" {
		t.Fatalf("package = %#v, want @example/managed-acp", runtimeUpdate["package"])
	}
	if runtimeUpdate["supported"] != true {
		t.Fatalf("supported = %#v, want true", runtimeUpdate["supported"])
	}
	if runtimeUpdate["current_version"] != "1.2.3" {
		t.Fatalf("current_version = %#v, want 1.2.3", runtimeUpdate["current_version"])
	}
}

func TestUnavailableManagedAgentOmitsRuntimeMetadata(t *testing.T) {
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec:      agents.ManagedNPMRuntimeSpec{Package: "@example/managed-acp"},
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	item := ctrl.buildAvailableAgentDTO(
		context.Background(),
		ag,
		discovery.Availability{Name: ag.ID(), Available: false},
		time.Now(),
	)
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal available agent: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal available agent: %v", err)
	}
	if _, exists := payload["runtime_update"]; exists {
		t.Fatal("runtime_update present for unavailable agent")
	}
}

func TestAvailableAgentIgnoresMismatchedManagedRuntimeSelection(t *testing.T) {
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec: agents.ManagedNPMRuntimeSpec{
			Package:        "@example/managed-acp",
			DefaultVersion: "1.2.3",
		},
	}
	selectionStore := newRecoverySelectionStore()
	selectionStore.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@other/managed-acp",
		Version: "9.9.9",
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	ctrl.SetManagedRuntimeSelectionStore(selectionStore)

	item := ctrl.buildAvailableAgentDTO(
		context.Background(),
		ag,
		discovery.Availability{Name: ag.ID(), Available: true},
		time.Now(),
	)
	if item.RuntimeUpdate == nil {
		t.Fatal("runtime_update missing")
	}
	if item.RuntimeUpdate.ActiveVersion != "" {
		t.Fatalf("active_version = %q, want empty for mismatched selection", item.RuntimeUpdate.ActiveVersion)
	}
	if item.RuntimeUpdate.EffectiveVersion != "1.2.3" {
		t.Fatalf("effective_version = %q, want default 1.2.3", item.RuntimeUpdate.EffectiveVersion)
	}
}

func TestAvailableACPInferenceAgentIsDynamicBeforeProbeSnapshot(t *testing.T) {
	ag := agents.NewMockAgent()
	ag.SetEnabled(true)
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})

	item := ctrl.buildAvailableAgentDTO(
		context.Background(),
		ag,
		discovery.Availability{Name: ag.ID(), Available: true},
		time.Now(),
	)

	if !item.ModelConfig.SupportsDynamicModels {
		t.Fatal("supports_dynamic_models = false, want true before the probe snapshot is cached")
	}
	if item.ModelConfig.Status != "not_configured" {
		t.Fatalf("status = %q, want not_configured without a host utility", item.ModelConfig.Status)
	}
}

func TestConfigOptionDTOsPreserveDescriptions(t *testing.T) {
	dtos := configOptionDTOs([]hostutility.ConfigOption{{
		Type:         "select",
		ID:           "reasoning_effort",
		Name:         "Reasoning effort",
		Description:  "Controls reasoning depth.",
		CurrentValue: "high",
		Options: []hostutility.ConfigOptionChoice{{
			Value:       "high",
			Name:        "High",
			Description: "More thorough reasoning.",
		}},
	}})

	raw, err := json.Marshal(dtos)
	if err != nil {
		t.Fatalf("marshal config option DTOs: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal config option DTOs: %v", err)
	}
	if got := payload[0]["description"]; got != "Controls reasoning depth." {
		t.Errorf("option description = %#v, want provider description", got)
	}
	values := payload[0]["options"].([]any)
	if got := values[0].(map[string]any)["description"]; got != "More thorough reasoning." {
		t.Errorf("value description = %#v, want provider description", got)
	}
}

// TestResolveDefaultModel_MockAgentSeedsAdvertisedModel pins the mock agent's
// seeded default profile model to one the mock agent actually advertises
// (mock-fast). The no-silent-model-fallback policy fails session start
// explicitly when the profile model is absent from the advertised list, so a
// sentinel like "mock-default" would break every default-profile session in
// the E2E environment.
func TestResolveDefaultModel_MockAgentSeedsAdvertisedModel(t *testing.T) {
	model, isPassthroughOnly, err := resolveDefaultModel(agents.NewMockAgent(), "")
	if err != nil {
		t.Fatalf("resolveDefaultModel: %v", err)
	}
	if isPassthroughOnly {
		t.Fatal("mock agent must not be seeded as passthrough-only")
	}
	if model != "mock-fast" {
		t.Fatalf("mock default profile model = %q, want %q (an advertised model)", model, "mock-fast")
	}
}
