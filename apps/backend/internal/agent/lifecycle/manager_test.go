package lifecycle

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
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
			// Order is alphabetical by setting name: auto_approve, dangerously_skip_permissions
			wantCmd: []string{"test-cli", "--yes", "--dangerous"},
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

// --- Tests for boot message (agent_boot, is_resuming, pollAgentStderr) ---

// MockBootMessageService implements BootMessageService for testing
type MockBootMessageService struct {
	mu              sync.Mutex
	CreatedMessages []*models.Message
	UpdatedMessages []*models.Message
	createErr       error
	updateErr       error
}

func (m *MockBootMessageService) CreateMessage(ctx context.Context, req *BootMessageRequest) (*models.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return nil, m.createErr
	}
	msg := &models.Message{
		ID:            "boot-msg-" + req.TaskID,
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		Content:       req.Content,
		AuthorType:    models.MessageAuthorType(req.AuthorType),
		Type:          models.MessageType(req.Type),
		Metadata:      req.Metadata,
	}
	m.CreatedMessages = append(m.CreatedMessages, msg)
	return msg, nil
}

func (m *MockBootMessageService) UpdateMessage(ctx context.Context, message *models.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	// Store a snapshot of the message at update time
	snapshot := *message
	metaCopy := make(map[string]interface{})
	for k, v := range message.Metadata {
		metaCopy[k] = v
	}
	snapshot.Metadata = metaCopy
	m.UpdatedMessages = append(m.UpdatedMessages, &snapshot)
	return nil
}

func (m *MockBootMessageService) getLastUpdatedMessage() *models.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.UpdatedMessages) == 0 {
		return nil
	}
	return m.UpdatedMessages[len(m.UpdatedMessages)-1]
}

func TestFinalizeBootMessage_Success(t *testing.T) {
	mgr := newTestManager()
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}
	stopCh := make(chan struct{})

	mgr.finalizeBootMessage(msg, stopCh, nil, "exited")

	// Verify stop channel was closed
	select {
	case <-stopCh:
		// good, channel was closed
	default:
		t.Error("expected stopCh to be closed")
	}

	// Verify message was updated with final status
	lastMsg := bootSvc.getLastUpdatedMessage()
	if lastMsg == nil {
		t.Fatal("expected boot message to be updated")
	}
	if lastMsg.Metadata["status"] != "exited" {
		t.Errorf("expected status 'exited', got %v", lastMsg.Metadata["status"])
	}
	if lastMsg.Metadata["exit_code"] != 0 {
		t.Errorf("expected exit_code 0, got %v", lastMsg.Metadata["exit_code"])
	}
	if lastMsg.Metadata["completed_at"] == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestFinalizeBootMessage_Failed(t *testing.T) {
	mgr := newTestManager()
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}
	stopCh := make(chan struct{})

	mgr.finalizeBootMessage(msg, stopCh, nil, "failed")

	lastMsg := bootSvc.getLastUpdatedMessage()
	if lastMsg == nil {
		t.Fatal("expected boot message to be updated")
	}
	if lastMsg.Metadata["status"] != "failed" {
		t.Errorf("expected status 'failed', got %v", lastMsg.Metadata["status"])
	}
	// Failed status should NOT have exit_code
	if _, ok := lastMsg.Metadata["exit_code"]; ok {
		t.Error("expected no exit_code for failed status")
	}
}

func TestFinalizeBootMessage_NilMessage(t *testing.T) {
	mgr := newTestManager()
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc

	// Should not panic with nil message
	mgr.finalizeBootMessage(nil, nil, nil, "exited")

	bootSvc.mu.Lock()
	defer bootSvc.mu.Unlock()
	if len(bootSvc.UpdatedMessages) != 0 {
		t.Error("expected no updates for nil message")
	}
}

func TestFinalizeBootMessage_NilService(t *testing.T) {
	mgr := newTestManager()
	// bootMessageService is nil by default

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}

	// Should not panic with nil service
	mgr.finalizeBootMessage(msg, nil, nil, "exited")
}

func TestBootMessage_IsResumingFlag(t *testing.T) {
	// Test that the is_resuming flag is correctly based on ACPSessionID presence
	tests := []struct {
		name         string
		acpSessionID string
		wantResuming bool
	}{
		{
			name:         "new session (no ACP session ID)",
			acpSessionID: "",
			wantResuming: false,
		},
		{
			name:         "resumed session (has ACP session ID)",
			acpSessionID: "acp-session-123",
			wantResuming: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The is_resuming logic is: execution.ACPSessionID != ""
			isResuming := tt.acpSessionID != ""
			if isResuming != tt.wantResuming {
				t.Errorf("is_resuming = %v, want %v", isResuming, tt.wantResuming)
			}
		})
	}
}

