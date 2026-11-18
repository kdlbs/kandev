package process

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-MCP-APPLY-001.6
func TestMCPProviderReplacementPreservesWorkspace(t *testing.T) {
	cfg := persistentRestartConfig(t.TempDir())
	cfg.ShellEnabled = true
	mgr := newRestartManager(t, cfg)
	startRestartManager(t, mgr)
	t.Cleanup(func() { require.NoError(t, mgr.CloseWorkspacePreview(context.Background())) })
	terminal, err := mgr.StartTerminalShell("kept", shell.DefaultConfig(mgr.cfg.WorkDir))
	require.NoError(t, err)
	before := terminal.Status()
	embedded := mgr.shell
	require.NotNil(t, embedded)
	embeddedPID := embedded.Status().Pid
	preview, err := mgr.PublishWorkspacePreview(WorkspacePreviewRequest{Path: "index.html", Content: "preserved preview"})
	require.NoError(t, err)
	root, _ := mgr.snapshotTrackers()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command, env := fixtureShellExec("sleep 60")
	workload, err := mgr.StartProcess(context.Background(), StartProcessRequest{SessionID: "workspace", Kind: types.ProcessKindDev, Command: command, Env: env})
	require.NoError(t, err)
	source := mgr.ProcessGeneration()
	ticket, err := mgr.stopProviderForReplacement(ctx, source)
	require.NoError(t, err)
	require.Equal(t, before.Pid, terminal.Status().Pid)
	require.True(t, terminal.Status().Running)
	require.True(t, root.live())
	next, err := mgr.startProviderForReplacement(ctx, ticket)
	require.NoError(t, err)
	require.Greater(t, next, source)
	currentWorkload, ok := mgr.GetProcess(workload.ID, false)
	require.True(t, ok)
	require.Equal(t, types.ProcessStatusRunning, currentWorkload.Status)
	require.Equal(t, before.Pid, terminal.Status().Pid)
	current, _ := mgr.snapshotTrackers()
	require.Same(t, root, current)
	require.Same(t, embedded, mgr.shell)
	require.Equal(t, embeddedPID, mgr.shell.Status().Pid)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", preview.Port, preview.Path))
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "preserved preview", string(body))
	output := make(chan []byte, 32)
	terminal.Subscribe(output)
	defer terminal.Unsubscribe(output)
	_, err = terminal.Write([]byte("echo preserved-terminal\n"))
	require.NoError(t, err)
	var received strings.Builder
	for !strings.Contains(received.String(), "preserved-terminal") {
		select {
		case b := <-output:
			received.Write(b)
		case <-ctx.Done():
			t.Fatal("existing PTY did not produce output")
		}
	}
	_, err = mgr.stopProviderForReplacement(ctx, source)
	require.Error(t, err, "a stale source must not stop its successor")
	require.Equal(t, next, mgr.ProcessGeneration())
}

func TestMCPProviderReplacementStopWins(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	ctx := context.Background()
	ticket, err := mgr.stopProviderForReplacement(ctx, mgr.ProcessGeneration())
	require.NoError(t, err)
	require.NoError(t, mgr.Stop(ctx))
	_, err = mgr.startProviderForReplacement(ctx, ticket)
	require.Error(t, err)
	require.Equal(t, StatusStopped, mgr.Status())
}

func TestMCPProviderReplacementFailedStartPreservesWorkspace(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	terminal, err := mgr.StartTerminalShell("kept", shell.DefaultConfig(mgr.cfg.WorkDir))
	require.NoError(t, err)
	before := terminal.Status()
	ctx := context.Background()
	ticket, err := mgr.stopProviderForReplacement(ctx, mgr.ProcessGeneration())
	require.NoError(t, err)
	// Inject a missing executable to exercise the actual spawn failure.
	missing := t.TempDir() + "/missing-provider"
	mgr.cfg.AgentArgs = []string{missing}
	_, err = mgr.startProviderForReplacement(ctx, ticket)
	require.Error(t, err)
	require.True(t, terminal.Status().Running)
	require.Equal(t, before.Pid, terminal.Status().Pid)
	require.NoError(t, os.WriteFile(mgr.cfg.WorkDir+"/still-usable", []byte("workspace"), 0600))
}

func TestMCPProviderReplacementConfigurationFence(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	ctx := context.Background()
	ticket, err := mgr.stopProviderForReplacement(ctx, mgr.ProcessGeneration())
	require.NoError(t, err)
	require.NoError(t, mgr.Configure("", fixtureArgs(), true, nil, "", nil, false))
	_, err = mgr.startProviderForReplacement(ctx, ticket)
	require.Error(t, err)
	require.Equal(t, ticket.processGeneration, mgr.ProcessGeneration())
}

func TestMCPProviderReplacementConcurrentWorkspaceStart(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	t.Cleanup(func() { require.NoError(t, mgr.CloseWorkspacePreview(context.Background())) })
	entered, proceed := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(proceed) })
	defer release()
	mgr.managerWaitFn = func(ctx context.Context, done <-chan struct{}, timeout time.Duration) bool {
		close(entered)
		<-proceed
		return waitForManagerDone(ctx, done, timeout)
	}
	result := make(chan error, 1)
	go func() {
		_, err := mgr.stopProviderForReplacement(context.Background(), mgr.ProcessGeneration())
		result <- err
	}()
	<-entered
	// Start terminal and preview while provider teardown is held at its wait boundary.
	terminal, err := mgr.StartTerminalShell("during", shell.DefaultConfig(mgr.cfg.WorkDir))
	require.NoError(t, err)
	require.True(t, terminal.Status().Running)
	_, err = mgr.PublishWorkspacePreview(WorkspacePreviewRequest{Path: "index.html", Content: "during refresh"})
	require.NoError(t, err)
	release()
	require.NoError(t, <-result)
	mgr.managerWaitFn = nil
	require.True(t, terminal.Status().Running)
}

func TestMCPProviderReplacementRejectsForeignManager(t *testing.T) {
	first := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	second := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, first)
	startRestartManager(t, second)
	ctx := context.Background()
	foreign, err := first.stopProviderForReplacement(ctx, first.ProcessGeneration())
	require.NoError(t, err)
	_, err = second.stopProviderForReplacement(ctx, second.ProcessGeneration())
	require.NoError(t, err)
	_, err = second.startProviderForReplacement(ctx, foreign)
	require.Error(t, err)
	require.Equal(t, StatusStopped, second.Status())
}
