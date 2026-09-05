package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

func TestGitMetadataMountsMasksSiblingWorktrees(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	runContainerGit(t, "", "init", "-b", "main", repo)
	runContainerGit(t, repo, "config", "user.email", "test@example.com")
	runContainerGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, repo, "add", "file")
	runContainerGit(t, repo, "commit", "-m", "initial")
	checkout := filepath.Join(t.TempDir(), "checkout")
	runContainerGit(t, repo, "worktree", "add", "-b", "task", checkout)
	projection, err := worktree.ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	mounts, err := gitMetadataMounts([]*worktree.GitMetadataProjection{projection})
	if err != nil {
		t.Fatal(err)
	}
	assertGitMount(t, mounts, projection.CommonDir, true, false)
	assertGitMount(t, mounts, projection.WorktreesDir, false, true)
	for _, path := range projection.WritablePaths {
		assertGitMount(t, mounts, path, false, false)
	}
	for _, mount := range mounts {
		if mount.Source == repo || mount.Target == repo {
			t.Fatalf("source checkout must not be mounted: %#v", mount)
		}
		if mount.Target == projection.CommonDir && !mount.ReadOnly {
			t.Fatalf("common git root must not be writable: %#v", mount)
		}
	}
}

// TestGitMetadataMountsRegularRepositoryHasNoConflictingDuplicateMount covers
// a regular (non-worktree) repository, where GitDir == CommonDir. Since GitDir
// is already granted writable via WritablePaths, also adding CommonDir to the
// read-only set would produce two Docker mounts targeting the same path with
// contradictory ReadOnly flags.
func TestGitMetadataMountsRegularRepositoryHasNoConflictingDuplicateMount(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	runContainerGit(t, "", "init", "-b", "main", repo)
	runContainerGit(t, repo, "config", "user.email", "test@example.com")
	runContainerGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, repo, "add", "file")
	runContainerGit(t, repo, "commit", "-m", "initial")
	projection, err := worktree.ResolveGitMetadata(repo)
	if err != nil {
		t.Fatal(err)
	}
	if projection.GitDir != projection.CommonDir {
		t.Fatalf("test fixture must be a regular repository: GitDir=%q CommonDir=%q", projection.GitDir, projection.CommonDir)
	}
	mounts, err := gitMetadataMounts([]*worktree.GitMetadataProjection{projection})
	if err != nil {
		t.Fatal(err)
	}
	targets := make(map[string]int, len(mounts))
	for _, mount := range mounts {
		targets[mount.Target]++
	}
	for target, count := range targets {
		if count > 1 {
			t.Fatalf("mount target %q used %d times, want exactly 1: %#v", target, count, mounts)
		}
	}
	assertGitMount(t, mounts, projection.GitDir, false, false)
}

func runContainerGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func assertGitMount(t *testing.T, mounts []docker.MountConfig, target string, readOnly, tmpfs bool) {
	t.Helper()
	for _, mount := range mounts {
		if mount.Target == target && mount.ReadOnly == readOnly && mount.Tmpfs == tmpfs {
			return
		}
	}
	t.Fatalf("missing mount target=%q readOnly=%t tmpfs=%t: %#v", target, readOnly, tmpfs, mounts)
}

// TestBuildContainerConfig_MutableCloneOmitsHostGitMetadataMounts guards the
// clone-in-container invariant: a checkout created fresh at /workspace inside
// the container has no host checkout to project, so even a caller that (by
// bug) still attaches host GitMetadataProjections under
// RequiresCloneGitMetadataPolicy must not have them bind-mounted into the
// container.
func TestBuildContainerConfig_MutableCloneOmitsHostGitMetadataMounts(t *testing.T) {
	cm := newCMTest(t)
	repo := filepath.Join(t.TempDir(), "repo")
	runContainerGit(t, "", "init", "-b", "main", repo)
	runContainerGit(t, repo, "config", "user.email", "test@example.com")
	runContainerGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runContainerGit(t, repo, "add", "file")
	runContainerGit(t, repo, "commit", "-m", "initial")
	projection, err := worktree.ResolveGitMetadata(repo)
	if err != nil {
		t.Fatal(err)
	}

	cfg := ContainerConfig{
		AgentConfig:                    newConfigStubAgent(),
		InstanceID:                     "0123456789abcdef",
		TaskID:                         "task-1",
		RequiresCloneGitMetadataPolicy: true,
		// A leaked host projection here must never surface as a mount.
		GitMetadataProjections: []*worktree.GitMetadataProjection{projection},
	}
	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	for _, mount := range got.Mounts {
		if mount.Source == repo || mount.Target == repo ||
			mount.Source == projection.CommonDir || mount.Target == projection.CommonDir ||
			mount.Source == projection.GitDir || mount.Target == projection.GitDir {
			t.Fatalf("mutable-clone container must not mount host Git metadata: %#v", mount)
		}
	}
}

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

