package lifecycle

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.3
func TestAgentctlURLPersistenceDuringWorkspaceCallbackDetach(t *testing.T) {
	ctx := context.Background()
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "lease.db"))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Lease"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Lease"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}))
	manager := newTestManager(t)
	manager.SetExecutorRunningWriter(repo)
	client := agentctl.NewClient("127.0.0.1", 41001, newTestLogger())
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session",
		Status: v1.AgentStatusRunning, agentctl: client}
	require.NoError(t, manager.executionStore.Add(execution))
	require.NoError(t, manager.persistExecutorRunningResult(ctx, execution))
	before, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	require.Equal(t, client.BaseURL(), before.AgentctlURL)

	entered, release, callbackDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	stopCh := make(chan struct{})
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnGitStatus: func(*AgentExecution, *agentctl.GitStatusUpdate) { close(entered); <-release },
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, sm)
	callbacks := sm.buildWorkspaceCallbacks(execution, client)
	go func() { defer close(callbackDone); callbacks.OnGitStatus(&agentctl.GitStatusUpdate{}) }()
	<-entered
	detachDone := make(chan struct{})
	go func() {
		defer close(detachDone)
		execution.agentctlLifecycleMu.Lock()
		defer execution.agentctlLifecycleMu.Unlock()
		execution.detachAgentctlClient()
	}()
	t.Cleanup(func() { releaseCallback(); <-callbackDone; <-detachDone })
	waitForPendingAgentctlSourceWriter(t, execution)
	require.NoError(t, manager.persistExecutorRunningResult(ctx, execution))
	during, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	require.Equal(t, client.BaseURL(), during.AgentctlURL, "detach must not blank the still-installed endpoint")
	require.Equal(t, 41001, during.AgentctlPort)
	require.NotNil(t, during.LastSeenAt)

	releaseCallback()
	<-detachDone
	require.NoError(t, manager.persistExecutorRunningResult(ctx, execution))
	after, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	require.Empty(t, after.AgentctlURL, "a completed detach must clear the persisted endpoint")
	require.Zero(t, after.AgentctlPort)
	require.Nil(t, after.LastSeenAt)
}

func waitForPendingAgentctlSourceWriter(t *testing.T, execution *AgentExecution) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for execution.agentctlSourceMu.TryRLock() {
		execution.agentctlSourceMu.RUnlock()
		if time.Now().After(deadline) {
			t.Fatal("client writer did not reach the active workspace callback")
		}
		runtime.Gosched()
	}
}
