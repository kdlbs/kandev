package lifecycle

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
	return NewManager(reg, eventBus, nil, nil, credsMgr, profileResolver, log)
}

func TestNewManager(t *testing.T) {
	mgr := newTestManager()

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(mgr.ListInstances()) != 0 {
		t.Errorf("expected empty instances, got %d", len(mgr.ListInstances()))
	}
}

func TestManager_GetInstance(t *testing.T) {
	mgr := newTestManager()

	// Manually add an instance for testing
	instance := &AgentInstance{
		ID:             "test-instance-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.instanceStore.Add(instance)

	// Test GetInstance
	got, found := mgr.GetInstance("test-instance-id")
	if !found {
		t.Fatal("expected to find instance")
	}
	if got.ID != instance.ID {
		t.Errorf("expected ID %q, got %q", instance.ID, got.ID)
	}

	// Test not found
	_, found = mgr.GetInstance("non-existent")
	if found {
		t.Error("expected not to find instance")
	}
}

func TestManager_GetInstanceByTaskID(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:             "test-instance-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.instanceStore.Add(instance)

	// Test GetInstanceByTaskID
	got, found := mgr.GetInstanceByTaskID("test-task-id")
	if !found {
		t.Fatal("expected to find instance")
	}
	if got.TaskID != instance.TaskID {
		t.Errorf("expected TaskID %q, got %q", instance.TaskID, got.TaskID)
	}

	// Test not found
	_, found = mgr.GetInstanceByTaskID("non-existent")
	if found {
		t.Error("expected not to find instance")
	}
}

func TestManager_GetInstanceByContainerID(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:             "test-instance-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.instanceStore.Add(instance)

	// Test GetInstanceByContainerID
	got, found := mgr.GetInstanceByContainerID("container-123")
	if !found {
		t.Fatal("expected to find instance")
	}
	if got.ContainerID != instance.ContainerID {
		t.Errorf("expected ContainerID %q, got %q", instance.ContainerID, got.ContainerID)
	}

	// Test not found
	_, found = mgr.GetInstanceByContainerID("non-existent")
	if found {
		t.Error("expected not to find instance")
	}
}

func TestManager_ListInstances(t *testing.T) {
	mgr := newTestManager()

	// Empty list
	list := mgr.ListInstances()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	// Add instances
	mgr.instanceStore.Add(&AgentInstance{ID: "instance-1", TaskID: "task-1", Status: v1.AgentStatusRunning})
	mgr.instanceStore.Add(&AgentInstance{ID: "instance-2", TaskID: "task-2", Status: v1.AgentStatusCompleted})

	list = mgr.ListInstances()
	if len(list) != 2 {
		t.Errorf("expected 2 instances, got %d", len(list))
	}
}

func TestManager_UpdateStatus(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:     "test-instance-id",
		TaskID: "test-task-id",
		Status: v1.AgentStatusRunning,
	}

	mgr.instanceStore.Add(instance)

	// Test UpdateStatus
	err := mgr.UpdateStatus("test-instance-id", v1.AgentStatusCompleted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetInstance("test-instance-id")
	if got.Status != v1.AgentStatusCompleted {
		t.Errorf("expected status %v, got %v", v1.AgentStatusCompleted, got.Status)
	}

	// Test not found
	err = mgr.UpdateStatus("non-existent", v1.AgentStatusCompleted)
	if err == nil {
		t.Error("expected error for non-existent instance")
	}
}

func TestManager_UpdateProgress(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:       "test-instance-id",
		TaskID:   "test-task-id",
		Status:   v1.AgentStatusRunning,
		Progress: 0,
	}

	mgr.instanceStore.Add(instance)

	// Test UpdateProgress
	err := mgr.UpdateProgress("test-instance-id", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetInstance("test-instance-id")
	if got.Progress != 50 {
		t.Errorf("expected progress 50, got %d", got.Progress)
	}

	// Test not found
	err = mgr.UpdateProgress("non-existent", 50)
	if err == nil {
		t.Error("expected error for non-existent instance")
	}
}

func TestManager_MarkCompleted_Success(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:             "test-instance-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
		Progress:       50,
	}

	mgr.instanceStore.Add(instance)

	// Mark as completed successfully (exit code 0)
	err := mgr.MarkCompleted("test-instance-id", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetInstance("test-instance-id")
	if got.Status != v1.AgentStatusCompleted {
		t.Errorf("expected status %v, got %v", v1.AgentStatusCompleted, got.Status)
	}
	if got.Progress != 100 {
		t.Errorf("expected progress 100, got %d", got.Progress)
	}
	if got.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", got.ExitCode)
	}
}

func TestManager_MarkCompleted_Failure(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:             "test-instance-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.instanceStore.Add(instance)

	// Mark as failed
	err := mgr.MarkCompleted("test-instance-id", 1, "process failed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetInstance("test-instance-id")
	if got.Status != v1.AgentStatusFailed {
		t.Errorf("expected status %v, got %v", v1.AgentStatusFailed, got.Status)
	}
	if got.ErrorMessage != "process failed" {
		t.Errorf("expected error message 'process failed', got %q", got.ErrorMessage)
	}
	if got.ExitCode == nil || *got.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %v", got.ExitCode)
	}
}

func TestManager_MarkCompleted_NotFound(t *testing.T) {
	mgr := newTestManager()

	err := mgr.MarkCompleted("non-existent", 0, "")
	if err == nil {
		t.Error("expected error for non-existent instance")
	}
}

func TestManager_RemoveInstance(t *testing.T) {
	mgr := newTestManager()

	instance := &AgentInstance{
		ID:          "test-instance-id",
		TaskID:      "test-task-id",
		ContainerID: "container-123",
	}

	mgr.instanceStore.Add(instance)

	// Remove instance
	mgr.RemoveInstance("test-instance-id")

	// Verify it's gone from all maps
	if _, found := mgr.GetInstance("test-instance-id"); found {
		t.Error("instance should be removed from instances map")
	}
	if _, found := mgr.GetInstanceByTaskID("test-task-id"); found {
		t.Error("instance should be removed from byTask map")
	}
	if _, found := mgr.GetInstanceByContainerID("container-123"); found {
		t.Error("instance should be removed from byContainer map")
	}

	// Remove non-existent should not panic
	mgr.RemoveInstance("non-existent")
}

func TestManager_StartStop(t *testing.T) {
	mgr := newTestManager()

	ctx := context.Background()

	// Test Start
	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting manager: %v", err)
	}

	// Test Stop
	err = mgr.Stop()
	if err != nil {
		t.Fatalf("unexpected error stopping manager: %v", err)
	}
}
