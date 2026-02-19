package registry

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
)

// testAgent is a minimal implementation of agents.Agent for testing.
type testAgent struct {
	id           string
	name         string
	description  string
	enabled      bool
	displayOrder int
	runtime      *agents.RuntimeConfig
}

func (a *testAgent) ID() string                     { return a.id }
func (a *testAgent) Name() string                   { return a.name }
func (a *testAgent) DisplayName() string            { return a.name }
func (a *testAgent) Description() string            { return a.description }
func (a *testAgent) Enabled() bool                  { return a.enabled }
func (a *testAgent) DisplayOrder() int              { return a.displayOrder }
func (a *testAgent) Logo(agents.LogoVariant) []byte { return nil }
func (a *testAgent) IsInstalled(context.Context) (*agents.DiscoveryResult, error) {
	return nil, agents.ErrNotSupported
}
func (a *testAgent) DefaultModel() string { return "" }
func (a *testAgent) ListModels(context.Context) (*agents.ModelList, error) {
	return nil, agents.ErrNotSupported
}
func (a *testAgent) BuildCommand(agents.CommandOptions) agents.Command { return agents.Command{} }
func (a *testAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *testAgent) Runtime() *agents.RuntimeConfig { return a.runtime }

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func validAgentConfig(id, name string) *testAgent {
	return &testAgent{
		id:          id,
		name:        name,
		description: "Test agent",
		enabled:     true,
		runtime: &agents.RuntimeConfig{
			Image:      "test/image",
			Tag:        "latest",
			WorkingDir: "/workspace",
			ResourceLimits: agents.ResourceLimits{
				MemoryMB: 1024,
				CPUCores: 1.0,
				Timeout:  time.Hour,
			},
		},
	}
}

func TestNewRegistry(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	if reg == nil {
		t.Fatal("expected non-nil registry")
	} else if len(reg.agents) != 0 {
		t.Errorf("expected empty agents map, got %d", len(reg.agents))
	}
}