func TestBuildContainerConfigPreflightsBrokerBeforePrepareClone(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
		Credentials: map[string]string{
			envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
			envKeyGitHubCredentialLease:     "lease",
		},
		PrepareScript: "git clone https://github.com/acme/widgets.git /workspace",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if len(got.Entrypoint) != 3 {
		t.Fatalf("entrypoint = %#v", got.Entrypoint)
	}
	script := got.Entrypoint[2]
	probeAt := strings.Index(script, "curl -sS --connect-timeout")
	cloneAt := strings.Index(script, `eval "$KANDEV_PREPARE_SCRIPT"`)
	if probeAt < 0 || cloneAt < 0 || probeAt >= cloneAt {
		t.Fatalf("broker probe must precede prepare/clone: %s", script)
	}
	if !strings.Contains(script, `exit "$probe_rc"`) {
		t.Fatalf("unreachable broker must stop bootstrap before clone: %s", script)
	}
}

func TestBuildContainerConfigBoundsPrepareScriptBeforeAgentctl(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig:   newConfigStubAgent(),
		InstanceID:    "0123456789abcdef",
		TaskID:        "task-1",
		PrepareScript: "sleep 1",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	if len(got.Entrypoint) != 3 {
		t.Fatalf("entrypoint = %#v", got.Entrypoint)
	}

	script := got.Entrypoint[2]
	want := fmt.Sprintf(
		"timeout -s TERM -k 1s %s sh -c",
		"600s",
	)
	if !strings.Contains(script, want) {
		t.Fatalf("prepare timeout = %q, want bootstrap to contain %q", script, want)
	}
	if strings.Index(script, want) >= strings.Index(script, "exec /usr/local/bin/agentctl") {
		t.Fatalf("prepare timeout must run before agentctl: %s", script)
	}
}

func TestBuildContainerConfigMutableCloneStartsAgentctlAfterPrepareFailure(t *testing.T) {
	cm := newCMTest(t)
	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig:                    newConfigStubAgent(),
		InstanceID:                     "0123456789abcdef",
		TaskID:                         "task-1",
		PrepareScript:                  "exit 42",
		RequiresCloneGitMetadataPolicy: true,
	})
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	for _, value := range got.Env {
		if strings.HasPrefix(value, "KANDEV_REQUIRE_GIT_METADATA_ATTESTATION=") {
			t.Fatalf("container environment must not make prepare failure bypass lifecycle attestation: %q", value)
		}
	}
	bootstrap := got.Entrypoint[2]
	if strings.Contains(bootstrap, `exit "$prep_rc"`) {
		t.Fatalf("prepare failure exits before trusted agentctl can attest the checkout: %s", bootstrap)
	}
	if !strings.Contains(bootstrap, "exec /usr/local/bin/agentctl") {
		t.Fatalf("bootstrap does not start trusted agentctl after prepare failure: %s", bootstrap)
	}
}

func TestCloneGitMetadataPrepareScriptAttestsCanonicalWorkspace(t *testing.T) {
	script := cloneGitMetadataPrepareScript("git clone https://example.test/repo /workspace")
	if !strings.Contains(script, "git clone https://example.test/repo /workspace") {
		t.Fatalf("prepare script lost clone command: %s", script)
	}
	if !strings.Contains(script, "git -C \"$workspace\" rev-parse --absolute-git-dir") {
		t.Fatalf("prepare script does not attest canonical Git directory: %s", script)
	}
}

func TestBuildContainerCreateInstanceRequestUsesContainerWorkspaceSourceRoot(t *testing.T) {
	request := buildContainerCreateInstanceRequest(ContainerConfig{InstanceID: "instance-1"}, "codex", false, false, false, false, nil)
	if !equalStrings(request.WorkspaceSourceRoots, []string{dockerWorkspacePath}) {
		t.Fatalf("WorkspaceSourceRoots = %v, want the container workspace only", request.WorkspaceSourceRoots)
	}
}

func TestBuildContainerConfigPublishesManagedGitCredentialHelperBeforeAgentctlStartup(t *testing.T) {
	cm := newCMTest(t)
	cfg := ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
		Credentials: map[string]string{
			envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
			envKeyGitHubCredentialLease:     "lease",
		},
		PrepareScript: "git clone https://github.com/acme/widgets.git /workspace",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}
	want := "KANDEV_GITHUB_CREDENTIAL_HELPER_PATH=/usr/local/bin/agentctl"
	if !containsExactString(got.Env, want) {
		t.Fatalf("container env missing pre-start credential helper %q: %#v", want, got.Env)
	}
}

func containsExactString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func TestBuildContainerConfig_LabelsE2EDockerScope(t *testing.T) {
	t.Setenv("KANDEV_E2E_DOCKER_SCOPE", "e2e-test-scope")
	cm := newCMTest(t)
	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
	})
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	assertLabel(t, got.Labels, "kandev.e2e.run", "e2e-test-scope")
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

