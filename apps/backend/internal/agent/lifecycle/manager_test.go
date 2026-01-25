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
	return NewManager(reg, eventBus, nil, nil, credsMgr, profileResolver, nil, RuntimeFallbackWarn, log)
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

func TestManager_MarkCompleted_Success(t *testing.T) {
	mgr := newTestManager()

	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.executionStore.Add(execution)

	// Mark as completed successfully (exit code 0)
	err := mgr.MarkCompleted("test-execution-id", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetExecution("test-execution-id")
	if got.Status != v1.AgentStatusCompleted {
		t.Errorf("expected status %v, got %v", v1.AgentStatusCompleted, got.Status)
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

	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.executionStore.Add(execution)

	// Mark as failed
	err := mgr.MarkCompleted("test-execution-id", 1, "process failed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetExecution("test-execution-id")
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
		t.Error("expected error for non-existent execution")
	}
}

func TestManager_RemoveExecution(t *testing.T) {
	mgr := newTestManager()

	execution := &AgentExecution{
		ID:          "test-execution-id",
		TaskID:      "test-task-id",
		SessionID:   "test-session-id",
		ContainerID: "container-123",
	}

	mgr.executionStore.Add(execution)

	// Remove execution
	mgr.RemoveExecution("test-execution-id")

	// Verify it's gone from all maps
	if _, found := mgr.GetExecution("test-execution-id"); found {
		t.Error("execution should be removed from executions map")
	}
	if _, found := mgr.GetExecutionBySessionID("test-session-id"); found {
		t.Error("execution should be removed from bySession map")
	}

	// Remove non-existent should not panic
	mgr.RemoveExecution("non-existent")
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

func TestManager_BuildPassthroughResumeCommand(t *testing.T) {
	mgr := newTestManager()

	tests := []struct {
		name        string
		agentConfig *registry.AgentTypeConfig
		profileInfo *AgentProfileInfo
		wantCmd     []string
	}{
		{
			name: "basic command without profile",
			agentConfig: &registry.AgentTypeConfig{
				ID: "test-agent",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"test-cli", "--verbose"},
				},
			},
			profileInfo: nil,
			wantCmd:     []string{"test-cli", "--verbose"},
		},
		{
			name: "command with model flag",
			agentConfig: &registry.AgentTypeConfig{
				ID: "test-agent",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"test-cli"},
					ModelFlag:      "--model {model}",
				},
			},
			profileInfo: &AgentProfileInfo{
				Model: "gpt-4",
			},
			wantCmd: []string{"test-cli", "--model", "gpt-4"},
		},
		{
			name: "command with resume flag",
			agentConfig: &registry.AgentTypeConfig{
				ID: "test-agent",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"test-cli"},
					ResumeFlag:     "-c",
				},
			},
			profileInfo: nil,
			wantCmd:     []string{"test-cli", "-c"},
		},
		{
			name: "command with multi-word resume flag",
			agentConfig: &registry.AgentTypeConfig{
				ID: "gemini-agent",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"gemini"},
					ResumeFlag:     "--resume latest",
				},
			},
			profileInfo: nil,
			wantCmd:     []string{"gemini", "--resume", "latest"},
		},
		{
			name: "command with permission settings",
			agentConfig: &registry.AgentTypeConfig{
				ID: "test-agent",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"test-cli"},
				},
				PermissionSettings: map[string]registry.PermissionSetting{
					"dangerously_skip_permissions": {
						Supported:   true,
						ApplyMethod: "cli_flag",
						CLIFlag:     "--dangerous",
					},
					"auto_approve": {
						Supported:   true,
						ApplyMethod: "cli_flag",
						CLIFlag:     "--yes",
					},
				},
			},
			profileInfo: &AgentProfileInfo{
				DangerouslySkipPermissions: true,
				AutoApprove:                true,
			},
			wantCmd: []string{"test-cli", "--dangerous", "--yes"},
		},
		{
			name: "full command with all options",
			agentConfig: &registry.AgentTypeConfig{
				ID: "claude-code",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"npx", "-y", "@anthropic-ai/claude-code"},
					ModelFlag:      "--model {model}",
					ResumeFlag:     "-c",
				},
				PermissionSettings: map[string]registry.PermissionSetting{
					"dangerously_skip_permissions": {
						Supported:   true,
						ApplyMethod: "cli_flag",
						CLIFlag:     "--dangerously-skip-permissions",
					},
				},
			},
			profileInfo: &AgentProfileInfo{
				Model:                      "claude-sonnet-4",
				DangerouslySkipPermissions: true,
			},
			wantCmd: []string{"npx", "-y", "@anthropic-ai/claude-code", "--model", "claude-sonnet-4", "--dangerously-skip-permissions", "-c"},
		},
		{
			name: "permission setting with cli_flag_value",
			agentConfig: &registry.AgentTypeConfig{
				ID: "test-agent",
				PassthroughConfig: registry.PassthroughConfig{
					Supported:      true,
					PassthroughCmd: []string{"test-cli"},
				},
				PermissionSettings: map[string]registry.PermissionSetting{
					"auto_approve": {
						Supported:    true,
						ApplyMethod:  "cli_flag",
						CLIFlag:      "--approve-level",
						CLIFlagValue: "all",
					},
				},
			},
			profileInfo: &AgentProfileInfo{
				AutoApprove: true,
			},
			wantCmd: []string{"test-cli", "--approve-level", "all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mgr.buildPassthroughResumeCommand(tt.agentConfig, tt.profileInfo)

			if len(got) != len(tt.wantCmd) {
				t.Errorf("buildPassthroughResumeCommand() = %v, want %v", got, tt.wantCmd)
				return
			}

			for i, arg := range got {
				if arg != tt.wantCmd[i] {
					t.Errorf("buildPassthroughResumeCommand()[%d] = %q, want %q", i, arg, tt.wantCmd[i])
				}
			}
		})
	}
}

func TestManager_VerifyPassthroughEnabled(t *testing.T) {
	tests := []struct {
		name      string
		profileID string
		wantErr   bool
	}{
		{
			name:      "valid profile with passthrough enabled",
			profileID: "test-profile",
			wantErr:   false,
		},
		{
			name:      "empty profile ID",
			profileID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager()

			// Override profile resolver for this test
			if tt.profileID != "" {
				mgr.profileResolver = &mockPassthroughProfileResolver{
					cliPassthrough: true,
				}
			}

			err := mgr.verifyPassthroughEnabled(context.Background(), "test-session", tt.profileID)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyPassthroughEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// mockPassthroughProfileResolver is a mock for testing passthrough verification
type mockPassthroughProfileResolver struct {
	cliPassthrough bool
	err            error
}

func (m *mockPassthroughProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &AgentProfileInfo{
		ProfileID:      profileID,
		CLIPassthrough: m.cliPassthrough,
	}, nil
}
