package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

type nodeFixture struct {
	dir, node, profile, child, receipt string
}

// newNodeFixture writes stub wrappers that record who ran, drop their own "--"
// marker, and exec the rest of the command line.
func newNodeFixture(t *testing.T) nodeFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	f := nodeFixture{dir: t.TempDir()}
	f.node = filepath.Join(f.dir, "node-wrapper")
	f.profile = filepath.Join(f.dir, "profile-wrapper")
	f.child = filepath.Join(f.dir, "child")
	f.receipt = filepath.Join(f.dir, "receipt")
	stub := func(name string) string {
		return "#!/bin/sh\nprintf '" + name + "\\n' >> \"$RECEIPT\"\nshift\nexec \"$@\"\n"
	}
	require.NoError(t, os.WriteFile(f.node, []byte(stub("node")), 0o700))
	require.NoError(t, os.WriteFile(f.profile, []byte(stub("profile")), 0o700))
	require.NoError(t, os.WriteFile(f.child, []byte("#!/bin/sh\nprintf 'child\\n' >> \"$RECEIPT\"\n"), 0o700))
	return f
}

func (f nodeFixture) nodeSpec() launchwrapper.Spec {
	return launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{f.node, "--"}}
}

func (f nodeFixture) config(profile bool) *config.InstanceConfig {
	cfg := &config.InstanceConfig{
		WorkDir: f.dir, AgentArgs: []string{f.child}, ContinueArgs: []string{f.child},
		AgentEnv:          []string{"RECEIPT=" + f.receipt},
		NodeLaunchWrapper: launchwrapper.Node{Spec: f.nodeSpec(), Required: true},
	}
	if profile {
		cfg.LaunchWrapper = launchwrapper.Profile([]string{f.profile, "--"})
		cfg.LaunchWrapperRequired = true
	}
	return cfg
}

func (f nodeFixture) receiptLines(t *testing.T) string {
	t.Helper()
	got, err := os.ReadFile(f.receipt)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(got)
}

// @covers AC-AGENTS-NODEWRAP-002.1 AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperIsFirstArgvOfSessionCommand(t *testing.T) {
	for _, profile := range []bool{false, true} {
		f := newNodeFixture(t)
		manager := NewManager(f.config(profile), newTestLogger(t))
		manager.adapter = newStubAdapter()
		require.NoError(t, manager.buildFinalCommand())
		require.Equal(t, f.node, manager.cmd.Path)
		require.Equal(t, f.node, manager.cmd.Args[0], "node argv is the first argv of the final command")
		if profile {
			require.Equal(t, []string{f.node, "--", f.profile, "--", f.child}, manager.cmd.Args)
		} else {
			require.Equal(t, []string{f.node, "--", f.child}, manager.cmd.Args)
		}
		require.NoError(t, manager.cmd.Run())
		want := "node\nchild\n"
		if profile {
			want = "node\nprofile\nchild\n"
		}
		require.Equal(t, want, f.receiptLines(t))
		require.NotContains(t, manager.GetFinalCommand(), f.node)
	}
}

// @covers AC-AGENTS-NODEWRAP-003.3
func TestNodeWrapperRequiredButMissingRefusesSessionCommand(t *testing.T) {
	f := newNodeFixture(t)
	cfg := f.config(false)
	cfg.NodeLaunchWrapper = launchwrapper.Node{Required: true}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.ErrorIs(t, manager.buildFinalCommand(), launchwrapper.ErrNodeRequired)
	require.Nil(t, manager.cmd)
	require.Empty(t, f.receiptLines(t))
}

