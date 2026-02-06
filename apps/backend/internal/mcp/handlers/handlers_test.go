package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// mockClarificationService implements ClarificationService for testing.
type mockClarificationService struct {
	createRequestFn    func(req *clarification.Request) string
	waitForResponseFn  func(ctx context.Context, pendingID string) (*clarification.Response, error)
}

func (m *mockClarificationService) CreateRequest(req *clarification.Request) string {
	if m.createRequestFn != nil {
		return m.createRequestFn(req)
	}
	return "pending-123"
}

func (m *mockClarificationService) WaitForResponse(ctx context.Context, pendingID string) (*clarification.Response, error) {
	if m.waitForResponseFn != nil {
		return m.waitForResponseFn(ctx, pendingID)
	}
	return nil, fmt.Errorf("clarification request timed out: %s", pendingID)
}

// mockMessageCreator implements MessageCreator for testing.
type mockMessageCreator struct {
	mu                              sync.Mutex
	createClarificationCalled       bool
	updateClarificationCalled       bool
	updateClarificationStatus       string
	updateClarificationSessionID    string
	updateClarificationPendingID    string
}

func (m *mockMessageCreator) CreateClarificationRequestMessage(_ context.Context, _, _, _ string, _ clarification.Question, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createClarificationCalled = true
	return "msg-123", nil
}

func (m *mockMessageCreator) UpdateClarificationMessage(_ context.Context, sessionID, pendingID, status string, _ *clarification.Answer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateClarificationCalled = true
	m.updateClarificationStatus = status
	m.updateClarificationSessionID = sessionID
	m.updateClarificationPendingID = pendingID
	return nil
}

// mockSessionRepo implements SessionRepository for testing.
type mockSessionRepo struct {
	mu     sync.Mutex
	states []sessionStateUpdate
}

type sessionStateUpdate struct {
	SessionID string
	State     models.TaskSessionState
}

func (m *mockSessionRepo) UpdateTaskSessionState(_ context.Context, sessionID string, state models.TaskSessionState, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, sessionStateUpdate{SessionID: sessionID, State: state})
	return nil
}

func (m *mockSessionRepo) getStates() []sessionStateUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sessionStateUpdate, len(m.states))
	copy(result, m.states)
	return result
}

// mockTaskRepo implements TaskRepository for testing.
type mockTaskRepo struct {
	mu     sync.Mutex
	states []taskStateUpdate
}

type taskStateUpdate struct {
	TaskID string
	State  v1.TaskState
}

func (m *mockTaskRepo) UpdateTaskState(_ context.Context, taskID string, state v1.TaskState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, taskStateUpdate{TaskID: taskID, State: state})
	return nil
}

func (m *mockTaskRepo) getStates() []taskStateUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]taskStateUpdate, len(m.states))
	copy(result, m.states)
	return result
}

