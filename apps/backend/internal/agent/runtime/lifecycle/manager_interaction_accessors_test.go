package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWasSessionInitializedReflectsExecutionFlag(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-init", sessionInitialized: true}))
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-pending"}))

	require.True(t, mgr.WasSessionInitialized("exec-init"))
	require.False(t, mgr.WasSessionInitialized("exec-pending"))
	require.False(t, mgr.WasSessionInitialized("exec-absent"), "an unknown execution is not initialized")
}

func TestGetAvailableCommandsForSessionReturnsCachedCommands(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	exec.SetAvailableCommands([]streams.AvailableCommand{
		{Name: "review", Description: "Review the diff"},
		{Name: "commit"},
	})
	require.NoError(t, mgr.executionStore.Add(exec))

	got := mgr.GetAvailableCommandsForSession("session-1")

	require.Len(t, got, 2)
	require.Equal(t, "review", got[0].Name)
	require.Equal(t, "Review the diff", got[0].Description)
	require.Equal(t, "commit", got[1].Name)
	require.Nil(t, mgr.GetAvailableCommandsForSession("session-absent"))
}

func TestGetModeStateForSessionReturnsCachedMode(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	exec.SetModeState(&CachedModeState{
		CurrentModeID:  "plan",
		AvailableModes: []streams.SessionModeInfo{{ID: "plan"}, {ID: "default"}},
	})
	require.NoError(t, mgr.executionStore.Add(exec))

	state := mgr.GetModeStateForSession("session-1")

	require.NotNil(t, state)
	require.Equal(t, "plan", state.CurrentModeID)
	require.Len(t, state.AvailableModes, 2)
	require.Nil(t, mgr.GetModeStateForSession("session-absent"))
}

func TestGetModelStateForSessionReturnsCachedModel(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	exec.SetModelState(&CachedModelState{CurrentModelID: "claude-opus-5"})
	require.NoError(t, mgr.executionStore.Add(exec))

	state := mgr.GetModelStateForSession("session-1")

	require.NotNil(t, state)
	require.Equal(t, "claude-opus-5", state.CurrentModelID)
	require.Nil(t, mgr.GetModelStateForSession("session-absent"))
}

func TestGetPromptGenerationForSessionTracksBeginPrompt(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", Status: v1.AgentStatusReady,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}))

	before, err := mgr.GetPromptGenerationForSession(context.Background(), "session-1")
	require.NoError(t, err)

	generation, err := mgr.BeginPrompt("exec-1")
	require.NoError(t, err)
	require.Greater(t, generation, before, "BeginPrompt must advance the prompt generation")

	after, err := mgr.GetPromptGenerationForSession(context.Background(), "session-1")
	require.NoError(t, err)
	require.Equal(t, generation, after,
		"the session-scoped read must observe the generation BeginPrompt handed out")
	require.True(t, mgr.OwnsPromptGeneration("session-1", "exec-1", generation))
	require.False(t, mgr.OwnsPromptGeneration("session-1", "exec-1", generation+1))
}

func TestGetPromptGenerationForSessionUnknownSession(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.GetPromptGenerationForSession(context.Background(), "session-absent")

	require.ErrorIs(t, err, ErrNoExecutionForSession)
}

// TestGetPromptActivityForSessionReturnsTrackedSnapshot is the codex P1 gap
// flagged during WO-38 Build round 7: GetPromptActivityForSession (the
// primitive the stuck-signal watchdog reads to gate reclaims on real elapsed
// inactivity, orchestrator/stuck_signal_watchdog.go) had no direct unit test
// of its own — only indirect coverage through the watchdog's mock. This pins
// its two branches: a tracked execution returns the execution ID, prompt
// generation, activity epoch, and lastActivityAt that markAgentActivity last
// recorded; an untracked session returns ErrNoExecutionForSession.
func TestGetPromptActivityForSessionReturnsTrackedSnapshot(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	require.NoError(t, mgr.executionStore.Add(exec))

	generation, err := mgr.BeginPrompt("exec-1")
	require.NoError(t, err)
	exec.markAgentActivity()
	wantActivityAt, _, wantEpoch := exec.promptActivitySnapshot()

	executionID, gotGeneration, gotEpoch, gotActivityAt, err := mgr.GetPromptActivityForSession(context.Background(), "session-1")

	require.NoError(t, err)
	require.Equal(t, "exec-1", executionID)
	require.Equal(t, generation, gotGeneration)
	require.Equal(t, wantEpoch, gotEpoch)
	require.True(t, wantActivityAt.Equal(gotActivityAt),
		"expected the session-scoped read to observe the same lastActivityAt markAgentActivity recorded")
}

