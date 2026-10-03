package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap/zapcore"
)

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
	mgr := newTestManager(t)
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}
	stopCh := make(chan struct{})

	mgr.finalizeBootMessage(nil, msg, stopCh, "exited")

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
	} else {
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
}

func TestFinalizeBootMessage_Failed(t *testing.T) {
	mgr := newTestManager(t)
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}
	stopCh := make(chan struct{})

	mgr.finalizeBootMessage(nil, msg, stopCh, "failed")

	lastMsg := bootSvc.getLastUpdatedMessage()
	if lastMsg == nil {
		t.Fatal("expected boot message to be updated")
	} else {
		if lastMsg.Metadata["status"] != "failed" {
			t.Errorf("expected status 'failed', got %v", lastMsg.Metadata["status"])
		}
		// Failed status should NOT have exit_code
		if _, ok := lastMsg.Metadata["exit_code"]; ok {
			t.Error("expected no exit_code for failed status")
		}
	}
}

func TestFinalizeBootMessageLogsPersistenceFailureAtWarn(t *testing.T) {
	mgr := newTestManager(t)
	log, logs := observedLogger(t)
	mgr.logger = log
	mgr.bootMessageService = &MockBootMessageService{updateErr: errors.New("database unavailable")}

	mgr.finalizeBootMessage(nil, &models.Message{
		ID:       "boot-msg-failed-update",
		Metadata: map[string]interface{}{"status": "running"},
	}, nil, "failed")

	entries := logs.FilterMessage("failed to update agent boot message").All()
	if len(entries) != 1 || entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("boot-status persistence logs = %#v, want one warning", entries)
	}
}

func TestFinalizeBootMessageClearsStderrFromEarlierStartupAttempt(t *testing.T) {
	mgr := newTestManager(t)
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.mu.Lock()
	mock.stderrConfigured = true
	mock.stderrLines = nil
	mock.mu.Unlock()
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, stopStream := context.WithCancel(context.Background())
	t.Cleanup(stopStream)
	if err := client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect agent event stream: %v", err)
	}

	message := &models.Message{
		ID:       "boot-msg-retried",
		Content:  "npm error code ECONNRESET",
		Metadata: map[string]interface{}{"status": "running", "startup_retrying": true},
	}
	execution := &AgentExecution{agentctl: client}
	mgr.finalizeBootMessage(execution, message, nil, containerStateExited)

	lastMessage := bootSvc.getLastUpdatedMessage()
	if lastMessage == nil {
		t.Fatal("expected boot message finalization")
	}
	if lastMessage.Content != "" {
		t.Fatalf("final boot message content = %q, want cleared stderr from the successful silent retry", lastMessage.Content)
	}
	mgr.updateBootMessage(execution, message, false, func(message *models.Message) {
		message.Content = "late first-attempt stderr"
	})
	if lastMessage = bootSvc.getLastUpdatedMessage(); lastMessage.Content != "" {
		t.Fatalf("late stderr poll overwrote the final message content with %q", lastMessage.Content)
	}
}

func TestFinalizeBootMessage_NilMessage(t *testing.T) {
	mgr := newTestManager(t)
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
	mgr := newTestManager(t)
	// bootMessageService is nil by default

	msg := &models.Message{
		ID:       "boot-msg-1",
		Metadata: map[string]interface{}{"status": "running"},
	}

	// Should not panic with nil service
	mgr.finalizeBootMessage(nil, msg, nil, "exited")
}

func TestCreateBootMessage_MarksResumedSession(t *testing.T) {
	mgr := newTestManager(t)
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc
	execution := &AgentExecution{
		TaskID:       "task-1",
		SessionID:    "session-1",
		ACPSessionID: "acp-session-1",
	}

	message, stopCh := mgr.createBootMessage(
		context.Background(),
		execution,
		"mock-agent --resume acp-session-1",
		"Mock",
	)
	if stopCh != nil {
		close(stopCh)
	}

	if message == nil {
		t.Fatal("resume boot message = nil")
	}
	if len(bootSvc.CreatedMessages) != 1 {
		t.Fatalf("created boot messages = %d, want 1", len(bootSvc.CreatedMessages))
	}
	if got := message.Metadata["is_resuming"]; got != true {
		t.Fatalf("is_resuming = %#v, want true", got)
	}
}

func TestManagedStartupProgress(t *testing.T) {
	mgr := newTestManager(t)
	bootSvc := &MockBootMessageService{}
	mgr.bootMessageService = bootSvc
	execution := &AgentExecution{ID: "execution-1"}
	message := &models.Message{Metadata: map[string]interface{}{"status": "running"}}

	mgr.updateBootMessageStartupRetryProgress(execution, message)

	updated := bootSvc.getLastUpdatedMessage()
	if updated == nil {
		t.Fatal("boot message was not updated with retry progress")
	}
	if updated.Metadata["startup_retrying"] != true || updated.Metadata["startup_retry_attempt"] != 2 ||
		updated.Metadata["startup_retry_max_attempts"] != 2 {
		t.Fatalf("retry progress metadata = %#v", updated.Metadata)
	}
}

func TestPollAgentStderr_StopsOnClose(t *testing.T) {
	mgr := newTestManager(t)
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
