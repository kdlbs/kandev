package lifecycle

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

func (a *testAgent) DefaultModel() string { return a.defaultModel }
func (a *testAgent) ListModels(_ context.Context) (*agents.ModelList, error) {
	return &agents.ModelList{}, nil
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
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.PublishedEvents = append(m.PublishedEvents, event)
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

// newTestManager creates a Manager for testing with mock dependencies
func newTestManager() *Manager {
	log := newTestLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBus{}
	credsMgr := &MockCredentialsManager{}
	profileResolver := &MockProfileResolver{}
	// Pass nil for runtime and containerManager - tests don't need them
	return NewManager(reg, eventBus, nil, nil, credsMgr, profileResolver, nil, ExecutorFallbackWarn, log)
}

func TestNewManager(t *testing.T) {
	mgr := newTestManager()

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(mgr.ListExecutions()) != 0 {
		t.Errorf("expected empty executions, got %d", len(mgr.ListExecutions()))
	}
}

func TestManager_GetExecution(t *testing.T) {
	mgr := newTestManager()

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
	mgr := newTestManager()

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
	mgr := newTestManager()

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
	mgr := newTestManager()

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
