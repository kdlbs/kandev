package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
)

// configStubAgent wraps MockAgent and overrides Runtime() with a fixed
// RuntimeConfig that mimics ACP agents (image+tag, {workspace} placeholder).
type configStubAgent struct {
	*agents.MockAgent
	rt *agents.RuntimeConfig
}

func (a *configStubAgent) Runtime() *agents.RuntimeConfig { return a.rt }

func newCMTest(t *testing.T) *ContainerManager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return &ContainerManager{
		logger:         log,
		networkName:    "kandev",
		commandBuilder: NewCommandBuilder(),
	}
}

func newConfigStubAgent() *configStubAgent {
	return &configStubAgent{
		MockAgent: agents.NewMockAgent(),
		rt: &agents.RuntimeConfig{
			Image:      "kandev/multi-agent",
			Tag:        "latest",
			Cmd:        agents.Cmd("/bin/true").Build(),
			WorkingDir: "{workspace}",
			Mounts:     []agents.MountTemplate{{Source: "{workspace}", Target: "/workspace"}},
			ResourceLimits: agents.ResourceLimits{
				MemoryMB: 256,
				CPUCores: 0.5,
			},
		},
	}
}

func TestBuildContainerConfig_ExpandsWorkingDirPlaceholder(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		// WorkspacePath empty → clone-inside-container path; should default to /workspace.
		InstanceID: "0123456789abcdef",
		TaskID:     "task-1",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if got.WorkingDir != "/workspace" {
		t.Errorf("WorkingDir = %q, want /workspace (placeholder must be expanded)", got.WorkingDir)
	}
	if strings.Contains(got.WorkingDir, "{") {
		t.Errorf("WorkingDir still contains placeholder syntax: %q", got.WorkingDir)
	}
}

func TestBuildContainerConfig_ExpandsWorkingDirWithProvidedWorkspace(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig:   newConfigStubAgent(),
		WorkspacePath: "/host/path/to/repo", // pre-clone-inside-container mount mode
		InstanceID:    "0123456789abcdef",
		TaskID:        "task-1",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if got.WorkingDir != "/host/path/to/repo" {
		t.Errorf("WorkingDir = %q, want /host/path/to/repo", got.WorkingDir)
	}
}

func TestBuildContainerConfig_ImageDefaultsToRuntime(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if got.Image != "kandev/multi-agent:latest" {
		t.Errorf("Image = %q, want kandev/multi-agent:latest", got.Image)
	}
}

func TestBuildContainerConfig_ImageTagOverrideWins(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig:      newConfigStubAgent(),
		InstanceID:       "0123456789abcdef",
		TaskID:           "task-1",
		ImageTagOverride: "kandev/agent:custom",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if got.Image != "kandev/agent:custom" {
		t.Errorf("Image = %q, want kandev/agent:custom (profile override must win over rt.Image)", got.Image)
	}
}