// mockEventBus implements EventBus for testing.
type mockEventBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (m *mockEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func newTestHandlers(
	clarSvc ClarificationService,
	msgCreator MessageCreator,
	sessionRepo SessionRepository,
	taskRepo TaskRepository,
	eventBus EventBus,
) *Handlers {
	return &Handlers{
		clarificationSvc: clarSvc,
		messageCreator:   msgCreator,
		sessionRepo:      sessionRepo,
		taskRepo:         taskRepo,
		eventBus:         eventBus,
		logger:           newTestLogger(),
	}
}

func makeAskUserQuestionMessage(t *testing.T, sessionID, taskID, prompt string) *ws.Message {
	t.Helper()
	payload := map[string]interface{}{
		"session_id": sessionID,
		"task_id":    taskID,
		"question": map[string]interface{}{
			"id":     "q1",
			"prompt": prompt,
			"options": []map[string]string{
				{"option_id": "opt_1", "label": "Yes", "description": "Proceed"},
				{"option_id": "opt_2", "label": "No", "description": "Cancel"},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return &ws.Message{
		ID:      "msg-1",
		Action:  ws.ActionMCPAskUserQuestion,
		Payload: data,
	}
}

func TestHandleAskUserQuestion_TimeoutRestoresSessionState(t *testing.T) {
	sessionRepo := &mockSessionRepo{}
	taskRepo := &mockTaskRepo{}
	eventBusMock := &mockEventBus{}
	msgCreator := &mockMessageCreator{}

	clarSvc := &mockClarificationService{
		waitForResponseFn: func(_ context.Context, pendingID string) (*clarification.Response, error) {
			return nil, fmt.Errorf("clarification request timed out: %s", pendingID)
		},
	}

	h := newTestHandlers(clarSvc, msgCreator, sessionRepo, taskRepo, eventBusMock)
	msg := makeAskUserQuestionMessage(t, "session-1", "task-1", "Which option?")

	result, err := h.handleAskUserQuestion(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return an error message (WS error response)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify session state transitions:
	// 1. WAITING_FOR_INPUT (when question is asked)
	// 2. RUNNING (restored after timeout)
	sessionStates := sessionRepo.getStates()
	if len(sessionStates) != 2 {
		t.Fatalf("expected 2 session state updates, got %d: %+v", len(sessionStates), sessionStates)
	}
	if sessionStates[0].State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected first state to be WAITING_FOR_INPUT, got %s", sessionStates[0].State)
	}
	if sessionStates[1].State != models.TaskSessionStateRunning {
		t.Errorf("expected second state to be RUNNING, got %s", sessionStates[1].State)
	}

	// Verify task state transitions:
	// 1. REVIEW (when question is asked)
	// 2. IN_PROGRESS (restored after timeout)
	taskStates := taskRepo.getStates()
	if len(taskStates) != 2 {
		t.Fatalf("expected 2 task state updates, got %d: %+v", len(taskStates), taskStates)
	}
	if taskStates[0].State != v1.TaskStateReview {
		t.Errorf("expected first task state to be REVIEW, got %s", taskStates[0].State)
	}
	if taskStates[1].State != v1.TaskStateInProgress {
		t.Errorf("expected second task state to be IN_PROGRESS, got %s", taskStates[1].State)
	}
}

func TestHandleAskUserQuestion_TimeoutUpdatesClarificationMessage(t *testing.T) {
	sessionRepo := &mockSessionRepo{}
	taskRepo := &mockTaskRepo{}
	eventBusMock := &mockEventBus{}
	msgCreator := &mockMessageCreator{}

	clarSvc := &mockClarificationService{
		waitForResponseFn: func(_ context.Context, pendingID string) (*clarification.Response, error) {
			return nil, fmt.Errorf("clarification request timed out: %s", pendingID)
		},
	}

	h := newTestHandlers(clarSvc, msgCreator, sessionRepo, taskRepo, eventBusMock)
	msg := makeAskUserQuestionMessage(t, "session-1", "task-1", "Which option?")

	_, err := h.handleAskUserQuestion(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify clarification message was updated to "expired"
	msgCreator.mu.Lock()
	defer msgCreator.mu.Unlock()

	if !msgCreator.updateClarificationCalled {
		t.Error("expected UpdateClarificationMessage to be called on timeout")
	}
	if msgCreator.updateClarificationStatus != "expired" {
		t.Errorf("expected clarification status 'expired', got %q", msgCreator.updateClarificationStatus)
	}
	if msgCreator.updateClarificationSessionID != "session-1" {
		t.Errorf("expected session ID 'session-1', got %q", msgCreator.updateClarificationSessionID)
	}
}

func TestHandleAskUserQuestion_SuccessRestoresSessionState(t *testing.T) {
	sessionRepo := &mockSessionRepo{}
	taskRepo := &mockTaskRepo{}
	eventBusMock := &mockEventBus{}
	msgCreator := &mockMessageCreator{}

	clarSvc := &mockClarificationService{
		waitForResponseFn: func(_ context.Context, _ string) (*clarification.Response, error) {
			return &clarification.Response{
				PendingID: "pending-123",
				Answer: &clarification.Answer{
					QuestionID:      "q1",
					SelectedOptions: []string{"opt_1"},
				},
				RespondedAt: time.Now(),
			}, nil
		},
	}

	h := newTestHandlers(clarSvc, msgCreator, sessionRepo, taskRepo, eventBusMock)
	msg := makeAskUserQuestionMessage(t, "session-1", "task-1", "Which option?")

	result, err := h.handleAskUserQuestion(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify session state transitions on success path too
	sessionStates := sessionRepo.getStates()
	if len(sessionStates) != 2 {
		t.Fatalf("expected 2 session state updates, got %d", len(sessionStates))
	}
	if sessionStates[0].State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected first state WAITING_FOR_INPUT, got %s", sessionStates[0].State)
	}
	if sessionStates[1].State != models.TaskSessionStateRunning {
		t.Errorf("expected second state RUNNING, got %s", sessionStates[1].State)
	}

	// Verify clarification message was NOT updated to expired on success
	msgCreator.mu.Lock()
	defer msgCreator.mu.Unlock()
	if msgCreator.updateClarificationCalled {
		t.Error("UpdateClarificationMessage should not be called on success path")
	}
}

func TestHandleAskUserQuestion_ContextCancelledRestoresState(t *testing.T) {
	sessionRepo := &mockSessionRepo{}
	taskRepo := &mockTaskRepo{}
	eventBusMock := &mockEventBus{}
	msgCreator := &mockMessageCreator{}

	clarSvc := &mockClarificationService{
		waitForResponseFn: func(_ context.Context, _ string) (*clarification.Response, error) {
			return nil, context.Canceled
		},
	}

	h := newTestHandlers(clarSvc, msgCreator, sessionRepo, taskRepo, eventBusMock)
	msg := makeAskUserQuestionMessage(t, "session-1", "task-1", "Which option?")

	result, err := h.handleAskUserQuestion(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Session should still be restored even on context cancellation
	sessionStates := sessionRepo.getStates()
	if len(sessionStates) != 2 {
		t.Fatalf("expected 2 session state updates, got %d", len(sessionStates))
	}
	if sessionStates[1].State != models.TaskSessionStateRunning {
		t.Errorf("expected session restored to RUNNING, got %s", sessionStates[1].State)
	}

	// Clarification message should be marked as expired
	msgCreator.mu.Lock()
	defer msgCreator.mu.Unlock()
	if !msgCreator.updateClarificationCalled {
		t.Error("expected UpdateClarificationMessage to be called on context cancellation")
	}
	if msgCreator.updateClarificationStatus != "expired" {
		t.Errorf("expected status 'expired', got %q", msgCreator.updateClarificationStatus)
	}
}
