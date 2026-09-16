package lifecycle

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
)

// mountPairs renders a mount set as comparable "source->target[:ro]" strings.
func mountPairs(mounts []docker.MountConfig) []string {
	out := make([]string, 0, len(mounts))
	for _, m := range mounts {
		entry := m.Source + "->" + m.Target
		if m.ReadOnly {
			entry += ":ro"
		}
		out = append(out, entry)
	}
	sort.Strings(out)
	return out
}

// newCMTestWithHostFiles builds a container manager with deterministic host
// file sources, so the mount set is a pure function of the configuration.
func newCMTestWithHostFiles(t *testing.T, agentctlPath, mockAgentPath string) *ContainerManager {
	t.Helper()
	cm := newCMTest(t)
	cm.kandevHomeDir = "/kandev-home"
	cm.resolveAgentctlBinary = func() (string, error) { return agentctlPath, nil }
	cm.resolveMockAgentBinary = func() (string, error) { return mockAgentPath, nil }
	return cm
}

// TestLocalMountSetIsUnchanged characterizes the mount set the shipped Docker
// executor produces. Task 02 moves these sources behind a provider so a remote
// daemon can supply them from the remote host instead; this test exists so
// that refactor cannot quietly change what a local container mounts.
func TestLocalMountSetIsUnchanged(t *testing.T) {
	cm := newCMTestWithHostFiles(t, "/host/bin/agentctl", "")

	// An agent with a full SessionDirTemplate+SessionDirTarget pair, because
	// only those add the session-dir bind mount.
	cfg := ContainerConfig{
		AgentConfig: agents.NewCodexACP(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
		SessionID:   "session-1",
	}

	got, err := cm.buildContainerConfig(cfg)
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	pairs := mountPairs(got.Mounts)

	// The agentctl helper is mounted from the host so user-built images do
	// not have to bake it in. This is the first source a remote daemon cannot
	// resolve.
	if !containsPair(pairs, "/host/bin/agentctl->/usr/local/bin/agentctl:ro") {
		t.Errorf("agentctl mount missing; mounts = %v", pairs)
	}

	// The per-instance session dir lives under the Kandev home, not the
	// user's home. This is the second host-resolved source.
	wantSessionRoot := filepath.Join("/kandev-home", "agent-sessions", "0123456789abcdef")
	if !hasSourcePrefix(pairs, wantSessionRoot) {
		t.Errorf("no mount sourced from %q; mounts = %v", wantSessionRoot, pairs)
	}

	// No mock-agent mount in the production case.
	if hasSourcePrefix(pairs, "/host/bin/mock-agent") {
		t.Errorf("mock-agent mounted when resolver returned empty; mounts = %v", pairs)
	}

	// Workspace content is cloned inside the container, never bind-mounted.
	for _, p := range pairs {
		if strings.HasPrefix(p, "->") {
			t.Errorf("mount with empty source: %q", p)
		}
	}
}

// TestLocalMountSetIncludesMockAgentWhenResolved covers the Docker E2E path,
// which is the third host-resolved source.
func TestLocalMountSetIncludesMockAgentWhenResolved(t *testing.T) {
	cm := newCMTestWithHostFiles(t, "/host/bin/agentctl", "/host/bin/mock-agent")

	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
	})
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	if !containsPair(mountPairs(got.Mounts), "/host/bin/mock-agent->/usr/local/bin/mock-agent:ro") {
		t.Errorf("mock-agent mount missing; mounts = %v", mountPairs(got.Mounts))
	}
}

func containsPair(pairs []string, want string) bool {
	for _, p := range pairs {
		if p == want {
			return true
		}
	}
	return false
}