func TestGetPromptActivityForSessionUnknownSession(t *testing.T) {
	mgr := newTestManager(t)

	executionID, generation, epoch, activityAt, err := mgr.GetPromptActivityForSession(context.Background(), "session-absent")

	require.ErrorIs(t, err, ErrNoExecutionForSession)
	require.Empty(t, executionID)
	require.Zero(t, generation)
	require.Zero(t, epoch)
	require.True(t, activityAt.IsZero())
}

func TestBeginPromptUnknownExecution(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.BeginPrompt("missing")

	require.ErrorContains(t, err, `execution "missing" not found`)
}

func TestGetExecutionIDForSession(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-42", SessionID: "session-42"}))

	id, err := mgr.GetExecutionIDForSession(context.Background(), "session-42")
	require.NoError(t, err)
	require.Equal(t, "exec-42", id)

	id, err = mgr.GetExecutionIDForSession(context.Background(), "session-absent")
	require.ErrorIs(t, err, ErrNoExecutionForSession)
	require.Empty(t, id)
}

func TestIsAgentCommandConfigured(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-agent", AgentCommand: "claude"}))
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-workspace"}))

	require.True(t, mgr.IsAgentCommandConfigured("exec-agent"))
	require.False(t, mgr.IsAgentCommandConfigured("exec-workspace"),
		"a workspace-only execution has not been promoted to an agent execution")
	require.False(t, mgr.IsAgentCommandConfigured("exec-absent"))
}

func TestResolveTaskEnvironmentIDPrefersInMemoryExecution(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-1": {SessionID: "session-1", TaskEnvironmentID: "env-from-db"},
	}}
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", TaskEnvironmentID: "env-in-memory",
	}))

	got, err := mgr.ResolveTaskEnvironmentID(context.Background(), "session-1")

	require.NoError(t, err)
	require.Equal(t, "env-in-memory", got)
}

func TestResolveTaskEnvironmentIDErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty session id", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.ResolveTaskEnvironmentID(ctx, "")
		require.ErrorContains(t, err, "session_id is required")
	})

	t.Run("execution without environment", func(t *testing.T) {
		mgr := newTestManager(t)
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-1", SessionID: "session-1"}))
		_, err := mgr.ResolveTaskEnvironmentID(ctx, "session-1")
		require.ErrorContains(t, err, "has no task environment ID")
	})

	t.Run("no provider configured", func(t *testing.T) {
		mgr := newTestManager(t)
		_, err := mgr.ResolveTaskEnvironmentID(ctx, "session-1")
		require.ErrorContains(t, err, "workspace info provider not configured")
	})

	t.Run("provider returns nil info", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{}}
		_, err := mgr.ResolveTaskEnvironmentID(ctx, "session-1")
		require.ErrorContains(t, err, "has no task environment ID")
	})

	t.Run("provider falls back for missing execution", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1", TaskEnvironmentID: "env-from-db"},
		}}
		got, err := mgr.ResolveTaskEnvironmentID(ctx, "session-1")
		require.NoError(t, err)
		require.Equal(t, "env-from-db", got)
	})
}