// TestDockerAgentctlPortBindings is a direct test for the helper that
// generates the published-port set for every kandev-managed Docker agent
// container. A regression here (wrong port range, missing agentctl port,
// non-loopback host IP) would silently break container reconnect, since
// `resolveDockerEndpoint` falls back to the container IP when the published
// port lookup fails.
func TestDockerAgentctlPortBindings(t *testing.T) {
	bindings := dockerAgentctlPortBindings()

	wantTotal := 1 + (dockerAgentctlInstancePortMax - dockerAgentctlInstancePortBase + 1)
	if len(bindings) != wantTotal {
		t.Fatalf("got %d bindings, want %d (control + instance range)", len(bindings), wantTotal)
	}

	// Control port must be present.
	assertHasPortBinding(t, bindings, AgentCtlPort)

	// Every port in the instance range must be present and bound to loopback
	// with a kernel-assigned host port.
	have := make(map[int]docker.PortBindingConfig, len(bindings))
	for _, b := range bindings {
		have[b.ContainerPort] = b
	}
	for port := dockerAgentctlInstancePortBase; port <= dockerAgentctlInstancePortMax; port++ {
		b, ok := have[port]
		if !ok {
			t.Fatalf("missing instance port %d in published bindings", port)
		}
		if b.HostIP != "127.0.0.1" {
			t.Errorf("port %d host_ip = %q, want 127.0.0.1", port, b.HostIP)
		}
		if b.HostPort != "0" {
			t.Errorf("port %d host_port = %q, want kernel-assigned (\"0\")", port, b.HostPort)
		}
	}
}

// TestBuildContainerConfig_SessionDirIsKandevManagedForEveryAgent locks in
// the agent-agnostic guarantee that bind sources for SessionDirTemplate
// resolve to <kandev-home>/agent-sessions/<instance>/<dotdir> and never to
// the user's host home — the codex bug was a leak of host state into the
// container, and any agent with a SessionDirTemplate is at the same risk.
func TestBuildContainerConfig_SessionDirIsKandevManagedForEveryAgent(t *testing.T) {
	allAgents := []struct {
		name string
		ag   agents.Agent
	}{
		{"codex-acp", agents.NewCodexACP()},
		{"claude-acp", agents.NewClaudeACP()},
		{"opencode-acp", agents.NewOpenCodeACP()},
		{"devin-acp", agents.NewDevinACP()},
		{"copilot-acp", agents.NewCopilotACP()},
		{"amp-acp", agents.NewAmpACP()},
		{"gemini", agents.NewGemini()},
		{"auggie", agents.NewAuggie()},
		{"grok-acp", agents.NewGrokACP()},
	}
	const kandevHome = "/tmp/kandev-test-home"
	const instanceID = "0123456789abcdef"
	expectedRoot := filepath.Join(kandevHome, "agent-sessions", instanceID)

	for _, tc := range allAgents {
		t.Run(tc.name, func(t *testing.T) {
			rt := tc.ag.Runtime()
			if rt == nil {
				t.Skipf("%s has no Runtime", tc.name)
			}
			// expandMounts only adds the session-dir bind when BOTH fields
			// are set; agents that omit one rely on the in-container
			// SetupScript for auth and never bind-mount the host home in the
			// first place. Skip those — the test guards the resolution shape
			// only for agents that DO add the bind mount.
			if rt.SessionConfig.SessionDirTemplate == "" || rt.SessionConfig.SessionDirTarget == "" {
				t.Skipf("%s has no full SessionDirTemplate+SessionDirTarget pair (no bind mount today)", tc.name)
			}

			cm := newCMTest(t)
			cm.kandevHomeDir = kandevHome
			cfg := ContainerConfig{
				AgentConfig: tc.ag,
				InstanceID:  instanceID,
				TaskID:      "task-1",
			}

			got, err := cm.buildContainerConfig(cfg)
			if err != nil {
				t.Fatalf("buildContainerConfig: %v", err)
			}

			target := rt.SessionConfig.SessionDirTarget
			var found *docker.MountConfig
			for i := range got.Mounts {
				if got.Mounts[i].Target == target {
					found = &got.Mounts[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected mount for SessionDirTarget %q, got %+v", target, got.Mounts)
			}
			if !strings.HasPrefix(found.Source, expectedRoot) {
				t.Fatalf("session-dir mount source %q not under %q (host home leaked into container?)",
					found.Source, expectedRoot)
			}
			if strings.Contains(found.Source, "{home}") {
				t.Fatalf("session-dir mount source %q still references {home} placeholder", found.Source)
			}
		})
	}
}

func TestBuildContainerConfig_MountsDevinCredentialSessionDir(t *testing.T) {
	cm := newCMTest(t)
	cm.kandevHomeDir = "/tmp/kandev-test-home"
	instanceID := "devin-instance"

	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig: agents.NewDevinACP(),
		InstanceID:  instanceID,
		TaskID:      "task-1",
	})
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	wantSource := filepath.Join(cm.kandevHomeDir, "agent-sessions", instanceID, ".local/share/devin")
	wantTarget := "/root/.local/share/devin"
	for _, mount := range got.Mounts {
		if mount.Source == wantSource && mount.Target == wantTarget {
			return
		}
	}
	t.Fatalf("expected Devin credential mount %s -> %s, got %+v", wantSource, wantTarget, got.Mounts)
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
