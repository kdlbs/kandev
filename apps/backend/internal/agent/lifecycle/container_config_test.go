package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
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

func TestBuildContainerConfig_WorkingDirIsAlwaysContainerPath(t *testing.T) {
	// Regression: WorkingDir is the container-side path, not the host path.
	// In host bind-mount mode, WorkspacePath holds the host path; the bind
	// mount target is the in-container /workspace, so WorkingDir must point
	// at the container target — otherwise Docker happily starts the
	// container in /host/path/to/repo (which doesn't exist inside) and the
	// agent runs in an unrelated directory.
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig:   newConfigStubAgent(),
		WorkspacePath: "/host/path/to/repo",
		InstanceID:    "0123456789abcdef",
		TaskID:        "task-1",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if got.WorkingDir != "/workspace" {
		t.Errorf("WorkingDir = %q, want /workspace (container-side path)", got.WorkingDir)
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

func TestBuildContainerConfig_LabelsExecutorProfileAndTaskEnvironment(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig:       newConfigStubAgent(),
		InstanceID:        "0123456789abcdef",
		TaskID:            "task-1",
		TaskTitle:         "Readable Task Title",
		SessionID:         "session-1",
		TaskEnvironmentID: "env-1",
		ExecutorProfileID: "profile-1",
		ImageTagOverride:  "kandev/agent:custom",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	assertLabel(t, got.Labels, "kandev.managed", boolStringTrue)
	assertLabel(t, got.Labels, "kandev.task_id", "task-1")
	assertLabel(t, got.Labels, "kandev.task_title", "Readable Task Title")
	assertLabel(t, got.Labels, "kandev.session_id", "session-1")
	assertLabel(t, got.Labels, "kandev.task_environment_id", "env-1")
	assertLabel(t, got.Labels, "kandev.executor_profile_id", "profile-1")
	assertLabel(t, got.Labels, "kandev.profile_id", "profile-1")
	assertLabel(t, got.Labels, "com.kandev.image", "kandev/agent:custom")
}

func TestBuildContainerConfig_PublishesAgentctlPorts(t *testing.T) {
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

	if len(got.PortBindings) == 0 {
		t.Fatal("expected agentctl ports to be published")
	}
	assertHasPortBinding(t, got.PortBindings, AgentCtlPort)
	assertHasPortBinding(t, got.PortBindings, dockerAgentctlInstancePortBase)
	assertHasPortBinding(t, got.PortBindings, dockerAgentctlInstancePortMax)
	assertEnvContains(t, got.Env, "AGENTCTL_INSTANCE_PORT_BASE=41001")
	assertEnvContains(t, got.Env, "AGENTCTL_INSTANCE_PORT_MAX=41100")
}

func TestBuildContainerConfig_MountsLocalClonePath(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig:    newConfigStubAgent(),
		InstanceID:     "0123456789abcdef",
		TaskID:         "task-1",
		LocalClonePath: "/tmp/e2e-docker-remote.git",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	assertHasMount(t, got.Mounts, "/tmp/e2e-docker-remote.git", "/tmp/e2e-docker-remote.git", true)
}

func assertLabel(t *testing.T, labels map[string]string, key, want string) {
	t.Helper()
	if labels[key] != want {
		t.Fatalf("label %s = %q, want %q in %#v", key, labels[key], want, labels)
	}
}

func assertHasMount(t *testing.T, mounts []docker.MountConfig, source, target string, readOnly bool) {
	t.Helper()
	for _, mount := range mounts {
		if mount.Source == source && mount.Target == target && mount.ReadOnly == readOnly {
			return
		}
	}
	t.Fatalf("missing mount source=%q target=%q readOnly=%v in %#v", source, target, readOnly, mounts)
}

func assertHasPortBinding(t *testing.T, bindings []docker.PortBindingConfig, port int) {
	t.Helper()
	for _, binding := range bindings {
		if binding.ContainerPort == port && binding.HostIP == "127.0.0.1" && binding.HostPort == "0" {
			return
		}
	}
	t.Fatalf("missing published port binding for %d/tcp: %#v", port, bindings)
}

func assertEnvContains(t *testing.T, env []string, want string) {
	t.Helper()
	for _, item := range env {
		if item == want {
			return
		}
	}
	t.Fatalf("missing env %q in %#v", want, env)
}
