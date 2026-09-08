package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// statusProviderExecutor is an ExecutorBackend that also implements
// RemoteStatusProvider, recording the instance the manager projected from the
// execution so the projection itself can be asserted.
type statusProviderExecutor struct {
	MockExecutor
	status   *RemoteStatus
	err      error
	observed []*ExecutorInstance
}

type refreshingStatusExecutor struct {
	statusProviderExecutor
	refresh        *RemoteInstanceRefresh
	refreshEntered chan struct{}
	releaseRefresh chan struct{}
	once           sync.Once
	mu             sync.Mutex
	refreshCalls   int
	committed      bool
	stopEntered    chan struct{}
	stopOnce       sync.Once
	onStop         func()
}

func (e *refreshingStatusExecutor) RefreshRemoteInstance(
	_ context.Context,
	_ *ExecutorInstance,
) (*RemoteInstanceRefresh, error) {
	e.mu.Lock()
	if e.committed {
		e.mu.Unlock()
		return nil, nil
	}
	e.refreshCalls++
	e.mu.Unlock()
	e.once.Do(func() { close(e.refreshEntered) })
	<-e.releaseRefresh
	return e.refresh, nil
}

func (e *refreshingStatusExecutor) GetRemoteStatus(_ context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.observed = append(e.observed, instance)
	return e.status, e.err
}

func (e *refreshingStatusExecutor) StopInstance(context.Context, *ExecutorInstance, bool) error {
	if e.stopEntered != nil {
		e.stopOnce.Do(func() { close(e.stopEntered) })
	}
	if e.onStop != nil {
		e.onStop()
	}
	return nil
}

type synchronizedRunningWriter struct {
	mu      sync.Mutex
	running *models.ExecutorRunning
}

func (w *synchronizedRunningWriter) GetExecutorRunningBySessionID(
	context.Context, string,
) (*models.ExecutorRunning, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running == nil {
		return nil, models.ErrExecutorRunningNotFound
	}
	return w.running, nil
}

func (w *synchronizedRunningWriter) UpsertExecutorRunning(
	_ context.Context, running *models.ExecutorRunning,
) error {
	w.mu.Lock()
	w.running = running
	w.mu.Unlock()
	return nil
}

func (w *synchronizedRunningWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	w.mu.Lock()
	w.running = nil
	w.mu.Unlock()
	return nil
}

func (w *synchronizedRunningWriter) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

func (w *synchronizedRunningWriter) clear() {
	w.mu.Lock()
	w.running = nil
	w.mu.Unlock()
}

func (w *synchronizedRunningWriter) snapshot() *models.ExecutorRunning {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (e *statusProviderExecutor) GetRemoteStatus(_ context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	e.observed = append(e.observed, instance)
	return e.status, e.err
}

func newRemoteStatusManager(t *testing.T, provider ExecutorBackend) *Manager {
	t.Helper()
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(provider)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr
}

// TestPollOneRemoteStatusProjectsExecutionIntoInstance pins the projection the
// provider is handed: a status poll must carry the runtime identifiers the
// backend needs to find the remote (container, sprite name in metadata), not
// just the session ID.
func TestPollOneRemoteStatusProjectsExecutionIntoInstance(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		status:       &RemoteStatus{State: "started"},
	}
	mgr := newRemoteStatusManager(t, provider)
	execution := &AgentExecution{
		ID:            "exec-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		RuntimeName:   agentruntime.RuntimeSprites,
		ContainerID:   "container-1",
		ContainerIP:   "10.0.0.2",
		WorkspacePath: "/workspace",
	}
	execution.setMetadataValue(MetadataKeySpriteName, "kandev-abc")

	mgr.pollOneRemoteStatus(context.Background(), execution)

	require.Len(t, provider.observed, 1)
	instance := provider.observed[0]
	require.Equal(t, "exec-1", instance.InstanceID)
	require.Equal(t, "task-1", instance.TaskID)
	require.Equal(t, "session-1", instance.SessionID)
	require.Equal(t, agentruntime.RuntimeSprites, instance.RuntimeName)
	require.Equal(t, "container-1", instance.ContainerID)
	require.Equal(t, "10.0.0.2", instance.ContainerIP)
	require.Equal(t, "/workspace", instance.WorkspacePath)
	require.Equal(t, "kandev-abc", instance.Metadata[MetadataKeySpriteName],
		"the remote's own name lives in metadata; without it the provider cannot find the sandbox")

	status, ok := mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok)
	require.Equal(t, "started", status.State)
	require.Equal(t, agentruntime.RuntimeSprites, status.RuntimeName,
		"a provider that omits the runtime name gets the execution's filled in")
	require.False(t, status.LastCheckedAt.IsZero(),
		"an unstamped status is stamped so the UI can show staleness")
}