func TestRegistry_Register(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	ag := validAgentConfig("test-agent", "Test Agent")

	// Test successful registration
	err := reg.Register(ag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test duplicate registration
	err = reg.Register(ag)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_RegisterValidation(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	tests := []struct {
		name   string
		agent  agents.Agent
		errMsg string
	}{
		{
			name:   "empty ID",
			agent:  &testAgent{id: "", name: "test"},
			errMsg: "agent type ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reg.Register(tt.agent)
			if err == nil {
				t.Error("expected error")
			} else if err.Error() != tt.errMsg {
				t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestRegistry_Get(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	ag := validAgentConfig("test-agent", "Test Agent")
	_ = reg.Register(ag)

	// Test successful get
	got, ok := reg.Get("test-agent")
	if !ok {
		t.Fatal("expected agent to be found")
	}
	if got.ID() != ag.ID() {
		t.Errorf("expected ID %q, got %q", ag.ID(), got.ID())
	}

	// Test not found
	_, ok = reg.Get("non-existent")
	if ok {
		t.Error("expected agent to not be found")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	ag := validAgentConfig("test-agent", "Test Agent")
	_ = reg.Register(ag)

	// Test successful unregister
	err := reg.Unregister("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone
	if reg.Exists("test-agent") {
		t.Error("agent type should not exist after unregister")
	}

	// Test unregister non-existent
	err = reg.Unregister("non-existent")
	if err == nil {
		t.Error("expected error for non-existent agent type")
	}
}

func TestRegistry_List(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	// Empty list
	list := reg.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	// Add agents
	_ = reg.Register(validAgentConfig("agent-1", "Agent 1"))
	_ = reg.Register(validAgentConfig("agent-2", "Agent 2"))

	list = reg.List()
	if len(list) != 2 {
		t.Errorf("expected 2 agents, got %d", len(list))
	}
}

func TestRegistry_ListEnabled(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	enabled := validAgentConfig("enabled-agent", "Enabled Agent")
	enabled.enabled = true
	_ = reg.Register(enabled)

	disabled := validAgentConfig("disabled-agent", "Disabled Agent")
	disabled.enabled = false
	_ = reg.Register(disabled)

	enabledList := reg.ListEnabled()
	if len(enabledList) != 1 {
		t.Errorf("expected 1 enabled agent, got %d", len(enabledList))
	}
	if enabledList[0].ID() != "enabled-agent" {
		t.Errorf("expected enabled-agent, got %s", enabledList[0].ID())
	}
}

func TestRegistry_Exists(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	ag := validAgentConfig("test-agent", "Test Agent")
	_ = reg.Register(ag)

	if !reg.Exists("test-agent") {
		t.Error("expected agent type to exist")
	}
	if reg.Exists("non-existent") {
		t.Error("expected agent type to not exist")
	}
}

func TestRegistry_LoadDefaults(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	reg.LoadDefaults()

	// Verify at least some default agents are loaded
	list := reg.List()
	if len(list) == 0 {
		t.Skip("no default agents configured")
	}

	// Check a known default agent exists
	if !reg.Exists("auggie") {
		t.Error("expected default agent 'auggie' to be loaded")
	}
}

func TestRegistry_List_OrderedByDisplayOrder(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	// Register agents in reverse display order
	a3 := validAgentConfig("agent-c", "Agent C")
	a3.displayOrder = 30
	a2 := validAgentConfig("agent-b", "Agent B")
	a2.displayOrder = 20
	a1 := validAgentConfig("agent-a", "Agent A")
	a1.displayOrder = 10

	_ = reg.Register(a3)
	_ = reg.Register(a1)
	_ = reg.Register(a2)

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(list))
	}
	if list[0].ID() != "agent-a" || list[1].ID() != "agent-b" || list[2].ID() != "agent-c" {
		t.Errorf("expected agents ordered by DisplayOrder [agent-a, agent-b, agent-c], got [%s, %s, %s]",
			list[0].ID(), list[1].ID(), list[2].ID())
	}
}

func TestRegistry_ListEnabled_OrderedByDisplayOrder(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	a3 := validAgentConfig("agent-c", "Agent C")
	a3.displayOrder = 30
	a2 := validAgentConfig("agent-b", "Agent B")
	a2.displayOrder = 20
	a2.enabled = false // disabled — should be excluded
	a1 := validAgentConfig("agent-a", "Agent A")
	a1.displayOrder = 10

	_ = reg.Register(a3)
	_ = reg.Register(a1)
	_ = reg.Register(a2)

	list := reg.ListEnabled()
	if len(list) != 2 {
		t.Fatalf("expected 2 enabled agents, got %d", len(list))
	}
	if list[0].ID() != "agent-a" || list[1].ID() != "agent-c" {
		t.Errorf("expected enabled agents ordered [agent-a, agent-c], got [%s, %s]",
			list[0].ID(), list[1].ID())
	}
}

func TestAgentTypeConfig_ToAPIType(t *testing.T) {
	ag := validAgentConfig("test-agent", "Test Agent")
	ag.description = "Test description"
	ag.runtime.Env = map[string]string{"KEY": "value"}

	apiType := ToAPIType(ag)

	if apiType.ID != ag.ID() {
		t.Errorf("expected ID %q, got %q", ag.ID(), apiType.ID)
	}
	if apiType.Name != ag.Name() {
		t.Errorf("expected Name %q, got %q", ag.Name(), apiType.Name)
	}
	if apiType.Description != ag.Description() {
		t.Errorf("expected Description %q, got %q", ag.Description(), apiType.Description)
	}
	if apiType.DockerImage != ag.runtime.Image {
		t.Errorf("expected DockerImage %q, got %q", ag.runtime.Image, apiType.DockerImage)
	}
	if apiType.Enabled != ag.Enabled() {
		t.Errorf("expected Enabled %v, got %v", ag.Enabled(), apiType.Enabled)
	}
}
