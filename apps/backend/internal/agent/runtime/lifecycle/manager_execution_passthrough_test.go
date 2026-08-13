package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestResolveSessionRuntimeFallbackOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("empty session id", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.ResolveSessionRuntime(ctx, "")
		require.ErrorContains(t, err, "session_id is required")
	})

	t.Run("in-memory execution wins", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1", ExecutorType: string(models.ExecutorTypeSSH)},
		}}
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "exec-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeDocker,
		}))

		got, err := mgr.ResolveSessionRuntime(ctx, "session-1")
		require.NoError(t, err)
		require.Equal(t, agentruntime.RuntimeDocker, got,
			"a live execution's runtime is authoritative over the persisted executor type")
	})

	t.Run("no provider configured", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.ResolveSessionRuntime(ctx, "session-1")
		require.ErrorContains(t, err, "workspace info provider not configured")
	})

	t.Run("provider error", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{err: errors.New("db down")}
		_, err := mgr.ResolveSessionRuntime(ctx, "session-1")
		require.ErrorContains(t, err, "failed to resolve runtime for session session-1")
	})

	t.Run("session not found", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{}}
		_, err := mgr.ResolveSessionRuntime(ctx, "session-1")
		require.ErrorContains(t, err, "session session-1 not found")
	})

	t.Run("falls back to stored runtime name", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1", RuntimeName: agentruntime.RuntimeSprites},
		}}
		got, err := mgr.ResolveSessionRuntime(ctx, "session-1")
		require.NoError(t, err)
		require.Equal(t, agentruntime.RuntimeSprites, got,
			"legacy records carry only a runtime name")
	})

	t.Run("defaults to standalone", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1"},
		}}
		got, err := mgr.ResolveSessionRuntime(ctx, "session-1")
		require.NoError(t, err)
		require.Equal(t, agentruntime.RuntimeStandalone, got)
	})
}

func TestVerifyPassthroughEnabledRejectsNonPassthroughProfile(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{cliPassthrough: false}

	_, err := mgr.verifyPassthroughEnabled(context.Background(), "session-1", "profile-1")

	require.ErrorContains(t, err, "is not configured for CLI passthrough mode")
}

func TestVerifyPassthroughEnabledSurfacesResolveFailure(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{err: errors.New("profile deleted")}

	_, err := mgr.verifyPassthroughEnabled(context.Background(), "session-1", "profile-1")

	require.ErrorContains(t, err, "failed to resolve profile profile-1")
	require.ErrorContains(t, err, "profile deleted")
}

func TestVerifyPassthroughEnabledWithoutResolver(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = nil

	_, err := mgr.verifyPassthroughEnabled(context.Background(), "session-1", "profile-1")

	require.ErrorContains(t, err, "has no profile configured for passthrough mode")
}

// TestEnsurePassthroughExecutionChecksAccessBeforeCacheLookup pins that a
// non-owner cannot reach a cached execution: the scoping check runs before the
// execution store is consulted.
func TestEnsurePassthroughExecutionChecksAccessBeforeCacheLookup(t *testing.T) {
	denied := errors.New("session access denied")
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-1": {TaskID: "task-1", SessionID: "session-1", WorkspacePath: "/work"},
	}}
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = provider
	mgr.SetSessionAccessChecker(func(context.Context, string) error { return denied })
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-1", SessionID: "session-1"}))

	_, err := mgr.EnsurePassthroughExecution(context.Background(), "session-1")

	require.ErrorIs(t, err, denied)
	require.Zero(t, provider.sessionCalls, "authorization must run before any lookup work")
}

// TestEnsurePassthroughExecutionResumesExistingExecution covers the recovery
// branch taken after a backend restart: an execution exists but its PTY does
// not, so the resume path runs (and here fails for lack of a runner).
func TestEnsurePassthroughExecutionResumesExistingExecution(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: "claude-acp", cliPassthrough: true}
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1", AgentProfileID: "profile-1",
		// A stale process ID: the runner no longer knows this process.
		PassthroughProcessID: "pty-dead",
	}))

	_, err := mgr.EnsurePassthroughExecution(context.Background(), "session-1")

	require.ErrorContains(t, err, "resume passthrough session session-1",
		"a stale process ID must fall through to the resume path rather than being returned as live")
	require.ErrorContains(t, err, "interactive runner not available")
}

func TestCreateExecutionFromSessionInfoRequiresProvider(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.EnsurePassthroughExecution(context.Background(), "session-1")

	require.ErrorContains(t, err, "cannot restore session session-1: workspace info provider not configured")
}

func TestCreateExecutionFromSessionInfoSurfacesProviderError(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{err: errors.New("db down")}

	_, err := mgr.EnsurePassthroughExecution(context.Background(), "session-1")

	require.ErrorContains(t, err, "get workspace info for session session-1")
	require.ErrorContains(t, err, "db down")
}

func TestCreateExecutionFromSessionInfoRequiresWorkspaceAndTask(t *testing.T) {
	ctx := context.Background()

	t.Run("no workspace path", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {TaskID: "task-1", SessionID: "session-1"},
		}}
		_, err := mgr.EnsurePassthroughExecution(ctx, "session-1")
		require.ErrorIs(t, err, ErrSessionWorkspaceNotReady,
			"the terminal handler retries on this sentinel instead of surfacing a hard failure")
	})

	t.Run("no task id", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1", WorkspacePath: "/work"},
		}}
		_, err := mgr.EnsurePassthroughExecution(ctx, "session-1")
		require.ErrorContains(t, err, "session session-1 has no associated task ID")
	})
}

