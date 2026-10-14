//go:build linux

package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/processidentity"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRetiredCleanupRequiresOriginalProcessTermination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, "/bin/sleep", "60")
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, process.Start())
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
	server := newStandaloneControlServer(t, true)
	host, port := splitTestServerHostPort(t, server.server)
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "retired-cleanup")
	t.Cleanup(owner.Stop)
	old, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, old.Configure(host, port, "old-secret", process.Process.Pid, nil))
	identity, err := processidentity.Capture(process.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, old.SetProcessIdentity(identity))
	require.NoError(t, old.Commit())
	lease, err := owner.Acquire(ctx)
	require.NoError(t, err)
	client := lease.NewBoundInstanceClient(port, newTestLogger())
	lease.Close()
	manager := newTestManager(t)
	manager.SetRuntimeOwner(owner)
	manager.executorRegistry = NewExecutorRegistry(newTestLogger())
	standalone := NewStandaloneExecutor(nil, "", 0, newTestLogger())
	standalone.SetRuntimeOwner(owner)
	manager.executorRegistry.Register(standalone)
	execution := &AgentExecution{ID: "old", SessionID: "session", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusReady, runtimeEpoch: old.Epoch(), standaloneInstanceID: "old", agentctl: client}
	require.NoError(t, manager.executionStore.Add(execution))
	require.True(t, owner.MarkUnavailableEpoch(old.Epoch(), agentctl.AvailabilityReasonAgentctlExited))
	next, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, next.Configure(host, port, "new-secret", os.Getpid(), nil))
	require.NoError(t, next.Commit())
	require.Error(t, manager.CleanupStaleExecutionBySessionIDIfCurrent(ctx, "session", "old", time.Time{}), "a retired lease alone is insufficient")
	require.NoError(t, process.Process.Kill())
	require.Error(t, process.Wait())
	require.NoError(t, manager.CleanupStaleExecutionBySessionIDIfCurrent(ctx, "session", "old", time.Time{}))
	_, exists := manager.GetExecutionBySessionID("session")
	require.False(t, exists)
	server.mu.Lock()
	defer server.mu.Unlock()
	require.Empty(t, server.deleted, "obsolete cleanup must not send Stop to the successor")
}
