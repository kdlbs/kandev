package lifecycle

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	instancepkg "github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// retainedBuiltinAdoptionBackend models the standalone executor returning the
// retained peer after the workspace-first restore path captured its binding.
type retainedBuiltinAdoptionBackend struct {
	MockExecutor

	mu          sync.Mutex
	client      *agentctl.Client
	lastRequest *ExecutorCreateRequest
	stopCount   atomic.Int32
}

func (e *retainedBuiltinAdoptionBackend) CreateInstance(_ context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	e.mu.Lock()
	e.lastRequest = req
	e.mu.Unlock()
	metadata := map[string]interface{}{MetadataKeyReuseExistingProcess: true}
	if req.RetainedBuiltinBinding != nil {
		metadata[p.BuiltinBindingMetadataKey] = req.RetainedBuiltinBinding
	}
	if req.RetainedResourceCircuitIdentity != nil {
		metadata[MetadataKeyResourceCircuitIdentity] = req.RetainedResourceCircuitIdentity
	}
	return &ExecutorInstance{
		InstanceID: "retained-execution", TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: e.Name(), Client: e.client,
		StandaloneInstanceID: "retained-execution", StandalonePort: 41001,
		WorkspacePath: req.WorkspacePath, ReusedWorkspace: true,
		Metadata:             metadata,
		Env:                  map[string]string{},
		WorkspaceSourceRoots: []string{},
	}, nil
}

func (e *retainedBuiltinAdoptionBackend) StopInstance(context.Context, *ExecutorInstance, bool) error {
	e.stopCount.Add(1)
	return nil
}

func newRetainedBuiltinTestManager(t *testing.T, backend ExecutorBackend) *Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBusWithTracking{}, registry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.dataDir = t.TempDir()
	mgr.SetBuiltinActionResolver(builtinTestActions)
	mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
	mgr.SetAgentDeliveryRepository(durableAdoptionRepository())
	cleanupManagerStopCh(t, mgr)
	return mgr
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
func TestStandaloneRetainedLaunchLoadsPersistedBindingWithoutPreviousExecutionID(t *testing.T) {
	bound := builtinDispatchProfile(p.PackageCore)
	encoded, err := p.MarshalBuiltinBinding(bound)
	require.NoError(t, err)
	backend := &retainedBuiltinAdoptionBackend{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr := newRetainedBuiltinTestManager(t, backend)
	writer := &captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "session-a", SessionID: "session-a", TaskID: "task-a",
		AgentExecutionID: "retained-execution", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true,
		Metadata: map[string]interface{}{p.BuiltinBindingMetadataKey: encoded},
	}}
	mgr.SetExecutorRunningWriter(writer)

	_, err = mgr.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace",
		AgentProfileID: "profile-1", IsEphemeral: true, McpProfile: &bound,
	})
	require.NoError(t, err)
	backend.mu.Lock()
	request := backend.lastRequest
	backend.mu.Unlock()
	require.NotNil(t, request)
	require.Equal(t, encoded, request.RetainedBuiltinBinding,
		"same-session reconciliation must load the persisted binding without a predecessor ID")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
func TestStandaloneRetainedLaunchDoesNotSubstituteCurrentPolicyBinding(t *testing.T) {
	bound := builtinDispatchProfile(p.PackageCore)
	backend := &retainedBuiltinAdoptionBackend{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr := newRetainedBuiltinTestManager(t, backend)
	writer := &captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "session-a", SessionID: "session-a", TaskID: "task-a",
		AgentExecutionID: "retained-execution", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true,
	}}
	mgr.SetExecutorRunningWriter(writer)

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace",
		AgentProfileID: "profile-1", IsEphemeral: true, McpProfile: &bound,
	})
	require.NoError(t, err)
	backend.mu.Lock()
	request := backend.lastRequest
	backend.mu.Unlock()
	require.NotNil(t, request)
	require.Nil(t, request.RetainedBuiltinBinding,
		"a retained launch must not register current policy as the peer's persisted binding")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