func TestKubernetesRefreshStatusInstanceOverlaysCurrentConnectionSettings(t *testing.T) {
	store := &restartKubernetesInventoryStore{
		executors: map[string]*models.Executor{"executor-1": {
			ID: "executor-1", Type: models.ExecutorTypeKubernetes,
			Config: map[string]string{
				MetadataKeyKubernetesAuthMode:              "kubeconfig",
				MetadataKeyKubernetesKubeconfigPath:        "/etc/kandev/current.yaml",
				MetadataKeyKubernetesKubeContext:           "current",
				MetadataKeyKubernetesConfigNamespace:       "new-agents",
				MetadataKeyKubernetesRequestTimeoutSeconds: "45",
			},
		}},
	}
	mgr := &Manager{runningWriter: store}
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes,
		metadata: map[string]interface{}{
			"executor_id":                              "executor-1",
			MetadataKeyKubernetesAuthMode:              "in_cluster",
			MetadataKeyKubernetesConfigNamespace:       "old-agents",
			MetadataKeyKubernetesNamespace:             "old-agents",
			MetadataKeyKubernetesRequestTimeoutSeconds: "30",
		},
	}

	instance, err := mgr.kubernetesRefreshStatusInstance(context.Background(), execution)

	require.NoError(t, err)
	require.Equal(t, "kubeconfig", instance.Metadata[MetadataKeyKubernetesAuthMode])
	require.Equal(t, "/etc/kandev/current.yaml", instance.Metadata[MetadataKeyKubernetesKubeconfigPath])
	require.Equal(t, "current", instance.Metadata[MetadataKeyKubernetesKubeContext])
	require.Equal(t, "new-agents", instance.Metadata[MetadataKeyKubernetesConfigNamespace])
	require.Equal(t, "old-agents", instance.Metadata[MetadataKeyKubernetesNamespace])
}

func TestPollOneRemoteStatusStoresProviderError(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		err:          errors.New("sprite unreachable"),
	}
	mgr := newRemoteStatusManager(t, provider)

	mgr.pollOneRemoteStatus(context.Background(), &AgentExecution{
		ID: "exec-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeSprites,
	})

	status, ok := mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok, "a failed poll must still record a status so the UI can show degraded")
	require.Equal(t, "sprite unreachable", status.ErrorMessage)
	require.Equal(t, agentruntime.RuntimeSprites, status.RuntimeName)
}