// TestCreateExecutionFromSessionInfoRejectsNonPassthroughProfile pins that the
// terminal recovery path refuses to spawn a PTY for an ACP session.
func TestCreateExecutionFromSessionInfoRejectsNonPassthroughProfile(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{cliPassthrough: false}
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-1": {
			TaskID: "task-1", SessionID: "session-1", WorkspacePath: "/work",
			ExecutionProfileID: "profile-1",
		},
	}}

	_, err := mgr.EnsurePassthroughExecution(context.Background(), "session-1")

	require.ErrorContains(t, err, "is not configured for CLI passthrough mode")
	_, exists := mgr.GetExecutionBySessionID("session-1")
	require.False(t, exists, "a rejected passthrough recovery must not leave an execution behind")
}

func TestApplyResumeIntentDerivesFromPreviousExecution(t *testing.T) {
	fresh := &AgentExecution{ID: "exec-1"}
	applyResumeIntent(fresh, &ExecutorCreateRequest{})
	require.False(t, fresh.isResumedSession,
		"a session that never ran has no CLI conversation for a resume flag to attach to")

	recovered := &AgentExecution{ID: "exec-2"}
	applyResumeIntent(recovered, &ExecutorCreateRequest{PreviousExecutionID: "exec-1"})
	require.True(t, recovered.isResumedSession)
}

func TestWorkspaceProfileIDHelpers(t *testing.T) {
	require.Empty(t, workspaceExecutionProfileID(nil))
	require.Empty(t, workspaceExecutorProfileID(nil))

	info := &WorkspaceInfo{AgentProfileID: "office-1"}
	require.Equal(t, "office-1", workspaceExecutionProfileID(info),
		"a legacy session uses its Office identity as the execution profile")

	info.ExecutionProfileID = "exec-profile-1"
	require.Equal(t, "exec-profile-1", workspaceExecutionProfileID(info))

	info.ExecutorProfileID = "executor-profile-1"
	require.Equal(t, "executor-profile-1", workspaceExecutorProfileID(info))
}

func TestGetOrEnsureExecutionRequiresSessionID(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.GetOrEnsureExecution(context.Background(), "")

	require.ErrorContains(t, err, "session_id is required")
}

func TestGetOrEnsureExecutionForEnvironmentValidatesProviderResponse(t *testing.T) {
	ctx := context.Background()

	t.Run("empty environment id", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.GetOrEnsureExecutionForEnvironment(ctx, "")
		require.ErrorContains(t, err, "task_environment_id is required")
	})

	t.Run("no provider", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.GetOrEnsureExecutionForEnvironment(ctx, "env-1")
		require.ErrorContains(t, err, "workspace info provider not configured")
	})

	t.Run("provider returns a different environment", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{envInfos: map[string]*WorkspaceInfo{
			"env-1": {TaskEnvironmentID: "env-other", SessionID: "session-1", WorkspacePath: "/work"},
		}}
		_, err := mgr.GetOrEnsureExecutionForEnvironment(ctx, "env-1")
		require.ErrorContains(t, err, "workspace info resolved environment env-other, want env-1",
			"a mismatched environment must fail loudly rather than silently binding the wrong workspace")
	})

	t.Run("environment without workspace path", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{envInfos: map[string]*WorkspaceInfo{
			"env-1": {TaskEnvironmentID: "env-1", SessionID: "session-1"},
		}}
		_, err := mgr.GetOrEnsureExecutionForEnvironment(ctx, "env-1")
		require.ErrorIs(t, err, ErrSessionWorkspaceNotReady)
	})

	t.Run("environment without a session", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{envInfos: map[string]*WorkspaceInfo{
			"env-1": {TaskEnvironmentID: "env-1", WorkspacePath: "/work"},
		}}
		_, err := mgr.GetOrEnsureExecutionForEnvironment(ctx, "env-1")
		require.ErrorContains(t, err, "task environment env-1 has no task session")
	})
}

func TestEnsureWorkspaceExecutionForSessionRequiresSessionID(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "task-1", "")

	require.ErrorContains(t, err, "session_id is required")
}

func TestEnsureWorkspaceExecutionLockedReusesEnvironmentExecution(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-new": {
			TaskID: "task-1", SessionID: "session-new", TaskEnvironmentID: "env-1", WorkspacePath: "/work",
		},
	}}
	existing := &AgentExecution{
		ID: "exec-existing", TaskID: "task-1", SessionID: "session-old", TaskEnvironmentID: "env-1",
	}
	require.NoError(t, mgr.executionStore.Add(existing))

	got, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "task-1", "session-new")

	require.NoError(t, err)
	require.Same(t, existing, got,
		"sibling sessions in one task environment must share the existing execution, not create a second one")
}

func TestEnsureWorkspaceExecutionLockedRequiresProviderAndInfo(t *testing.T) {
	ctx := context.Background()

	t.Run("no provider", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.EnsureWorkspaceExecutionForSession(ctx, "task-1", "session-1")
		require.ErrorContains(t, err, "workspace info provider not configured")
	})

	t.Run("provider error", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{err: errors.New("db down")}
		_, err := mgr.EnsureWorkspaceExecutionForSession(ctx, "task-1", "session-1")
		require.ErrorContains(t, err, "failed to get workspace info for session session-1")
	})

	t.Run("session not found", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{}}
		_, err := mgr.EnsureWorkspaceExecutionForSession(ctx, "task-1", "session-1")
		require.ErrorContains(t, err, "session session-1 not found")
	})

	t.Run("workspace not ready", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {TaskID: "task-1", SessionID: "session-1"},
		}}
		_, err := mgr.EnsureWorkspaceExecutionForSession(ctx, "task-1", "session-1")
		require.ErrorIs(t, err, ErrSessionWorkspaceNotReady)
	})
}
