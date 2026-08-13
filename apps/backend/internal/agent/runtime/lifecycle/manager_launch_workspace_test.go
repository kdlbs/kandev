package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestResolveResumeWorktreePathUsesProviderPath pins the worktree-resume
// branch: the workspace comes from the persisted session, and the main repo's
// git dir is derived from the request so the executor can find the parent repo.
func TestResolveResumeWorktreePathUsesProviderPath(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-1": {SessionID: "session-1", WorkspacePath: "/work/task-1/backend"},
	}}
	req := &LaunchRequest{
		TaskID: "task-1", SessionID: "session-1",
		RepositoryPath: "/repos/backend", WorktreeID: "wt-1",
	}

	ws, gitDir, worktreeID, branch := mgr.resolveResumeWorktreePath(context.Background(), req)

	require.Equal(t, "/work/task-1/backend", ws)
	require.Equal(t, filepath.Join("/repos/backend", ".git"), gitDir)
	require.Equal(t, "wt-1", worktreeID)
	require.Empty(t, branch, "a resume reuses the existing branch rather than naming a new one")
}

func TestResolveResumeWorktreePathWithoutRepositoryPath(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-1": {SessionID: "session-1", WorkspacePath: "/work/task-1"},
	}}

	ws, gitDir, _, _ := mgr.resolveResumeWorktreePath(context.Background(),
		&LaunchRequest{TaskID: "task-1", SessionID: "session-1"})

	require.Equal(t, "/work/task-1", ws)
	require.Empty(t, gitDir)
}

func TestResolveResumeWorktreePathUnresolvableWorkspace(t *testing.T) {
	mgr := newTestManager(t)

	ws, gitDir, worktreeID, branch := mgr.resolveResumeWorktreePath(context.Background(),
		&LaunchRequest{TaskID: "task-1", SessionID: "session-1", WorktreeID: "wt-1"})

	require.Empty(t, ws)
	require.Empty(t, gitDir)
	require.Empty(t, worktreeID, "an unresolvable workspace must not leak a worktree ID")
	require.Empty(t, branch)
}

// TestLaunchResolveWorkspacePathSkipsCloneBasedExecutors pins the containment
// rule: a host checkout must never be handed to a clone-based executor.
func TestLaunchResolveWorkspacePathSkipsCloneBasedExecutors(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&capabilityExecutor{
		MockExecutor:  MockExecutor{name: "sprites"},
		requiresClone: true,
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	ws, gitDir, worktreeID, branch := mgr.launchResolveWorkspacePath(context.Background(), &LaunchRequest{
		ExecutorType:   string(models.ExecutorTypeSprites),
		WorkspacePath:  "/host/checkout",
		RepositoryPath: "/repos/backend",
		SessionID:      "session-1",
	})

	require.Empty(t, ws, "a clone-based executor owns its own filesystem")
	require.Empty(t, gitDir)
	require.Empty(t, worktreeID)
	require.Empty(t, branch)
}

func TestLaunchResolveWorkspacePathFallsBackToRepositoryPath(t *testing.T) {
	mgr := newTestManager(t)

	ws, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), &LaunchRequest{
		RepositoryPath: "/repos/backend",
		SessionID:      "session-1",
	})

	require.Equal(t, "/repos/backend", ws,
		"a launch with no explicit workspace works directly in the repository")
}

// TestLaunchResolveWorkspacePathDefersWorktreeCreation pins that a fresh
// worktree launch returns nothing — the WorktreePreparer owns creation.
func TestLaunchResolveWorkspacePathDefersWorktreeCreation(t *testing.T) {
	mgr := newTestManager(t)

	ws, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), &LaunchRequest{
		UseWorktree:    true,
		RepositoryPath: "/repos/backend",
		SessionID:      "session-1",
	})

	require.Empty(t, ws, "a first worktree launch has no ACP session and defers to the preparer")
}

func TestTruncateID(t *testing.T) {
	require.Equal(t, "abc", truncateID("abc", 8))
	require.Equal(t, "abcdefgh", truncateID("abcdefgh", 8))
	require.Equal(t, "abcdefgh", truncateID("abcdefghijkl", 8))
}

func TestExecutorRunningStatusFromExecution(t *testing.T) {
	require.Equal(t, models.ExecutorRunningStatusStarting, executorRunningStatusFromExecution(nil))

	for status, want := range map[v1.AgentStatus]string{
		v1.AgentStatusRunning:   models.ExecutorRunningStatusRunning,
		v1.AgentStatusReady:     models.ExecutorRunningStatusReady,
		v1.AgentStatusFailed:    models.ExecutorRunningStatusFailed,
		v1.AgentStatusStopped:   models.ExecutorRunningStatusStopped,
		v1.AgentStatusCompleted: models.ExecutorRunningStatusComplete,
		v1.AgentStatusStarting:  models.ExecutorRunningStatusStarting,
		"":                      models.ExecutorRunningStatusStarting,
	} {
		require.Equal(t, want, executorRunningStatusFromExecution(&AgentExecution{Status: status}),
			"status %q", status)
	}
}

func TestIsTerminalExecutorRunningStatus(t *testing.T) {
	require.True(t, isTerminalExecutorRunningStatus(models.ExecutorRunningStatusFailed))
	require.True(t, isTerminalExecutorRunningStatus(models.ExecutorRunningStatusStopped))
	require.True(t, isTerminalExecutorRunningStatus(models.ExecutorRunningStatusComplete))
	require.False(t, isTerminalExecutorRunningStatus(models.ExecutorRunningStatusRunning),
		"a live row must keep its local liveness handle")
	require.False(t, isTerminalExecutorRunningStatus(models.ExecutorRunningStatusStarting))
}