func TestStandaloneRetainedLifecycleStartupAdoptsResolverOwner(t *testing.T) {
	root := t.TempDir()
	location, err := journal.ResolveLocation(root, "session-a")
	require.NoError(t, err)
	peer := newMockAgentServer(t)
	bound := builtinDispatchProfile(p.PackageCore)
	encoded, err := p.MarshalBuiltinBinding(bound)
	require.NoError(t, err)
	peer.builtinBinding = &bound
	peer.taskSessionID = "session-a"
	peer.nativeSessionID = "native-1"
	delivery := durableAdoptionStatus()
	delivery.SessionID = "session-a"
	delivery.Stream.SessionID = "session-a"
	delivery.Stream.HighWater = 2
	delivery.Stream.FirstRetained = 1
	delivery.Stream.Acknowledged = 2
	peer.deliveryStatus = delivery
	t.Cleanup(peer.Close)

	instances, _ := newRealInstanceManager(t, 4)
	instances.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return peer.server.Config.Handler
	})
	live, err := instances.CreateInstance(context.Background(), &instancepkg.CreateRequest{
		ID: "retained-execution", WorkspacePath: root, SessionID: "session-a", TaskID: "task-a",
		DurableJournalPath: location.Path, DeliveryStreamID: "stream-1",
		DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 2,
	})
	require.NoError(t, err)
	peer.instanceID = live.ID

	control := newRealStandaloneControl(instances)
	controlServer := newRealStandaloneControlServer(t, control)
	host, port := parseHTTPURL(t, controlServer.URL)
	runtime := NewStandaloneExecutor(agentctl.NewControlClient(host, port, newTestLogger()), host, port, newTestLogger())
	mgr := newRetainedBuiltinTestManager(t, runtime)
	mgr.dataDir = root
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "session-a", SessionID: "session-a", TaskID: "task-a",
		AgentExecutionID: "retained-execution", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true,
	}})
	repository := durableAdoptionRepository()
	repository.generation.SessionID = "session-a"
	repository.cursor.SessionID = "session-a"
	mgr.SetAgentDeliveryRepository(repository)
	info := &WorkspaceInfo{
		TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace",
		AgentID: "auggie", ExecutionProfileID: "profile-1", ExecutorType: "local",
		WorkspacePath: root, AgentExecutionID: "retained-execution", ACPSessionID: "native-1",
		DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 2,
		DeliveryStreamID: "stream-1",
		Metadata:         map[string]interface{}{p.BuiltinBindingMetadataKey: encoded},
	}
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{"session-a": info}}

	execution, err := mgr.GetOrEnsureExecution(context.Background(), "session-a")
	require.NoError(t, err)
	require.Equal(t, "retained-execution", execution.ID)
	require.Equal(t, "retained-execution", execution.standaloneInstanceID)

	_, err = mgr.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace",
		AgentProfileID: "profile-1", McpProfile: &bound,
	})
	require.NoError(t, err)
	require.NoError(t, mgr.StartAgentProcess(context.Background(), execution.ID))

	peer.mu.Lock()
	builtinRequests := append([]string(nil), peer.builtinRequests...)
	peer.mu.Unlock()
	require.NotEmpty(t, builtinRequests)
	for _, method := range builtinRequests {
		require.Equal(t, "GET", method, "lifecycle startup must verify the retained binding only")
	}
	if got := peer.getActionLog(); len(got) != 0 {
		t.Fatalf("startup dispatched ACP actions on the retained peer: %v", got)
	}
	actual, boundNow := execution.metadataValue(p.BuiltinBindingMetadataKey)
	require.True(t, boundNow)
	require.Equal(t, encoded, actual)
	require.True(t, execution.isSessionInitialized())
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
func TestStandaloneRetainedWorkspaceRestoreKeepsBuiltinBinding(t *testing.T) {
	peer := newMockAgentServer(t)
	bound := builtinDispatchProfile(p.PackageCore)
	bound.BuiltinPackages.WorkspaceID = "workspace"
	bound.BuiltinPackages.TaskID = "task-a"
	encoded, err := p.MarshalBuiltinBinding(bound)
	require.NoError(t, err)
	peer.builtinBinding = &bound
	peer.instanceID = "retained-execution"
	peer.taskSessionID = "session-a"
	peer.nativeSessionID = "native-1"
	delivery := durableAdoptionStatus()
	delivery.SessionID = "session-a"
	delivery.IncarnationID = "incarnation-1"
	delivery.StreamID = "stream-1"
	delivery.HarnessGeneration = 2
	delivery.Stream.SessionID = "session-a"
	delivery.Stream.IncarnationID = "incarnation-1"
	delivery.Stream.StreamID = "stream-1"
	delivery.Stream.HarnessGeneration = 2
	delivery.Stream.HighWater = 2
	delivery.Stream.FirstRetained = 1
	delivery.Stream.Acknowledged = 2
	peer.deliveryStatus = delivery
	t.Cleanup(peer.Close)

	backend := &retainedBuiltinAdoptionBackend{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       createTestClient(t, peer.server.URL),
	}
	mgr := newRetainedBuiltinTestManager(t, backend)
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		ID: "session-a", SessionID: "session-a", TaskID: "task-a",
		AgentExecutionID: "retained-execution", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true,
	}})
	repository := durableAdoptionRepository()
	repository.generation.SessionID = "session-a"
	repository.cursor.SessionID = "session-a"
	mgr.SetAgentDeliveryRepository(repository)
	info := &WorkspaceInfo{
		TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace",
		AgentID: "auggie", ExecutionProfileID: "profile-1", ExecutorType: "local",
		WorkspacePath: t.TempDir(), AgentExecutionID: "retained-execution", ACPSessionID: "native-1",
		DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 2,
		DeliveryStreamID: "stream-1",
		Metadata: map[string]interface{}{
			p.BuiltinBindingMetadataKey: encoded,
			MetadataKeyResourceCircuitIdentity: resourceCircuitIdentity{
				ExecutionID: "retained-execution", ProfileID: "profile-1", Key: "retained-resource",
			},
		},
	}
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{"session-a": info}}

	execution, err := mgr.GetOrEnsureExecution(context.Background(), "session-a")
	require.NoError(t, err)
	require.NotNil(t, execution)
	backend.mu.Lock()
	request := backend.lastRequest
	backend.mu.Unlock()
	require.NotNil(t, request)
	require.Equal(t, encoded, request.RetainedBuiltinBinding,
		"workspace restore must capture the persisted binding before fresh-helper metadata strips it")
	require.NotContains(t, request.Metadata, p.BuiltinBindingMetadataKey)
	actual, boundNow := execution.metadataValue(p.BuiltinBindingMetadataKey)
	require.True(t, boundNow)
	require.Equal(t, encoded, actual, "adoption must restore the exact persisted binding")
	require.Equal(t, "retained-resource", execution.resourceCircuitKey(),
		"adoption must restore the retained resource identity without current-policy substitution")

	_, err = mgr.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace",
		AgentProfileID: "profile-1", McpProfile: &bound,
	})
	require.NoError(t, err)
	require.NoError(t, mgr.StartAgentProcess(context.Background(), execution.ID))

	peer.mu.Lock()
	builtinRequests := append([]string(nil), peer.builtinRequests...)
	peer.mu.Unlock()
	require.NotEmpty(t, builtinRequests)
	for _, method := range builtinRequests {
		require.Equal(t, "GET", method, "retained adoption must verify, never prepare or rebind")
	}
	if got := peer.getActionLog(); len(got) != 0 {
		t.Fatalf("ACP lifecycle actions on retained peer = %v, want none", got)
	}
	actual, boundNow = execution.metadataValue(p.BuiltinBindingMetadataKey)
	require.True(t, boundNow)
	require.Equal(t, encoded, actual)
	require.Zero(t, backend.stopCount.Load())
	require.NotNil(t, backend.lastRequest)
	require.NotEmpty(t, backend.lastRequest.InstanceID)
	require.NotEqual(t, execution.ID, backend.lastRequest.InstanceID)
}
