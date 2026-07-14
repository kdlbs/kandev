package lifecycle

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// testAgent implements agents.Agent for use in lifecycle tests.
// Embed StandardPassthrough to optionally satisfy agents.PassthroughAgent.
type testAgent struct {
	agents.StandardPassthrough
	id                 string
	name               string
	displayName        string
	description        string
	enabled            bool
	defaultModel       string
	permissionSettings map[string]agents.PermissionSetting
	runtimeConfig      *agents.RuntimeConfig
}

func (a *testAgent) ID() string          { return a.id }
func (a *testAgent) Name() string        { return a.name }
func (a *testAgent) DisplayName() string { return a.displayName }
func (a *testAgent) Description() string { return a.description }
func (a *testAgent) Enabled() bool       { return a.enabled }
func (a *testAgent) DisplayOrder() int   { return 0 }

func (a *testAgent) Logo(_ agents.LogoVariant) []byte { return nil }
func (a *testAgent) IsInstalled(_ context.Context) (*agents.DiscoveryResult, error) {
	return &agents.DiscoveryResult{Available: false}, nil
}

func (a *testAgent) BuildCommand(_ agents.CommandOptions) agents.Command { return agents.Command{} }

func (a *testAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return a.permissionSettings
}

func (a *testAgent) Runtime() *agents.RuntimeConfig {
	if a.runtimeConfig != nil {
		return a.runtimeConfig
	}
	return &agents.RuntimeConfig{
		Cmd:            agents.NewCommand(a.id),
		ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
	}
}
func (a *testAgent) BillingType() usage.BillingType { return usage.BillingTypeAPIKey }
func (a *testAgent) RemoteAuth() *agents.RemoteAuth { return nil }
func (a *testAgent) InstallScript() string          { return "" }

var (
	_ agents.Agent            = (*testAgent)(nil)
	_ agents.PassthroughAgent = (*testAgent)(nil)
)

// MockDockerClient implements a mock for the docker.Client for testing
type MockDockerClient struct {
	CreateContainerFn  func(ctx context.Context, cfg docker.ContainerConfig) (string, error)
	StartContainerFn   func(ctx context.Context, containerID string) error
	StopContainerFn    func(ctx context.Context, containerID string, timeout time.Duration) error
	KillContainerFn    func(ctx context.Context, containerID string, signal string) error
	RemoveContainerFn  func(ctx context.Context, containerID string, force bool) error
	GetContainerInfoFn func(ctx context.Context, containerID string) (*docker.ContainerInfo, error)
	GetContainerLogsFn func(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error)
	ListContainersFn   func(ctx context.Context, labels map[string]string) ([]docker.ContainerInfo, error)
}

func (m *MockDockerClient) CreateContainer(ctx context.Context, cfg docker.ContainerConfig) (string, error) {
	if m.CreateContainerFn != nil {
		return m.CreateContainerFn(ctx, cfg)
	}
	return "mock-container-id", nil
}

func (m *MockDockerClient) StartContainer(ctx context.Context, containerID string) error {
	if m.StartContainerFn != nil {
		return m.StartContainerFn(ctx, containerID)
	}
	return nil
}

func (m *MockDockerClient) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	if m.StopContainerFn != nil {
		return m.StopContainerFn(ctx, containerID, timeout)
	}
	return nil
}

func (m *MockDockerClient) KillContainer(ctx context.Context, containerID string, signal string) error {
	if m.KillContainerFn != nil {
		return m.KillContainerFn(ctx, containerID, signal)
	}
	return nil
}

func (m *MockDockerClient) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	if m.RemoveContainerFn != nil {
		return m.RemoveContainerFn(ctx, containerID, force)
	}
	return nil
}

func (m *MockDockerClient) GetContainerInfo(ctx context.Context, containerID string) (*docker.ContainerInfo, error) {
	if m.GetContainerInfoFn != nil {
		return m.GetContainerInfoFn(ctx, containerID)
	}
	return &docker.ContainerInfo{ID: containerID, State: "running"}, nil
}

func (m *MockDockerClient) GetContainerLogs(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error) {
	if m.GetContainerLogsFn != nil {
		return m.GetContainerLogsFn(ctx, containerID, follow, tail)
	}
	return io.NopCloser(strings.NewReader("test logs")), nil
}

func (m *MockDockerClient) ListContainers(ctx context.Context, labels map[string]string) ([]docker.ContainerInfo, error) {
	if m.ListContainersFn != nil {
		return m.ListContainersFn(ctx, labels)
	}
	return []docker.ContainerInfo{}, nil
}

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	PublishedEvents []*bus.Event
	// Notify, when non-nil, receives a value after each Publish call appends
	// its event — lets a test synchronize with an async publish (e.g.
	// escalateStuckCancel's asyncPublish=true path spawns Publish on its own
	// goroutine) via a channel receive instead of racing on unsynchronized
	// access to PublishedEvents. The send blocks, so set it with enough
	// buffer for however many events the test expects, or drain it promptly.
	Notify chan struct{}
	// OnPublish, when non-nil, runs synchronously inside Publish itself
	// (before appending/notifying) — lets a test simulate a slow or
	// (deliberately, for deadlock regression tests) blocking subscriber,
	// standing in for a real queue subscription's synchronous handler
	// (see bus.MemoryEventBus.Publish/publishToQueue's "deliver
	// synchronously to preserve ordering" comment).
	OnPublish func(subject string, event *bus.Event)
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if m.OnPublish != nil {
		m.OnPublish(subject, event)
	}
	m.PublishedEvents = append(m.PublishedEvents, event)
	if m.Notify != nil {
		m.Notify <- struct{}{}
	}
	return nil
}