// TestIsAgentRunningForSessionStartingGrace pins the bounded boot window: a
// STARTING execution is reported live without probing agentctl, but only until
// the grace expires.
func TestIsAgentRunningForSessionStartingGrace(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-fresh", SessionID: "session-fresh",
		Status: v1.AgentStatusStarting, StartedAt: time.Now(),
	}))
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-stale", SessionID: "session-stale",
		Status: v1.AgentStatusStarting, StartedAt: time.Now().Add(-2 * agentStartupLivenessGrace),
	}))

	require.True(t, mgr.IsAgentRunningForSession(context.Background(), "session-fresh"))
	require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-stale"),
		"a starting execution past the grace window must fall through to probing and be reaped")
	require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-absent"))
}

func TestIsAgentRunningForSessionPassthroughWithoutRunner(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-pty", SessionID: "session-pty", Status: v1.AgentStatusReady,
		PassthroughProcessID: "pty-1", StartedAt: time.Now(),
	}))

	require.False(t, mgr.IsAgentRunningForSession(context.Background(), "session-pty"),
		"without an interactive runner a passthrough process cannot be confirmed alive")
}

func TestIsAgentReadyForPromptRequiresInitializedACPSession(t *testing.T) {
	ctx := context.Background()

	t.Run("no execution", func(t *testing.T) {
		mgr := newTestManager(t)
		require.False(t, mgr.IsAgentReadyForPrompt(ctx, "session-absent"))
	})

	t.Run("not ready status", func(t *testing.T) {
		mgr := newTestManager(t)
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "exec-1", SessionID: "session-1", Status: v1.AgentStatusRunning,
		}))
		require.False(t, mgr.IsAgentReadyForPrompt(ctx, "session-1"))
	})

	t.Run("ready but session not initialized", func(t *testing.T) {
		mgr := newTestManager(t)
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "exec-1", SessionID: "session-1", Status: v1.AgentStatusReady,
			agentctl: newReadyAgentctlClient(t, newTestLogger()),
		}))
		require.False(t, mgr.IsAgentReadyForPrompt(ctx, "session-1"),
			"an uninitialized ACP session cannot take a prompt even when the status is ready")
	})

	t.Run("passthrough delegates to running check", func(t *testing.T) {
		mgr := newTestManager(t)
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "exec-pty", SessionID: "session-pty", Status: v1.AgentStatusReady,
			IsPassthrough: true, StartedAt: time.Now(),
		}))
		require.False(t, mgr.IsAgentReadyForPrompt(ctx, "session-pty"))
	})
}

func TestStopBySessionIDStopsResolvedExecution(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	stopTracker := &mockStopTracker{name: executor.NameStandalone}
	execRegistry.Register(stopTracker)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID:          "exec-stop",
		TaskID:      "task-stop",
		SessionID:   "session-stop",
		RuntimeName: executor.NameStandalone,
		Status:      v1.AgentStatusRunning,
	}))

	require.NoError(t, mgr.StopBySessionID(context.Background(), "session-stop", false))

	require.True(t, stopTracker.stopCalled, "StopBySessionID must reach the executor backend")
	require.Equal(t, "exec-stop", stopTracker.stoppedInstanceID)
	_, exists := mgr.GetExecutionBySessionID("session-stop")
	require.False(t, exists, "a stopped execution is removed from tracking")

	bus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)
	require.NotEmpty(t, bus.PublishedEvents)
	require.Equal(t, events.AgentStopped, bus.PublishedEvents[len(bus.PublishedEvents)-1].Type)
}

func TestStopBySessionIDUnknownSession(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.StopBySessionID(context.Background(), "session-absent", false)

	require.ErrorContains(t, err, `no agent running for session "session-absent"`)
}

func TestStopAgentUnknownExecution(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.StopAgent(context.Background(), "missing", false)

	require.ErrorIs(t, err, ErrExecutionNotFound)
}