func TestPollOneRemoteStatusAtomicallyRefreshesRestartedKubernetesAgentctl(t *testing.T) {
	var configureCalls atomic.Int32
	var startCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/agent/configure":
			configureCalls.Add(1)
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`{"success":true,"command":"agent"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	newClient := newTestAgentctlClient(t, server.URL, newTestLogger())
	oldClient := newReadyAgentctlClient(t, newTestLogger())

	refresh := &RemoteInstanceRefresh{Instance: &ExecutorInstance{
		InstanceID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes, Client: newClient,
		AuthToken: "rotated-token", BootstrapNonce: "durable-nonce",
		Metadata: map[string]interface{}{
			MetadataKeyKubernetesAgentctlRemotePort: "41002",
		},
	}, ProcessRestarted: true}
	provider := &refreshingStatusExecutor{
		statusProviderExecutor: statusProviderExecutor{
			MockExecutor: MockExecutor{name: executor.NameKubernetes},
			status:       &RemoteStatus{State: kubernetesStatusRunning},
		},
		refresh: refresh, refreshEntered: make(chan struct{}), releaseRefresh: make(chan struct{}),
	}
	refresh.Commit = func(publish func()) error {
		provider.mu.Lock()
		provider.committed = true
		provider.mu.Unlock()
		if publish != nil {
			publish()
		}
		return nil
	}
	refresh.Abort = func() { t.Error("successful refresh was aborted") }
	mgr := newRemoteStatusManager(t, provider)
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:exec-1:agentctl-auth":      "old-token",
		"kandev-runtime:exec-1:agentctl-bootstrap": "durable-nonce",
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	mgr.SetSecretStore(store)
	writer := &captureExecutorRunningWriter{}
	mgr.SetExecutorRunningWriter(writer)
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusRunning, AgentCommand: "agent", agentctl: oldClient,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:              "kandev-runtime:exec-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret:         "kandev-runtime:exec-1:agentctl-bootstrap",
			MetadataKeyKubernetesAgentctlRemotePort: "41001",
		},
	}
	execution.setRuntimeEnvironment(map[string]string{"FOO": "bar"})
	require.NoError(t, mgr.executionStore.Add(execution))

	firstDone := make(chan struct{})
	go func() {
		mgr.pollOneRemoteStatus(context.Background(), execution)
		close(firstDone)
	}()
	<-provider.refreshEntered
	if execution.remoteInstanceLifecycleMu.TryLock() {
		execution.remoteInstanceLifecycleMu.Unlock()
		close(provider.releaseRefresh)
		<-firstDone
		t.Fatal("active remote refresh did not own the execution lifecycle boundary")
	}
	secondDone := make(chan struct{})
	go func() {
		mgr.pollOneRemoteStatus(context.Background(), execution)
		close(secondDone)
	}()
	close(provider.releaseRefresh)
	<-firstDone
	<-secondDone

	provider.mu.Lock()
	refreshCalls := provider.refreshCalls
	committed := provider.committed
	provider.mu.Unlock()
	require.Equal(t, 1, refreshCalls, "concurrent polls must share one refresh")
	require.True(t, committed)
	client, releaseClient := execution.AcquireAgentCtlClient()
	require.Same(t, newClient, client)
	releaseClient()
	require.Equal(t, int32(1), configureCalls.Load())
	require.Equal(t, int32(1), startCalls.Load())
	revealed, err := store.Reveal(context.Background(), "kandev-runtime:exec-1:agentctl-auth")
	require.NoError(t, err)
	require.Equal(t, "rotated-token", revealed)
	store.mu.RLock()
	secretCount := len(store.store)
	store.mu.RUnlock()
	require.Equal(t, 2, secretCount, "token rotation must reuse the internal secret reference")
	require.NotNil(t, writer.running)
	require.Equal(t, "41002", getMetadataString(writer.running.Metadata, MetadataKeyKubernetesAgentctlRemotePort))
}

func TestPollOneRemoteStatusReattachesKubernetesClientWithoutRestartingAgent(t *testing.T) {
	var configureCalls atomic.Int32
	var startCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/agent/configure":
			configureCalls.Add(1)
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`{"success":true,"command":"agent"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	newClient := newTestAgentctlClient(t, server.URL, newTestLogger())
	oldClient := newReadyAgentctlClient(t, newTestLogger())
	releaseRefresh := make(chan struct{})
	close(releaseRefresh)
	refresh := &RemoteInstanceRefresh{Instance: &ExecutorInstance{
		InstanceID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes, Client: newClient,
		AuthToken: "old-token", BootstrapNonce: "durable-nonce",
		Metadata: map[string]interface{}{
			MetadataKeyKubernetesAgentctlRemotePort: "41001",
		},
	}}
	provider := &refreshingStatusExecutor{
		statusProviderExecutor: statusProviderExecutor{
			MockExecutor: MockExecutor{name: executor.NameKubernetes},
			status:       &RemoteStatus{State: kubernetesStatusRunning},
		},
		refresh: refresh, refreshEntered: make(chan struct{}), releaseRefresh: releaseRefresh,
	}
	refresh.Commit = func(publish func()) error {
		provider.mu.Lock()
		provider.committed = true
		provider.mu.Unlock()
		if publish != nil {
			publish()
		}
		return nil
	}
	refresh.Abort = func() { t.Error("successful refresh was aborted") }
	mgr := newRemoteStatusManager(t, provider)
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:exec-1:agentctl-auth":      "old-token",
		"kandev-runtime:exec-1:agentctl-bootstrap": "durable-nonce",
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	mgr.SetSecretStore(store)
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{})
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusRunning, AgentCommand: "agent", agentctl: oldClient,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:              "kandev-runtime:exec-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret:         "kandev-runtime:exec-1:agentctl-bootstrap",
			MetadataKeyKubernetesAgentctlRemotePort: "41001",
		},
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	mgr.pollOneRemoteStatus(context.Background(), execution)

	client, releaseClient := execution.AcquireAgentCtlClient()
	require.Same(t, newClient, client)
	releaseClient()
	require.Zero(t, configureCalls.Load(), "local transport reattachment must not restart the remote agent")
	require.Zero(t, startCalls.Load(), "local transport reattachment must not restart the remote agent")
}

