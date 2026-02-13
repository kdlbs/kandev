package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func validAgentConfig(id, name string) *AgentTypeConfig {
	return &AgentTypeConfig{
		ID:          id,
		Name:        name,
		Description: "Test agent",
		Image:       "test/image",
		Tag:         "latest",
		WorkingDir:  "/workspace",
		ResourceLimits: ResourceLimits{
			MemoryMB:       1024,
			CPUCores:       1.0,
			TimeoutSeconds: 3600,
		},
		Capabilities: []string{"test"},
		Enabled:      true,
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

	config := validAgentConfig("test-agent", "Test Agent")

	// Test successful registration
	err := reg.Register(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test duplicate registration
	err = reg.Register(config)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_RegisterValidation(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	tests := []struct {
		name   string
		config *AgentTypeConfig
		errMsg string
	}{
		{
			name:   "empty ID",
			config: &AgentTypeConfig{Name: "test", Image: "img", ResourceLimits: ResourceLimits{MemoryMB: 1, CPUCores: 1, TimeoutSeconds: 1}},
			errMsg: "agent type ID is required",
		},
		{
			name:   "empty name",
			config: &AgentTypeConfig{ID: "test", Image: "img", ResourceLimits: ResourceLimits{MemoryMB: 1, CPUCores: 1, TimeoutSeconds: 1}},
			errMsg: "agent type name is required",
		},
		{
			name:   "empty image and cmd",
			config: &AgentTypeConfig{ID: "test", Name: "test", ResourceLimits: ResourceLimits{MemoryMB: 1, CPUCores: 1, TimeoutSeconds: 1}},
			errMsg: "agent type requires either image (Docker) or cmd (standalone)",
		},
		{
			name:   "zero memory",
			config: &AgentTypeConfig{ID: "test", Name: "test", Image: "img", ResourceLimits: ResourceLimits{MemoryMB: 0, CPUCores: 1, TimeoutSeconds: 1}},
			errMsg: "memory limit must be positive",
		},
		{
			name:   "zero CPU",
			config: &AgentTypeConfig{ID: "test", Name: "test", Image: "img", ResourceLimits: ResourceLimits{MemoryMB: 1, CPUCores: 0, TimeoutSeconds: 1}},
			errMsg: "CPU cores must be positive",
		},
		{
			name:   "zero timeout",
			config: &AgentTypeConfig{ID: "test", Name: "test", Image: "img", ResourceLimits: ResourceLimits{MemoryMB: 1, CPUCores: 1, TimeoutSeconds: 0}},
			errMsg: "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reg.Register(tt.config)
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

	config := validAgentConfig("test-agent", "Test Agent")
	_ = reg.Register(config)

	// Test successful get
	got, ok := reg.Get("test-agent")
	if !ok {
		t.Fatal("expected agent to be found")
	}
	if got.ID != config.ID {
		t.Errorf("expected ID %q, got %q", config.ID, got.ID)
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

	config := validAgentConfig("test-agent", "Test Agent")
	_ = reg.Register(config)

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
	enabled.Enabled = true
	_ = reg.Register(enabled)

	disabled := validAgentConfig("disabled-agent", "Disabled Agent")
	disabled.Enabled = false
	_ = reg.Register(disabled)

	enabledList := reg.ListEnabled()
	if len(enabledList) != 1 {
		t.Errorf("expected 1 enabled agent, got %d", len(enabledList))
	}
	if enabledList[0].ID != "enabled-agent" {
		t.Errorf("expected enabled-agent, got %s", enabledList[0].ID)
	}
}

func TestRegistry_Exists(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	config := validAgentConfig("test-agent", "Test Agent")
	_ = reg.Register(config)

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

	// Check that default agents are loaded
	defaults := DefaultAgents()
	if len(defaults) == 0 {
		t.Skip("no default agents configured")
	}

	for _, def := range defaults {
		if !reg.Exists(def.ID) {
			t.Errorf("default agent %q not loaded", def.ID)
		}
	}
}

func TestRegistry_LoadFromFile(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	// Create temp file with valid config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agents.json")

	configs := []*AgentTypeConfig{
		validAgentConfig("file-agent-1", "File Agent 1"),
		validAgentConfig("file-agent-2", "File Agent 2"),
	}

	data, _ := json.Marshal(configs)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test loading from file
	err := reg.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reg.Exists("file-agent-1") || !reg.Exists("file-agent-2") {
		t.Error("agents from file not loaded")
	}
}

func TestRegistry_LoadFromFile_NotFound(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.LoadFromFile("/non/existent/path.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestRegistry_LoadFromFile_InvalidJSON(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(configPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err := reg.LoadFromFile(configPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestAgentTypeConfig_ToAPIType(t *testing.T) {
	config := validAgentConfig("test-agent", "Test Agent")
	config.Description = "Test description"
	config.Capabilities = []string{"cap1", "cap2"}
	config.Env = map[string]string{"KEY": "value"}

	apiType := config.ToAPIType()

	if apiType.ID != config.ID {
		t.Errorf("expected ID %q, got %q", config.ID, apiType.ID)
	}
	if apiType.Name != config.Name {
		t.Errorf("expected Name %q, got %q", config.Name, apiType.Name)
	}
	if apiType.Description != config.Description {
		t.Errorf("expected Description %q, got %q", config.Description, apiType.Description)
	}
	if apiType.DockerImage != config.Image {
		t.Errorf("expected DockerImage %q, got %q", config.Image, apiType.DockerImage)
	}
	if len(apiType.Capabilities) != len(config.Capabilities) {
		t.Errorf("expected %d capabilities, got %d", len(config.Capabilities), len(apiType.Capabilities))
	}
	if apiType.Enabled != config.Enabled {
		t.Errorf("expected Enabled %v, got %v", config.Enabled, apiType.Enabled)
	}
}

func TestValidateConfig(t *testing.T) {
	// Valid config with default tag (Docker-based agent)
	config := &AgentTypeConfig{
		ID:    "test",
		Name:  "test",
		Image: "test/image",
		ResourceLimits: ResourceLimits{
			MemoryMB:       1024,
			CPUCores:       1.0,
			TimeoutSeconds: 3600,
		},
	}

	err := ValidateConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tag should be set to default
	if config.Tag != "latest" {
		t.Errorf("expected default tag 'latest', got %q", config.Tag)
	}

	// Valid standalone agent with cmd but no image (like Claude Code)
	standaloneConfig := &AgentTypeConfig{
		ID:   "standalone-test",
		Name: "Standalone Test",
		Cmd:  []string{"npx", "-y", "some-cli"},
		ResourceLimits: ResourceLimits{
			MemoryMB:       1024,
			CPUCores:       1.0,
			TimeoutSeconds: 3600,
		},
	}

	err = ValidateConfig(standaloneConfig)
	if err != nil {
		t.Fatalf("standalone agent should be valid: %v", err)
	}

	// Tag should NOT be set when image is empty
	if standaloneConfig.Tag != "" {
		t.Errorf("expected no tag for standalone agent, got %q", standaloneConfig.Tag)
	}
}