// TestStopAgentForcePassesForceToBackend pins that the force flag reaches the
// executor backend rather than being swallowed by the graceful-stop branch.
func TestStopAgentForcePassesForceToBackend(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	stopTracker := &forceRecordingStopTracker{mockStopTracker: mockStopTracker{name: executor.NameStandalone}}
	execRegistry.Register(stopTracker)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-force", SessionID: "session-force", RuntimeName: executor.NameStandalone,
	}))

	require.NoError(t, mgr.StopAgentWithReason(context.Background(), "exec-force", StopReasonBackendShutdown, true))

	require.True(t, stopTracker.forced, "force must be forwarded to StopInstance")
	require.Equal(t, StopReasonBackendShutdown, stopTracker.stopReason)
}

func TestStopAgentWithReasonRetainsExecutionWhenBackendCleanupFails(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	cleanupErr := errors.New("exact Kubernetes cleanup failed")
	stopTracker := &mockStopTracker{name: executor.NameKubernetes, stopErr: cleanupErr}
	execRegistry.Register(stopTracker)
	bus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), bus, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	runtimeSecrets := newInMemorySecretStore()
	for id := range map[string]struct{}{"legacy-auth-ref": {}, "legacy-nonce-ref": {}} {
		require.NoError(t, runtimeSecrets.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: "secret",
		}))
	}
	mgr.SetSecretStore(runtimeSecrets)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-k8s", TaskID: "task-k8s", SessionID: "session-k8s",
		RuntimeName: executor.NameKubernetes, Status: v1.AgentStatusRunning,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "legacy-auth-ref",
			MetadataKeyBootstrapNonceSecret: "legacy-nonce-ref",
		},
	}))

	err := mgr.StopAgentWithReason(context.Background(), "exec-k8s", StopReasonTaskDeleted, true)

	require.ErrorIs(t, err, cleanupErr)
	require.Equal(t, "session-k8s", stopTracker.stoppedSessionID)
	execution, exists := mgr.GetExecution("exec-k8s")
	require.True(t, exists, "failed exact cleanup must remain retryable")
	require.Equal(t, v1.AgentStatusRunning, execution.Status)
	require.Len(t, runtimeSecrets.store, 2, "runtime cleanup failure must preserve recovery secrets")
	for _, event := range bus.PublishedEvents {
		require.NotEqual(t, events.AgentStopped, event.Type)
	}
}

func TestStopAgentWithReasonCleansPersistedKubernetesInventoryAfterRestart(t *testing.T) {
	testPersistedKubernetesCleanupAfterRestart(t, StopReasonTaskDeleted, true)
}

func TestForceStopAgentCleansPersistedKubernetesInventoryAfterRestartWithoutTerminalReason(t *testing.T) {
	testPersistedKubernetesCleanupAfterRestart(t, "", true)
}