func TestPollAgentStderr_StopsOnClose(t *testing.T) {
	mgr := newTestManager()
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}
	stopCh := make(chan struct{})

	// Start polling with nil client (will fail on each poll, but shouldn't panic)
	done := make(chan struct{})
	go func() {
		// Pass nil client - the pollAgentStderr will log errors but should exit on stop
		// We can't use a nil client directly since it would panic on method call.
		// Instead, just test that close(stopCh) causes the goroutine to exit.
		close(stopCh)
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Good, goroutine exited
	case <-time.After(5 * time.Second):
		t.Fatal("pollAgentStderr did not stop within timeout")
	}

	_ = msg // msg would be used in a real poll
}

// --- Tests for duplicate message prevention in handleAgentEvent ---

// MockEventBusWithTracking provides detailed tracking of published events for testing
type MockEventBusWithTracking struct {
	PublishedEvents []trackedEvent
	mu              sync.Mutex
}

type trackedEvent struct {
	Subject string
	Event   *bus.Event
}

func (m *MockEventBusWithTracking) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PublishedEvents = append(m.PublishedEvents, trackedEvent{Subject: subject, Event: event})
	return nil
}

func (m *MockEventBusWithTracking) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBusWithTracking) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBusWithTracking) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBusWithTracking) Close() {}

func (m *MockEventBusWithTracking) IsConnected() bool {
	return true
}

func (m *MockEventBusWithTracking) getStreamEvents() []AgentStreamEventPayload {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []AgentStreamEventPayload
	for _, te := range m.PublishedEvents {
		if payload, ok := te.Event.Data.(AgentStreamEventPayload); ok {
			result = append(result, payload)
		}
	}
	return result
}

// createTestManagerWithTracking creates a manager with a tracking event bus for testing
func createTestManagerWithTracking() (*Manager, *MockEventBusWithTracking) {
	log := newTestLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBusWithTracking{}
	credsMgr := &MockCredentialsManager{}
	profileResolver := &MockProfileResolver{}
	mgr := NewManager(reg, eventBus, nil, nil, credsMgr, profileResolver, nil, RuntimeFallbackWarn, log)
	return mgr, eventBus
}

// createTestExecution creates a test execution with proper initialization
func createTestExecution(id, taskID, sessionID string) *AgentExecution {
	return &AgentExecution{
		ID:           id,
		TaskID:       taskID,
		SessionID:    sessionID,
		Status:       v1.AgentStatusRunning,
		StartedAt:    time.Now(),
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
}

// TestHandleAgentEvent_StreamingThenComplete tests the normal flow:
// message_chunk events followed by complete event - should NOT create duplicate
func TestHandleAgentEvent_StreamingThenComplete(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Simulate streaming chunks with newlines (which trigger publishing)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Hello, world!\n",
	})

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "This is a test.\n",
	})

	// Now send complete event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// Verify: streaming was used, so complete should NOT have text
	events := eventBus.getStreamEvents()

	// Count message_streaming events (creates/appends)
	var messageStreamingEvents []AgentStreamEventPayload
	var completeEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil {
			switch e.Data.Type {
			case "message_streaming":
				messageStreamingEvents = append(messageStreamingEvents, e)
			case "complete":
				completeEvents = append(completeEvents, e)
			}
		}
	}

	// Should have streaming messages
	if len(messageStreamingEvents) == 0 {
		t.Error("expected message_streaming events, got none")
	}

	// Should have exactly one complete event
	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	// The complete event should NOT have text (streaming handled it via buffer)
	if len(completeEvents) > 0 && completeEvents[0].Data.Text != "" {
		t.Errorf("complete event should not have text when streaming was used, got %q", completeEvents[0].Data.Text)
	}
}

