package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

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

// testAgentConfig returns a default AgentConfig for testing
func testAgentConfig() config.AgentConfig {
	return config.AgentConfig{
		Runtime:        "docker",
		StandaloneHost: "localhost",
		StandalonePort: 9999,
	}
}

func newTestRegistry() *registry.Registry {
	log := newTestLogger()
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	return reg
}

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestNewShellHandlers(t *testing.T) {
	log := newTestLogger()

	// NewShellHandlers accepts *lifecycle.Manager, but we can pass nil for basic construction test
	handlers := NewShellHandlers(nil, log)

	if handlers == nil {
		t.Fatal("expected non-nil handlers")
	}
	if handlers.lifecycleMgr != nil {
		t.Error("expected nil lifecycleMgr when nil passed")
	}
	if handlers.logger == nil {
		t.Error("expected non-nil logger")
	}
	if handlers.activeStreams == nil {
		t.Error("expected non-nil activeStreams map")
	}
	if handlers.inputChannels == nil {
		t.Error("expected non-nil inputChannels map")
	}
}

func TestRegisterHandlers(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	// Check that shell.status is registered
	if !dispatcher.HasHandler(ws.ActionShellStatus) {
		t.Error("expected shell.status handler to be registered")
	}

	// Check that shell.input is registered
	if !dispatcher.HasHandler(ws.ActionShellInput) {
		t.Error("expected shell.input handler to be registered")
	}
}

func TestWsShellStatus_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	// Create a message with invalid payload
	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionShellStatus,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsShellStatus(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsShellStatus_MissingTaskID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	// Create a message with empty task_id
	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{TaskID: ""})

	_, err := handlers.wsShellStatus(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing task_id")
	}
	if err.Error() != "task_id is required" {
		t.Errorf("expected 'task_id is required' error, got: %v", err)
	}
}

func TestWsShellInput_InvalidPayload(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	msg := &ws.Message{
		ID:      "test-1",
		Action:  ws.ActionShellInput,
		Payload: json.RawMessage(`{invalid json`),
	}

	_, err := handlers.wsShellInput(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestWsShellInput_MissingTaskID(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellInput, ShellInputRequest{TaskID: "", Data: "test"})

	_, err := handlers.wsShellInput(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing task_id")
	}
	if err.Error() != "task_id is required" {
		t.Errorf("expected 'task_id is required' error, got: %v", err)
	}
}

// newTestManager creates a lifecycle.Manager for testing
func newTestManager() *lifecycle.Manager {
	log := newTestLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBus{}
	return lifecycle.NewManager(nil, reg, eventBus, testAgentConfig(), log)
}

func TestWsShellStatus_NoInstanceFound(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{TaskID: "non-existent-task"})

	resp, err := handlers.wsShellStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return a response indicating no agent available
	var payload map[string]interface{}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if payload["available"] != false {
		t.Errorf("expected available=false, got %v", payload["available"])
	}
	if payload["error"] != "no agent running for this task" {
		t.Errorf("expected 'no agent running for this task', got %v", payload["error"])
	}
}

func TestWsShellStatus_NoClientAvailable(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, log)

	// When no instance exists for the task, handler returns "no agent running"
	// Note: Testing the "agent client not available" path requires injecting an
	// instance with nil agentctl client, which requires access to lifecycle.Manager
	// internal maps. This test verifies the handler works correctly with the manager.
	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{TaskID: "test-task-id"})

	resp, err := handlers.wsShellStatus(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Since there's no instance, we expect the "no agent running" error
	var payload map[string]interface{}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if payload["available"] != false {
		t.Errorf("expected available=false, got %v", payload["available"])
	}
}

func TestWsShellInput_NoInstanceFound(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()
	handlers := NewShellHandlers(mgr, log)

	msg, _ := ws.NewRequest("test-1", ws.ActionShellInput, ShellInputRequest{
		TaskID: "non-existent-task",
		Data:   "test input",
	})

	_, err := handlers.wsShellInput(context.Background(), msg)
	if err == nil {
		t.Error("expected error for non-existent task")
	}
	expectedErr := "no agent running for task non-existent-task"
	if err.Error() != expectedErr {
		t.Errorf("expected '%s', got: %v", expectedErr, err)
	}
}

func TestSendShellInput_NoActiveStream(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	err := handlers.SendShellInput("non-existent-task", "test input")
	if err == nil {
		t.Error("expected error for non-existent stream")
	}
	expectedErr := "no active shell stream for task non-existent-task"
	if err.Error() != expectedErr {
		t.Errorf("expected '%s', got: %v", expectedErr, err)
	}
}

func TestStopShellStream_NonExistent(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	// StopShellStream should not panic when called with non-existent taskID
	handlers.StopShellStream("non-existent-task")
	// No panic = success
}

func TestNewShellHandlers_WithManager(t *testing.T) {
	log := newTestLogger()
	mgr := newTestManager()

	handlers := NewShellHandlers(mgr, log)

	if handlers == nil {
		t.Fatal("expected non-nil handlers")
	}
	if handlers.lifecycleMgr != mgr {
		t.Error("expected lifecycleMgr to be set to provided manager")
	}
}

func TestRegisterHandlers_DispatcherReceivesMessages(t *testing.T) {
	log := newTestLogger()
	handlers := NewShellHandlers(nil, log)

	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	// Test that dispatcher can dispatch to shell.status
	msg, _ := ws.NewRequest("test-1", ws.ActionShellStatus, ShellStatusRequest{TaskID: ""})
	_, err := dispatcher.Dispatch(context.Background(), msg)

	// Should get "task_id is required" error since we passed empty task ID
	if err == nil {
		t.Error("expected error from dispatcher")
	}
}