func TestPersistActiveKubernetesRefreshOutlivesCanceledPollContext(t *testing.T) {
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:exec-1:agentctl-auth":      "old-token",
		"kandev-runtime:exec-1:agentctl-bootstrap": "durable-nonce",
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	store.rejectCanceledCalls = true
	mgr := newRemoteStatusManager(t, &MockExecutor{name: executor.NameKubernetes})
	mgr.SetSecretStore(store)
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{})
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusRunning,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "kandev-runtime:exec-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret: "kandev-runtime:exec-1:agentctl-bootstrap",
		},
	}
	instance := &ExecutorInstance{
		AuthToken: "rotated-token", BootstrapNonce: "durable-nonce",
		Metadata: map[string]interface{}{MetadataKeyKubernetesAgentctlRemotePort: "41002"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rollback, err := mgr.persistActiveKubernetesRefresh(ctx, execution, instance)
	require.NoError(t, err)
	require.NotNil(t, rollback)
	rotated, err := store.Reveal(context.Background(), "kandev-runtime:exec-1:agentctl-auth")
	require.NoError(t, err)
	require.Equal(t, "rotated-token", rotated)
	require.NoError(t, rollback(ctx))
	restored, err := store.Reveal(context.Background(), "kandev-runtime:exec-1:agentctl-auth")
	require.NoError(t, err)
	require.Equal(t, "old-token", restored)
}

func TestStopAgentSerializesWithActiveKubernetesRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	newClient := newTestAgentctlClient(t, server.URL, newTestLogger())
	oldClient := newReadyAgentctlClient(t, newTestLogger())
	writer := &synchronizedRunningWriter{}
	refresh := &RemoteInstanceRefresh{Instance: &ExecutorInstance{
		InstanceID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes, Client: newClient,
		AuthToken: "old-token", BootstrapNonce: "durable-nonce",
		Metadata: map[string]interface{}{
			MetadataKeyKubernetesAgentctlRemotePort: "41001",
		},
	}}
	provider := &refreshingStatusExecutor{
		statusProviderExecutor: statusProviderExecutor{
			MockExecutor: MockExecutor{name: executor.NameKubernetes},
			status:       &RemoteStatus{State: kubernetesStatusRunning},
		},
		refresh: refresh, refreshEntered: make(chan struct{}), releaseRefresh: make(chan struct{}),
		stopEntered: make(chan struct{}), onStop: writer.clear,
	}
	refresh.Commit = func(publish func()) error {
		provider.mu.Lock()
		provider.committed = true
		provider.mu.Unlock()
		if publish != nil {
			publish()
		}
		return nil
	}
	refresh.Abort = func() { t.Error("successful refresh was aborted") }
	mgr := newRemoteStatusManager(t, provider)
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:exec-1:agentctl-auth":      "old-token",
		"kandev-runtime:exec-1:agentctl-bootstrap": "durable-nonce",
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	mgr.SetSecretStore(store)
	mgr.SetExecutorRunningWriter(writer)
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusRunning, AgentCommand: "agent", agentctl: oldClient,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:              "kandev-runtime:exec-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret:         "kandev-runtime:exec-1:agentctl-bootstrap",
			MetadataKeyKubernetesAgentctlRemotePort: "41001",
		},
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	refreshDone := make(chan struct{})
	go func() {
		mgr.pollOneRemoteStatus(context.Background(), execution)
		close(refreshDone)
	}()
	<-provider.refreshEntered
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonTaskDeleted, true)
	}()
	close(provider.releaseRefresh)
	<-refreshDone
	require.NoError(t, <-stopDone)

	require.Nil(t, writer.snapshot(), "terminal stop must remain the final persistence owner")
	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists)
	store.mu.RLock()
	defer store.mu.RUnlock()
	require.Empty(t, store.store)
}

func TestPollOneRemoteStatusSkipsUnsupportedExecutions(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		status:       &RemoteStatus{State: "started"},
	}
	mgr := newRemoteStatusManager(t, provider)
	ctx := context.Background()

	mgr.pollOneRemoteStatus(ctx, nil)
	mgr.pollOneRemoteStatus(ctx, &AgentExecution{ID: "exec-1"}) // no session
	mgr.pollOneRemoteStatus(ctx, &AgentExecution{
		ID: "exec-2", SessionID: "session-2", RuntimeName: agentruntime.RuntimeDocker,
	}) // runtime not registered

	require.Empty(t, provider.observed)
	_, ok := mgr.GetRemoteStatusBySession("session-2")
	require.False(t, ok)
}