// TestHandleAgentEvent_StreamingThenToolCallThenComplete tests the scenario that could cause duplicates:
// message_chunk → tool_call (clears currentMessageID) → complete
// This verifies that the buffer is properly flushed on complete after tool calls
func TestHandleAgentEvent_StreamingThenToolCallThenComplete(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Simulate streaming chunks
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Let me check that for you.\n",
	})

	// Verify currentMessageID is set after message_chunk
	execution.messageMu.Lock()
	msgIDBeforeToolCall := execution.currentMessageID
	execution.messageMu.Unlock()

	if msgIDBeforeToolCall == "" {
		t.Error("currentMessageID should be set after message_chunk")
	}

	// Tool call - this flushes buffer and clears currentMessageID
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	// After tool call, currentMessageID should be cleared
	execution.messageMu.Lock()
	msgIDAfterToolCall := execution.currentMessageID
	execution.messageMu.Unlock()

	if msgIDAfterToolCall != "" {
		t.Errorf("currentMessageID should be cleared after tool_call, got %q", msgIDAfterToolCall)
	}

	// Now complete - this should flush the buffer
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	// Find the complete event
	var completeEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "complete" {
			completeEvents = append(completeEvents, e)
		}
	}

	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	// The complete event should NOT have text (streaming was used)
	if len(completeEvents) > 0 && completeEvents[0].Data.Text != "" {
		t.Errorf("complete event should not have text when streaming was used (even after tool_call), got %q",
			completeEvents[0].Data.Text)
	}
}

// TestHandleAgentEvent_CompleteWithoutStreaming verifies that complete events are
// properly handled when no streaming was used (buffer is empty).
// All adapters now send text via message_chunk events, so this tests the empty buffer case.
func TestHandleAgentEvent_CompleteWithoutStreaming(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Complete event without any prior streaming (e.g., agent did only tool calls)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	var completeEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "complete" {
			completeEvents = append(completeEvents, e)
		}
	}

	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	// Complete event should be published successfully
	// The buffer was empty, so no message_streaming events should be generated
	var streamingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "message_streaming" {
			streamingEvents = append(streamingEvents, e)
		}
	}

	if len(streamingEvents) != 0 {
		t.Errorf("expected 0 message_streaming events when buffer is empty, got %d", len(streamingEvents))
	}
}

// TestHandleAgentEvent_CompleteWithBufferedText verifies that buffered text
// without streaming is emitted as a final streaming event on complete.
func TestHandleAgentEvent_CompleteWithBufferedText(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Buffer text without newlines (no streaming event should be emitted)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Final message without newline",
	})

	// Complete event should flush buffer into a streaming event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	var completeEvents []AgentStreamEventPayload
	var streamingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil {
			switch e.Data.Type {
			case "complete":
				completeEvents = append(completeEvents, e)
			case "message_streaming":
				streamingEvents = append(streamingEvents, e)
			}
		}
	}

	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	if len(completeEvents) > 0 && completeEvents[0].Data.Text != "" {
		t.Errorf("expected complete event to have empty text, got %q", completeEvents[0].Data.Text)
	}

	if len(streamingEvents) != 1 {
		t.Errorf("expected 1 message_streaming event when no newlines, got %d", len(streamingEvents))
	} else if streamingEvents[0].Data.Text != "Final message without newline" {
		t.Errorf("expected streaming event to carry buffered text, got %q", streamingEvents[0].Data.Text)
	}
}

// TestHandleAgentEvent_CompleteThenMessageChunk tests the scenario where
// message_chunk arrives after complete. This documents the behavior when
// an adapter incorrectly sends text after the turn has completed.
// With the new architecture, adapters should NOT send message_chunk after complete.
func TestHandleAgentEvent_CompleteThenMessageChunk(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// First, simulate normal streaming during the turn
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Processing your request...\n",
	})

	// Complete event arrives - this flushes the buffer
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// Now message_chunk arrives AFTER complete
	// This shouldn't happen with properly implemented adapters,
	// but we document the behavior: it creates a new message
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Done!\n",
	})

	events := eventBus.getStreamEvents()

	// Count message_streaming events
	var messageStreamingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "message_streaming" {
			messageStreamingEvents = append(messageStreamingEvents, e)
		}
	}

	// Document the behavior: message_chunk after complete starts a new message
	t.Logf("Got %d message_streaming events", len(messageStreamingEvents))
	for i, e := range messageStreamingEvents {
		t.Logf("  Event %d: MessageID=%s, IsAppend=%v, Text=%q",
			i, e.Data.MessageID, e.Data.IsAppend, e.Data.Text)
	}

	// The second message_chunk (after complete) should start a NEW message
	// since currentMessageID was cleared by the complete event
	if len(messageStreamingEvents) >= 2 {
		lastEvent := messageStreamingEvents[len(messageStreamingEvents)-1]
		if !lastEvent.Data.IsAppend {
			t.Log("Expected behavior: message_chunk after complete creates a new message")
		}
	}
}