// @covers AC-AGENTS-NODEWRAP-003.3
func TestNodeWrapperUnavailableAtLaunchRefusesWithoutRetry(t *testing.T) {
	f := newNodeFixture(t)
	require.NoError(t, os.Remove(f.node))
	manager := NewManager(f.config(false), newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.Error(t, manager.buildFinalCommand())
	require.Nil(t, manager.cmd)
}

// @covers AC-AGENTS-NODEWRAP-002.2
func TestProfileConfigurationCannotClearNodeWrapper(t *testing.T) {
	f := newNodeFixture(t)
	cfg := f.config(true)
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	for _, wrapper := range [][]launchwrapper.Spec{nil, {{}}, {launchwrapper.Profile([]string{f.profile, "--"})}} {
		require.NoError(t, manager.Configure(f.child, []string{f.child}, true, nil, "", nil, false, wrapper...))
		require.NoError(t, manager.buildFinalCommand())
		require.Equal(t, f.node, manager.cmd.Args[0])
		require.Equal(t, f.nodeSpec(), cfg.NodeLaunchWrapper.Spec)
	}
	snapshot, err := manager.LaunchWrapperSnapshot()
	require.NoError(t, err)
	require.NotContains(t, snapshot.Argv, f.node, "node wrapper is not in the profile snapshot")
	require.Equal(t, launchwrapper.SourceProfile, snapshot.Source)
}

// @covers AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperOneShotArgsAreWrappedExactlyOnce(t *testing.T) {
	f := newNodeFixture(t)
	cfg := f.config(false)
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newOneShotStubAdapter()
	manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: cfg.AgentArgs, ContinueArgs: cfg.ContinueArgs}}
	for range 3 {
		require.NoError(t, manager.startOneShot())
		oneShot := manager.adapterCfg.OneShotConfig
		require.Equal(t, []string{f.node, "--", f.child}, oneShot.InitialArgs)
		require.Equal(t, []string{f.node, "--", f.child}, oneShot.ContinueArgs)
		close(manager.stopCh)
		manager.wg.Wait()
	}
	t.Cleanup(func() { manager.workspaceTracker.Stop() })
}

// @covers AC-AGENTS-NODEWRAP-003.3
func TestNodeWrapperRequiredButMissingRefusesOneShot(t *testing.T) {
	f := newNodeFixture(t)
	cfg := f.config(false)
	cfg.NodeLaunchWrapper = launchwrapper.Node{Required: true}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newOneShotStubAdapter()
	manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: cfg.AgentArgs, ContinueArgs: cfg.ContinueArgs}}
	require.ErrorIs(t, manager.startOneShot(), launchwrapper.ErrNodeRequired)
	require.Equal(t, cfg.AgentArgs, manager.adapterCfg.OneShotConfig.InitialArgs)
}

// @covers AC-AGENTS-NODEWRAP-003.4
func TestNoNodeWrapperLeavesSessionCommandUnchanged(t *testing.T) {
	f := newNodeFixture(t)
	cfg := f.config(false)
	cfg.NodeLaunchWrapper = launchwrapper.Node{}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.NoError(t, manager.buildFinalCommand())
	require.Equal(t, []string{f.child}, manager.cmd.Args)
}

// @covers AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperAppliesToInteractivePassthrough(t *testing.T) {
	f := newNodeFixture(t)
	manager := NewManager(f.config(false), newTestLogger(t))
	req := manager.Passthrough().runnerRequest(passthroughStartForTest(f), "process-1")
	require.Equal(t, f.nodeSpec(), req.NodeLaunchWrapper.Spec)
	require.True(t, req.NodeLaunchWrapper.Required)

	runner := NewInteractiveRunner(nil, newTestLogger(t), 1024)
	req.Env = map[string]string{"RECEIPT": f.receipt}
	req.Command = []string{f.child}
	req.WorkingDir = f.dir
	info, err := runner.Start(context.Background(), req)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = runner.Stop(ctx, info.ID)
	})
	require.Eventually(t, func() bool { return f.receiptLines(t) == "node\nchild\n" }, 3*time.Second, 10*time.Millisecond)
}

// @covers AC-AGENTS-NODEWRAP-003.3
func TestNodeWrapperRequiredButMissingRefusesInteractivePassthrough(t *testing.T) {
	f := newNodeFixture(t)
	runner := NewInteractiveRunner(nil, newTestLogger(t), 1024)
	_, err := runner.Start(context.Background(), InteractiveStartRequest{
		SessionID: "s", Command: []string{f.child}, WorkingDir: f.dir, ImmediateStart: true,
		Env:               map[string]string{"RECEIPT": f.receipt},
		NodeLaunchWrapper: launchwrapper.Node{Required: true},
	})
	require.Error(t, err)
	require.Empty(t, f.receiptLines(t))
}

// @covers AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperDoesNotWrapUserShells(t *testing.T) {
	f := newNodeFixture(t)
	runner := NewInteractiveRunner(nil, newTestLogger(t), 1024)
	info, err := runner.Start(context.Background(), InteractiveStartRequest{
		SessionID: "s", Command: []string{f.child}, WorkingDir: f.dir, ImmediateStart: true, IsUserShell: true,
		Env: map[string]string{"RECEIPT": f.receipt},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = runner.Stop(ctx, info.ID)
	})
	require.Eventually(t, func() bool { return f.receiptLines(t) == "child\n" }, 3*time.Second, 10*time.Millisecond)
}

func passthroughStartForTest(f nodeFixture) types.PassthroughStartRequest {
	return types.PassthroughStartRequest{SessionID: "s", OperationID: "o", Command: []string{f.child}}
}