func testPersistedKubernetesCleanupAfterRestart(t *testing.T, reason string, force bool) {
	t.Helper()
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	instance, err := initial.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	metadata := cloneKubernetesMetadata(req.Metadata)
	for key, value := range instance.Metadata {
		metadata[key] = value
	}
	metadata[MetadataKeyKubernetesAuthMode] = "kubeconfig"
	metadata[MetadataKeyKubernetesKubeconfigPath] = "/stale/kubeconfig"
	metadata[MetadataKeyKubernetesKubeContext] = "stale-context"
	metadata[MetadataKeyKubernetesRequestTimeoutSeconds] = "10"
	runtimeSecrets := newInMemorySecretStore()
	for id, value := range map[string]string{
		"legacy-auth-ref": "agentctl-token", "legacy-nonce-ref": "bootstrap-nonce",
	} {
		require.NoError(t, runtimeSecrets.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	metadata[MetadataKeyAuthTokenSecret] = "legacy-auth-ref"
	metadata[MetadataKeyBootstrapNonceSecret] = "legacy-nonce-ref"
	row := &models.ExecutorRunning{
		ID: instance.SessionID, SessionID: instance.SessionID, TaskID: instance.TaskID,
		ExecutorID: "executor-1", ExecutionProfileID: "agent-profile-1", AgentExecutionID: instance.InstanceID,
		Runtime: agentruntime.RuntimeKubernetes, Metadata: metadata,
	}
	store := &restartKubernetesInventoryStore{
		captureExecutorRunningWriter: captureExecutorRunningWriter{prior: row},
		rows:                         []*models.ExecutorRunning{row},
		executors: map[string]*models.Executor{"executor-1": {
			ID: "executor-1", Type: models.ExecutorTypeKubernetes,
			Config: map[string]string{
				MetadataKeyKubernetesAuthMode:              "in_cluster",
				MetadataKeyKubernetesConfigNamespace:       "new-agents",
				MetadataKeyKubernetesRequestTimeoutSeconds: "45",
			},
		}},
	}
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	originalFactory := restarted.clientFactory
	var cleanupConfig kubeexecutor.ExecutorConfig
	restarted.clientFactory = func(config kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		cleanupConfig = config
		return originalFactory(config)
	}
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(restarted)
	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)
	mgr.SetSecretStore(runtimeSecrets)

	err = mgr.StopAgentWithReason(context.Background(), instance.InstanceID, reason, force)

	require.NoError(t, err)
	require.Equal(t, []string{"pod", "pvc"}, resources.deletionOrder)
	require.Equal(t, kubeexecutor.AuthModeInCluster, cleanupConfig.AuthMode)
	require.Empty(t, cleanupConfig.KubeconfigPath)
	require.Empty(t, cleanupConfig.KubeContext)
	require.Equal(t, 45, cleanupConfig.RequestTimeoutSeconds)
	require.Equal(t, "new-agents", cleanupConfig.Namespace)
	require.Nil(t, store.running, "service cleanup owns row deletion after successful remote teardown")
	require.Empty(t, runtimeSecrets.store, "destructive restart cleanup must remove recorded runtime secret refs")
}

func TestStopAgentWithReasonPreservesPersistedKubernetesInventoryForBackendShutdown(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	stopTracker := &mockStopTracker{name: executor.NameKubernetes}
	registry.Register(stopTracker)
	store := &restartKubernetesInventoryStore{rows: []*models.ExecutorRunning{{
		AgentExecutionID: "instance-1", Runtime: agentruntime.RuntimeKubernetes,
	}}}
	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)

	err := mgr.StopAgentWithReason(context.Background(), "instance-1", StopReasonBackendShutdown, false)

	require.ErrorIs(t, err, ErrExecutionNotFound)
	require.False(t, stopTracker.stopCalled)
}

type restartKubernetesInventoryStore struct {
	captureExecutorRunningWriter
	rows      []*models.ExecutorRunning
	executors map[string]*models.Executor
}

func (s *restartKubernetesInventoryStore) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return s.rows, nil
}

func (s *restartKubernetesInventoryStore) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	executor := s.executors[id]
	if executor == nil {
		return nil, models.ErrExecutorNotFound
	}
	return executor, nil
}

func TestStopAgentWithReason_BackendFailureKeepsExecutionRetryable(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &retryableStopBackend{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		stopErr:      errors.New("runtime stop failed"),
	}
	execRegistry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	coordinator := activity.NewCoordinator(activity.Options{})
	mgr.SetActivityCoordinator(coordinator)
	runningLease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionRunning)
	require.NoError(t, err)
	mgr.trackActivity(executionActivityKey("exec-retryable-stop"), runningLease)

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID:          "exec-retryable-stop",
		TaskID:      "task-retryable-stop",
		SessionID:   "session-retryable-stop",
		RuntimeName: executor.NameStandalone,
		Status:      v1.AgentStatusRunning,
	}))

	err = mgr.StopAgentWithReason(context.Background(), "exec-retryable-stop", "idle cleanup", false)
	require.ErrorIs(t, err, backend.stopErr)
	_, exists := mgr.executionStore.Get("exec-retryable-stop")
	require.True(t, exists, "a failed runtime stop must retain the execution for retry")

	bus := mgr.eventBus.(*MockEventBus)
	require.Empty(t, bus.PublishedEvents, "a failed runtime stop must not publish agent.stopped")
	_, _, err = coordinator.TryAcquireMaintenance(context.Background(), 0)
	require.ErrorIs(t, err, activity.ErrBusy, "a failed stop must retain execution activity")

	backend.stopErr = nil
	require.NoError(t, mgr.StopAgentWithReason(context.Background(), "exec-retryable-stop", "idle cleanup retry", false))
	_, exists = mgr.executionStore.Get("exec-retryable-stop")
	require.False(t, exists)
	require.Len(t, bus.PublishedEvents, 1)
	require.Equal(t, events.AgentStopped, bus.PublishedEvents[0].Type)
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	require.NoError(t, err)
	maintenance.Release()
}