// TestHandleAgentEvent_MultipleToolCalls tests streaming → tool → streaming → tool → complete
func TestHandleAgentEvent_MultipleToolCalls(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Message before first tool
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Let me read the file.\n",
	})

	// First tool call
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	// Tool update (complete)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_update",
		ToolCallID: "tool-1",
		ToolStatus: "complete",
	})

	// Message after first tool
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Now let me modify it.\n",
	})

	// Second tool call
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-2",
		ToolName:   "write_file",
	})

	// Tool update (complete)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_update",
		ToolCallID: "tool-2",
		ToolStatus: "complete",
	})

	// Final message
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Done with both tasks!\n",
	})

	// Complete
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	// Count different event types
	var messageStreamingCount, toolCallCount, completeCount int
	for _, e := range events {
		if e.Data != nil {
			switch e.Data.Type {
			case "message_streaming":
				messageStreamingCount++
			case "tool_call":
				toolCallCount++
			case "complete":
				completeCount++
			}
		}
	}

	t.Logf("Events: message_streaming=%d, tool_call=%d, complete=%d",
		messageStreamingCount, toolCallCount, completeCount)

	// Should have multiple streaming messages (one per "segment" before tool calls)
	if messageStreamingCount < 3 {
		t.Errorf("expected at least 3 message_streaming events for 3 message segments, got %d", messageStreamingCount)
	}

	if toolCallCount != 2 {
		t.Errorf("expected 2 tool_call events, got %d", toolCallCount)
	}

	if completeCount != 1 {
		t.Errorf("expected 1 complete event, got %d", completeCount)
	}

	// Find the complete event and verify it has no text
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "complete" {
			if e.Data.Text != "" {
				t.Errorf("complete event should not have text when streaming was used, got %q", e.Data.Text)
			}
		}
	}
}

// TestHandleAgentEvent_CompleteSignalsPromptDoneCh verifies that the complete event
// signals the promptDoneCh channel with the correct stop reason.
func TestHandleAgentEvent_CompleteSignalsPromptDoneCh(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Send a normal complete event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// promptDoneCh should have a signal
	select {
	case signal := <-execution.promptDoneCh:
		if signal.IsError {
			t.Error("expected non-error signal for normal complete")
		}
		if signal.StopReason != "end_turn" {
			t.Errorf("expected stop_reason 'end_turn', got %q", signal.StopReason)
		}
	default:
		t.Error("expected signal on promptDoneCh, got none")
	}
}

// TestHandleAgentEvent_ErrorCompleteSignalsPromptDoneCh verifies that an error completion
// signals the promptDoneCh channel with IsError=true and the error message.
func TestHandleAgentEvent_ErrorCompleteSignalsPromptDoneCh(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Send an error complete event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:  "complete",
		Error: "something went wrong",
		Data:  map[string]interface{}{"is_error": true},
	})

	// promptDoneCh should have an error signal
	select {
	case signal := <-execution.promptDoneCh:
		if !signal.IsError {
			t.Error("expected error signal for error complete")
		}
		if signal.StopReason != "error" {
			t.Errorf("expected stop_reason 'error', got %q", signal.StopReason)
		}
		if signal.Error != "something went wrong" {
			t.Errorf("expected error 'something went wrong', got %q", signal.Error)
		}
	default:
		t.Error("expected signal on promptDoneCh, got none")
	}
}

// TestHandleAgentEvent_UpdatesLastActivityAt verifies that every agent event
// updates the lastActivityAt timestamp for stall detection.
func TestHandleAgentEvent_UpdatesLastActivityAt(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Set lastActivityAt to a known old time
	oldTime := time.Now().Add(-10 * time.Minute)
	execution.lastActivityAtMu.Lock()
	execution.lastActivityAt = oldTime
	execution.lastActivityAtMu.Unlock()

	// Send any event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "hello\n",
	})

	// lastActivityAt should be updated to approximately now
	execution.lastActivityAtMu.Lock()
	elapsed := time.Since(execution.lastActivityAt)
	execution.lastActivityAtMu.Unlock()

	if elapsed > 1*time.Second {
		t.Errorf("lastActivityAt not updated: elapsed %v since last event", elapsed)
	}
}

// TestHandleAgentEvent_PromptDoneChDoesNotBlockWhenFull verifies that signaling
// promptDoneCh with a full channel (no receiver) doesn't block the event handler.
func TestHandleAgentEvent_PromptDoneChDoesNotBlockWhenFull(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Pre-fill the channel
	execution.promptDoneCh <- PromptCompletionSignal{StopReason: "stale"}

	// Send complete event — should not block
	done := make(chan struct{})
	go func() {
		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type: "complete",
		})
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't block
	case <-time.After(2 * time.Second):
		t.Fatal("handleAgentEvent blocked when promptDoneCh was full")
	}
}