func TestPollOneRemoteStatusSkipsBackendWithoutStatusSupport(t *testing.T) {
	mgr := newRemoteStatusManager(t, &MockExecutor{name: executor.NameStandalone})

	mgr.pollOneRemoteStatus(context.Background(), &AgentExecution{
		ID: "exec-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeStandalone,
	})

	_, ok := mgr.GetRemoteStatusBySession("session-1")
	require.False(t, ok, "a local executor has no remote status to report")
}

func TestPollOneRemoteStatusIgnoresNilProviderResult(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameSprites}}
	mgr := newRemoteStatusManager(t, provider)

	mgr.pollOneRemoteStatus(context.Background(), &AgentExecution{
		ID: "exec-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeSprites,
	})

	_, ok := mgr.GetRemoteStatusBySession("session-1")
	require.False(t, ok, "a provider returning (nil, nil) must not create an empty cache entry")
}

func TestPollRemoteStatusesCoversEveryTrackedExecution(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		status:       &RemoteStatus{State: "started"},
	}
	mgr := newRemoteStatusManager(t, provider)
	for _, id := range []string{"session-1", "session-2"} {
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "exec-" + id, SessionID: id, RuntimeName: agentruntime.RuntimeSprites,
		}))
	}

	mgr.pollRemoteStatuses(context.Background())

	require.Len(t, provider.observed, 2)
	for _, id := range []string{"session-1", "session-2"} {
		_, ok := mgr.GetRemoteStatusBySession(id)
		require.True(t, ok, "session %s was not polled", id)
	}
}

func TestGetRemoteStatusBySessionIDRefreshesTrackedExecution(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		status:       &RemoteStatus{State: "started", LastCheckedAt: time.Now().UTC()},
	}
	mgr := newRemoteStatusManager(t, provider)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeSprites,
	}))

	status, ok := mgr.GetRemoteStatusBySessionID(context.Background(), "session-1")

	require.True(t, ok)
	require.Equal(t, "started", status.State)
	require.Len(t, provider.observed, 1, "a tracked session is refreshed opportunistically on read")

	_, ok = mgr.GetRemoteStatusBySessionID(context.Background(), "session-absent")
	require.False(t, ok)
	require.Len(t, provider.observed, 1, "an untracked session must not trigger a poll")
}

func TestGetRemoteStatusBySessionReturnsDefensiveCopy(t *testing.T) {
	mgr := newRemoteStatusManager(t, &MockExecutor{name: executor.NameStandalone})
	mgr.storeRemoteStatus("session-1", &RemoteStatus{State: "started"})

	first, ok := mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok)
	first.State = "mutated-by-caller"

	second, ok := mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok)
	require.Equal(t, "started", second.State, "the cache must hand out copies, not its own entry")
}

func TestStoreAndClearRemoteStatusIgnoreEmptyInputs(t *testing.T) {
	mgr := newRemoteStatusManager(t, &MockExecutor{name: executor.NameStandalone})

	mgr.storeRemoteStatus("", &RemoteStatus{State: "started"})
	mgr.storeRemoteStatus("session-1", nil)
	_, ok := mgr.GetRemoteStatusBySession("session-1")
	require.False(t, ok)

	mgr.storeRemoteStatus("session-1", &RemoteStatus{State: "started"})
	mgr.clearRemoteStatus("")
	_, ok = mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok, "clearing an empty session ID must not wipe unrelated entries")

	mgr.clearRemoteStatus("session-1")
	_, ok = mgr.GetRemoteStatusBySession("session-1")
	require.False(t, ok)
}

// TestPollRemoteStatusForRecordsPopulatesCacheBeforeResume pins the startup
// path: sessions that have no in-memory execution yet still get their remote
// status cached from the persisted executor records.
func TestPollRemoteStatusForRecordsPopulatesCacheBeforeResume(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		status:       &RemoteStatus{State: "suspended"},
	}
	mgr := newRemoteStatusManager(t, provider)

	mgr.PollRemoteStatusForRecords(context.Background(), []RemoteStatusPollRecord{
		{SessionID: "", Runtime: agentruntime.RuntimeSprites},                 // skipped
		{SessionID: "session-no-runtime"},                                     // skipped
		{SessionID: "session-unregistered", Runtime: agentruntime.RuntimeSSH}, // skipped
		{
			TaskID:           "task-1",
			SessionID:        "session-1",
			Runtime:          agentruntime.RuntimeSprites,
			AgentExecutionID: "exec-1",
			ContainerID:      "container-1",
			Metadata:         map[string]interface{}{MetadataKeySpriteName: "kandev-abc"},
		},
	})

	require.Len(t, provider.observed, 1, "only the complete, registered record is polled")
	require.Equal(t, "exec-1", provider.observed[0].InstanceID)
	require.Equal(t, "task-1", provider.observed[0].TaskID)
	require.Equal(t, "container-1", provider.observed[0].ContainerID)
	require.Equal(t, "kandev-abc", provider.observed[0].Metadata[MetadataKeySpriteName])

	status, ok := mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok)
	require.Equal(t, "suspended", status.State)
	require.Equal(t, agentruntime.RuntimeSprites, status.RuntimeName)
	require.False(t, status.LastCheckedAt.IsZero())
}