type retryableStopBackend struct {
	MockExecutor
	stopErr error
}

func (b *retryableStopBackend) StopInstance(context.Context, *ExecutorInstance, bool) error {
	return b.stopErr
}

type forceRecordingStopTracker struct {
	mockStopTracker
	forced bool
}

func (m *forceRecordingStopTracker) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	m.forced = force
	return m.mockStopTracker.StopInstance(ctx, instance, force)
}

func TestSteerAgentWithDispatchCallbackUnknownExecution(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.SteerAgentWithDispatchCallback(context.Background(), "missing", "hi", nil, false, nil)

	require.ErrorIs(t, err, ErrExecutionNotFound)
}

func TestUpdateStatusUnknownExecution(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.UpdateStatus("missing", v1.AgentStatusReady)

	require.ErrorContains(t, err, `execution "missing" not found`)
}

func TestUpdateStatusTransitionsExecution(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1", Status: v1.AgentStatusStarting}
	require.NoError(t, mgr.executionStore.Add(exec))

	require.NoError(t, mgr.UpdateStatus("exec-1", v1.AgentStatusRunning))

	require.Equal(t, v1.AgentStatusRunning, exec.Status)
}

func TestMarkBootReadyFromFailedOnlyPromotesFailedExecutions(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-ready", SessionID: "session-ready", Status: v1.AgentStatusReady,
	}))
	failed := &AgentExecution{ID: "exec-failed", SessionID: "session-failed", Status: v1.AgentStatusFailed}
	require.NoError(t, mgr.executionStore.Add(failed))

	require.NoError(t, mgr.markBootReadyFromFailed(context.Background(), "exec-ready"),
		"a non-failed execution is a no-op, not an error")
	require.ErrorContains(t, mgr.markBootReadyFromFailed(context.Background(), "missing"),
		`execution "missing" not found`)

	require.NoError(t, mgr.markBootReadyFromFailed(context.Background(), "exec-failed"))
	require.Equal(t, v1.AgentStatusReady, failed.Status)

	bus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)
	require.Len(t, bus.PublishedEvents, 1, "only the failed execution publishes a boot-ready event")
	require.Equal(t, events.AgentBootReady, bus.PublishedEvents[0].Type)
}

func TestGetSessionAuthMethodsFallsBackByAgentID(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-claude", SessionID: "session-claude", AgentID: "claude-acp",
	}))
	cached := &AgentExecution{ID: "exec-cached", SessionID: "session-cached", AgentID: "claude-acp"}
	cached.SetAuthMethods([]streams.AuthMethodInfo{{ID: "cached-method"}})
	require.NoError(t, mgr.executionStore.Add(cached))

	fallback := mgr.GetSessionAuthMethods("session-claude")
	require.Len(t, fallback, 1)
	require.Equal(t, "claude-auth-login", fallback[0].ID)
	require.NotNil(t, fallback[0].TerminalAuth)
	require.Equal(t, "claude", fallback[0].TerminalAuth.Command)

	live := mgr.GetSessionAuthMethods("session-cached")
	require.Len(t, live, 1)
	require.Equal(t, "cached-method", live[0].ID,
		"reported capabilities win over the static fallback")

	require.Nil(t, mgr.GetSessionAuthMethods("session-absent"))
}