func (m *MockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBus) Close() {}

func (m *MockEventBus) IsConnected() bool {
	return true
}

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func newTestRegistry() *registry.Registry {
	log := newTestLogger()
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	return reg
}

var testStopChClosers sync.Map

func closeStopChOnce(stopCh chan struct{}) {
	closer, _ := testStopChClosers.LoadOrStore(stopCh, &sync.Once{})
	closer.(*sync.Once).Do(func() { close(stopCh) })
}

func cleanupManagerStopCh(t *testing.T, mgr *Manager) {
	t.Helper()
	t.Cleanup(func() {
		mgr.closeStopCh()
		if mgr.streamManager != nil {
			mgr.streamManager.Wait()
		}
	})
}

func cleanupStreamManager(t *testing.T, stopCh chan struct{}, streamMgr *StreamManager) {
	t.Helper()
	t.Cleanup(func() {
		closeStopChOnce(stopCh)
		streamMgr.Wait()
	})
}

// MockCredentialsManager implements CredentialsManager for testing
type MockCredentialsManager struct{}

func (m *MockCredentialsManager) GetCredentialValue(ctx context.Context, key string) (string, error) {
	return "", nil
}

// MockProfileResolver implements ProfileResolver for testing
type MockProfileResolver struct{}

func (m *MockProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	return &AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Test Profile",
		AgentID:     "augment-agent",
		AgentName:   "auggie",
		Model:       "claude-sonnet-4-20250514",
	}, nil
}

// newTestManager creates a Manager for testing with mock dependencies.
// Registers t.Cleanup to close stopCh and drain StreamManager goroutines so
// goleak.VerifyTestMain stays green when tests trigger ConnectWorkspaceStream.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	log := newTestLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBus{}
	credsMgr := &MockCredentialsManager{}
	profileResolver := &MockProfileResolver{}
	// Pass nil for runtime - tests don't need them
	mgr := NewManager(reg, eventBus, nil, credsMgr, profileResolver, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr
}

func TestNewManager(t *testing.T) {
	mgr := newTestManager(t)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(mgr.ListExecutions()) != 0 {
		t.Errorf("expected empty executions, got %d", len(mgr.ListExecutions()))
	}
}

func TestNewManager_WiresRemediateNpxCacheDefault(t *testing.T) {
	m := newTestManager(t)
	if m.remediateNpxCache == nil {
		t.Fatal("NewManager must wire default remediateNpxCache")
	}
}

func TestManager_GetExecution(t *testing.T) {
	mgr := newTestManager(t)

	// Manually add an execution for testing
	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.executionStore.Add(execution)

	// Test GetExecution
	got, found := mgr.GetExecution("test-execution-id")
	if !found {
		t.Fatal("expected to find execution")
	}
	if got.ID != execution.ID {
		t.Errorf("expected ID %q, got %q", execution.ID, got.ID)
	}

	// Test not found
	_, found = mgr.GetExecution("non-existent")
	if found {
		t.Error("expected not to find execution")
	}
}

func TestManager_GetExecutionBySessionID(t *testing.T) {
	mgr := newTestManager(t)

	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		SessionID:      "test-session-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.executionStore.Add(execution)

	// Test GetExecutionBySessionID
	got, found := mgr.GetExecutionBySessionID("test-session-id")
	if !found {
		t.Fatal("expected to find execution")
	}
	if got.SessionID != execution.SessionID {
		t.Errorf("expected SessionID %q, got %q", execution.SessionID, got.SessionID)
	}

	// Test not found
	_, found = mgr.GetExecutionBySessionID("non-existent")
	if found {
		t.Error("expected not to find execution")
	}
}

func TestManager_ListExecutions(t *testing.T) {
	mgr := newTestManager(t)

	// Empty list
	list := mgr.ListExecutions()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	// Add executions
	mgr.executionStore.Add(&AgentExecution{ID: "execution-1", TaskID: "task-1", Status: v1.AgentStatusRunning})
	mgr.executionStore.Add(&AgentExecution{ID: "execution-2", TaskID: "task-2", Status: v1.AgentStatusCompleted})

	list = mgr.ListExecutions()
	if len(list) != 2 {
		t.Errorf("expected 2 executions, got %d", len(list))
	}
}

func TestManager_UpdateStatus(t *testing.T) {
	mgr := newTestManager(t)

	execution := &AgentExecution{
		ID:     "test-execution-id",
		TaskID: "test-task-id",
		Status: v1.AgentStatusRunning,
	}

	mgr.executionStore.Add(execution)

	// Test UpdateStatus
	err := mgr.UpdateStatus("test-execution-id", v1.AgentStatusCompleted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetExecution("test-execution-id")
	if got.Status != v1.AgentStatusCompleted {
		t.Errorf("expected status %v, got %v", v1.AgentStatusCompleted, got.Status)
	}

	// Test not found
	err = mgr.UpdateStatus("non-existent", v1.AgentStatusCompleted)
	if err == nil {
		t.Error("expected error for non-existent execution")
	}
}

func TestManager_BeginPromptAlwaysAdvancesPromptGeneration(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{
		ID:        "exec-prompt-generation",
		SessionID: "session-prompt-generation",
		Status:    v1.AgentStatusRunning,
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	require.NoError(t, mgr.BeginPrompt(exec.ID))
	require.True(t, mgr.OwnsPromptGeneration(exec.SessionID, exec.ID, 1))
	require.False(t, mgr.OwnsPromptGeneration(exec.SessionID, "other-execution", 1))

	require.NoError(t, mgr.BeginPrompt(exec.ID))
	require.True(t, mgr.OwnsPromptGeneration(exec.SessionID, exec.ID, 2))
}