func hasSourcePrefix(pairs []string, prefix string) bool {
	for _, p := range pairs {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// fakeRemoteStore records what the remote provider asks for and answers with
// remote-looking paths.
type fakeRemoteStore struct {
	agentctlPath  string
	agentctlErr   error
	sessionDir    string
	sessionErr    error
	gotPlatform   SSHRemotePlatform
	gotInstanceID string
}

func (f *fakeRemoteStore) EnsureAgentctl(platform SSHRemotePlatform) (string, error) {
	f.gotPlatform = platform
	return f.agentctlPath, f.agentctlErr
}

func (f *fakeRemoteStore) EnsureSessionDir(instanceID string) (string, error) {
	f.gotInstanceID = instanceID
	return f.sessionDir, f.sessionErr
}

// TestRemoteHostFilesResolveOnTheRemote is the contract that makes a remote
// daemon possible: every mount source must be a path on the remote host. A
// backend-host path here would be resolved by the remote daemon against its
// own filesystem, silently mounting the wrong thing or nothing at all.
func TestRemoteHostFilesResolveOnTheRemote(t *testing.T) {
	store := &fakeRemoteStore{
		agentctlPath: "/home/dev/.kandev/bin/agentctl",
		sessionDir:   "/home/dev/.kandev/agent-sessions/0123456789abcdef",
	}
	provider := newRemoteContainerHostFiles(store, SSHRemotePlatform{GOOS: "linux", GOARCH: "arm64"}, NewCommandBuilder())

	cm := newCMTest(t)
	cm.kandevHomeDir = "/kandev-home"
	cm.resolveAgentctlBinary = func() (string, error) {
		t.Fatal("remote provider resolved the agentctl binary on the backend host")
		return "", nil
	}
	cm.containerHostFiles = provider

	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig: agents.NewCodexACP(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
	})
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	pairs := mountPairs(got.Mounts)
	if !containsPair(pairs, "/home/dev/.kandev/bin/agentctl->/usr/local/bin/agentctl:ro") {
		t.Errorf("agentctl not mounted from the remote path; mounts = %v", pairs)
	}
	if hasSourcePrefix(pairs, "/kandev-home") {
		t.Errorf("a backend-host path leaked into a remote container; mounts = %v", pairs)
	}
	if store.gotInstanceID != "0123456789abcdef" {
		t.Errorf("session dir requested for %q, want the instance ID", store.gotInstanceID)
	}
}

// TestRemoteHostFilesSelectsProbedPlatform fixes the current Docker executor
// limitation of always choosing the linux/amd64 helper. That is invisible with
// a local daemon on an amd64 host and fatal against an arm64 remote.
func TestRemoteHostFilesSelectsProbedPlatform(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			store := &fakeRemoteStore{agentctlPath: "/remote/agentctl"}
			provider := newRemoteContainerHostFiles(store, SSHRemotePlatform{GOOS: "linux", GOARCH: arch}, NewCommandBuilder())

			if _, err := provider.AgentctlBinary(); err != nil {
				t.Fatalf("AgentctlBinary: %v", err)
			}
			if store.gotPlatform.GOARCH != arch {
				t.Fatalf("resolved helper for arch %q, want %q", store.gotPlatform.GOARCH, arch)
			}
		})
	}
}

// TestRemoteHostFilesNeverMountsAMockAgent keeps the E2E-only host binary out
// of a remote container, where the backend path does not exist.
func TestRemoteHostFilesNeverMountsAMockAgent(t *testing.T) {
	provider := newRemoteContainerHostFiles(&fakeRemoteStore{}, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, NewCommandBuilder())
	path, err := provider.MockAgentBinary()
	if err != nil {
		t.Fatalf("MockAgentBinary: %v", err)
	}
	if path != "" {
		t.Fatalf("MockAgentBinary = %q, want empty for a remote daemon", path)
	}
}

// TestRemoteHostFilesFailsBeforeLaunchOnUnsupportedPlatform surfaces the cause
// rather than starting a container whose helper cannot execute.
func TestRemoteHostFilesFailsBeforeLaunchOnUnsupportedPlatform(t *testing.T) {
	provider := newRemoteContainerHostFiles(&fakeRemoteStore{}, SSHRemotePlatform{GOOS: "plan9", GOARCH: "mips"}, NewCommandBuilder())
	if _, err := provider.AgentctlBinary(); err == nil {
		t.Fatal("AgentctlBinary on an unsupported platform = nil error, want error")
	}
}

// TestRemoteLaunchConfigDropsLocalClonePath is finding 3 from branch review.
//
// LocalClonePath is a path on the backend host. Forwarding it to a remote
// daemon makes the daemon resolve it against its own filesystem, which either
// mounts the wrong directory or fails the launch. A remote container must
// carry no backend-host mount source at all.
func TestRemoteLaunchConfigDropsLocalClonePath(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		Metadata: map[string]interface{}{
			"repository_clone_url": "/home/dev/src/project",
		},
	}

	local, err := buildDockerContainerConfig(req, "local_docker")
	if err != nil {
		t.Fatalf("local config: %v", err)
	}
	if local.LocalClonePath == "" {
		t.Fatal("local_docker lost its clone mount; this test would not detect the remote bug")
	}

	remote, err := buildDockerContainerConfig(req, "remote_docker")
	if err != nil {
		t.Fatalf("remote config: %v", err)
	}
	if remote.LocalClonePath != "" {
		t.Fatalf("remote_docker forwarded the backend-host clone path %q", remote.LocalClonePath)
	}
}
