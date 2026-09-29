package lifecycle

import (
	"context"
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
	cm.resolveAgentctlBinary = func(context.Context, PrepareProgressCallback) (string, error) {
		return agentctlPath, nil
	}
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

func TestBuildContainerConfigPassesLaunchContextAndProgressToAgentctlResolver(t *testing.T) {
	cm := newCMTest(t)
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "launch")
	var gotContext context.Context
	var gotProgress []int
	cm.resolveAgentctlBinary = func(resolverCtx context.Context, onProgress PrepareProgressCallback) (string, error) {
		gotContext = resolverCtx
		if onProgress == nil {
			t.Fatal("agentctl resolver did not receive launch progress callback")
		}
		onProgress(PrepareStep{}, 0, 1)
		return "/host/bin/agentctl", nil
	}
	config := ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		OnProgress: func(_ PrepareStep, index, total int) {
			gotProgress = append(gotProgress, index, total)
		},
	}

	got, err := cm.buildContainerConfigWithContext(ctx, config)
	if err != nil {
		t.Fatalf("buildContainerConfigWithContext: %v", err)
	}
	if gotContext != ctx || gotContext.Value(contextKey{}) != "launch" {
		t.Fatalf("resolver context = %v; want original launch context", gotContext)
	}
	if len(gotProgress) != 2 || gotProgress[0] != 1 || gotProgress[1] != 2 {
		t.Fatalf("progress = %v; want forwarded step index 1 of 2", gotProgress)
	}
	if !containsPair(mountPairs(got.Mounts), "/host/bin/agentctl->/usr/local/bin/agentctl:ro") {
		t.Fatalf("agentctl mount missing; mounts = %v", mountPairs(got.Mounts))
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