func TestPollRemoteStatusForRecordsStoresProviderError(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		err:          errors.New("api token expired"),
	}
	mgr := newRemoteStatusManager(t, provider)

	mgr.PollRemoteStatusForRecords(context.Background(), []RemoteStatusPollRecord{
		{SessionID: "session-1", Runtime: agentruntime.RuntimeSprites},
	})

	status, ok := mgr.GetRemoteStatusBySession("session-1")
	require.True(t, ok)
	require.Equal(t, "api token expired", status.ErrorMessage)
}

func TestPollRemoteStatusForRecordsSkipsNilProviderResultAndNoRegistry(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameSprites}}
	mgr := newRemoteStatusManager(t, provider)

	mgr.PollRemoteStatusForRecords(context.Background(), []RemoteStatusPollRecord{
		{SessionID: "session-1", Runtime: agentruntime.RuntimeSprites},
	})
	_, ok := mgr.GetRemoteStatusBySession("session-1")
	require.False(t, ok)

	bare := newTestManager(t)
	bare.PollRemoteStatusForRecords(context.Background(), []RemoteStatusPollRecord{
		{SessionID: "session-1", Runtime: agentruntime.RuntimeSprites},
	})
	_, ok = bare.GetRemoteStatusBySession("session-1")
	require.False(t, ok, "without an executor registry there is nothing to poll")
}

// TestStopAgentClearsRemoteStatus pins that a stopped session does not keep
// reporting a stale cloud status in the UI.
func TestStopAgentClearsRemoteStatus(t *testing.T) {
	provider := &statusProviderExecutor{
		MockExecutor: MockExecutor{name: executor.NameSprites},
		status:       &RemoteStatus{State: "started"},
	}
	mgr := newRemoteStatusManager(t, provider)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeSprites,
	}))
	mgr.storeRemoteStatus("session-1", &RemoteStatus{State: "started"})

	require.NoError(t, mgr.StopAgent(context.Background(), "exec-1", false))

	_, ok := mgr.GetRemoteStatusBySession("session-1")
	require.False(t, ok)
}

func TestManagerEnvironmentRequiresTypedDockerBackend(t *testing.T) {
	ctx := context.Background()
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{name: executor.NameDocker})
	registry.Register(&MockExecutor{name: executor.NameSprites})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	require.NoError(t, mgr.DestroyContainer(ctx, ""), "an empty container ID is a no-op")
	require.ErrorContains(t, mgr.DestroyContainer(ctx, "container-1"), "docker backend has unexpected type")

	status, err := mgr.GetContainerLiveStatus(ctx, "")
	require.NoError(t, err)
	require.Nil(t, status)
	_, err = mgr.GetContainerLiveStatus(ctx, "container-1")
	require.ErrorContains(t, err, "docker backend has unexpected type")

	require.NoError(t, mgr.DestroySandbox(ctx, "", "exec-1"), "an empty sandbox ID is a no-op")
	require.ErrorContains(t, mgr.DestroySandbox(ctx, "sandbox-1", "exec-1"),
		"sprites backend has unexpected type")
}

func TestManagerEnvironmentReportsMissingBackends(t *testing.T) {
	ctx := context.Background()
	log := newTestRegistryLogger()
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, NewExecutorRegistry(log), nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	require.ErrorContains(t, mgr.DestroyContainer(ctx, "container-1"), "docker backend unavailable")
	_, err := mgr.GetContainerLiveStatus(ctx, "container-1")
	require.ErrorContains(t, err, "docker backend unavailable")
	require.ErrorContains(t, mgr.DestroySandbox(ctx, "sandbox-1", "exec-1"), "sprites backend unavailable")
}
