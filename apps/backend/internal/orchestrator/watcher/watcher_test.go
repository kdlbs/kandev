package watcher

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/acp/protocol"
)

// mockSubscription implements bus.Subscription for testing
type mockSubscription struct {
	valid        bool
	mu           sync.Mutex
	unsubscribed bool
}

func (s *mockSubscription) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.valid = false
	s.unsubscribed = true
	return nil
}

func (s *mockSubscription) IsValid() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.valid
}

// mockEventBus implements bus.EventBus for testing
type mockEventBus struct {
	subscriptions    map[string][]bus.EventHandler
	queuedSubs       map[string]map[string][]bus.EventHandler
	mu               sync.RWMutex
	connected        bool
	subscribeErr     error
	queueSubscribeErr error
}

func newMockEventBus() *mockEventBus {
	return &mockEventBus{
		subscriptions: make(map[string][]bus.EventHandler),
		queuedSubs:    make(map[string]map[string][]bus.EventHandler),
		connected:     true,
	}
}

func (b *mockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Call handlers for matching subjects
	if handlers, ok := b.subscriptions[subject]; ok {
		for _, handler := range handlers {
			go handler(ctx, event)
		}
	}

	// Check queue subscriptions
	for _, queueGroup := range b.queuedSubs {
		if handlers, ok := queueGroup[subject]; ok && len(handlers) > 0 {
			// Deliver to first handler (simplified)
			go handlers[0](ctx, event)
		}
	}

	return nil
}

func (b *mockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	if b.subscribeErr != nil {
		return nil, b.subscribeErr
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscriptions[subject] = append(b.subscriptions[subject], handler)
	return &mockSubscription{valid: true}, nil
}

func (b *mockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	if b.queueSubscribeErr != nil {
		return nil, b.queueSubscribeErr
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.queuedSubs[queue] == nil {
		b.queuedSubs[queue] = make(map[string][]bus.EventHandler)
	}
	b.queuedSubs[queue][subject] = append(b.queuedSubs[queue][subject], handler)

	return &mockSubscription{valid: true}, nil
}

func (b *mockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (b *mockEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connected = false
}

func (b *mockEventBus) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

func createTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error", // Suppress logs during tests
		Format: "console",
	})
	return log
}

func TestNewWatcher(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)
	if w == nil {
		t.Fatal("NewWatcher returned nil")
	}
	if w.running {
		t.Error("watcher should not be running initially")
	}
}

func TestStartStop(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{
		OnTaskCreated: func(ctx context.Context, data TaskEventData) {},
	}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)

	err := w.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !w.IsRunning() {
		t.Error("watcher should be running after Start")
	}

	err = w.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if w.IsRunning() {
		t.Error("watcher should not be running after Stop")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)

	_ = w.Start(context.Background())
	err := w.Start(context.Background())

	// Starting an already running watcher should be a no-op (no error)
	if err != nil {
		t.Errorf("starting already running watcher should not error: %v", err)
	}

	_ = w.Stop()
}

func TestStopNotRunning(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)

	err := w.Stop()
	// Stopping a not running watcher should be a no-op (no error)
	if err != nil {
		t.Errorf("stopping not running watcher should not error: %v", err)
	}
}

func TestTaskEventHandling(t *testing.T) {
	eventBus := newMockEventBus()

	var receivedData TaskEventData
	var received bool
	var mu sync.Mutex

	handlers := EventHandlers{
		OnTaskStateChanged: func(ctx context.Context, data TaskEventData) {
			mu.Lock()
			receivedData = data
			received = true
			mu.Unlock()
		},
	}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)
	_ = w.Start(context.Background())
	defer w.Stop()

	// Simulate publishing a task state changed event
	oldState := v1.TaskStateTODO
	newState := v1.TaskStateInProgress
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]interface{}{
		"task_id":   "task-123",
		"old_state": string(oldState),
		"new_state": string(newState),
	})

	_ = eventBus.Publish(context.Background(), events.TaskStateChanged, event)

	// Wait for handler to be called
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Error("OnTaskStateChanged handler was not called")
	}
	if receivedData.TaskID != "task-123" {
		t.Errorf("expected task_id = 'task-123', got %s", receivedData.TaskID)
	}
}

func TestAgentEventHandling(t *testing.T) {
	eventBus := newMockEventBus()

	var receivedData AgentEventData
	var received bool
	var mu sync.Mutex

	handlers := EventHandlers{
		OnAgentCompleted: func(ctx context.Context, data AgentEventData) {
			mu.Lock()
			receivedData = data
			received = true
			mu.Unlock()
		},
	}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)
	_ = w.Start(context.Background())
	defer w.Stop()

	// Simulate publishing an agent completed event
	exitCode := 0
	event := bus.NewEvent(events.AgentCompleted, "test", map[string]interface{}{
		"task_id":           "task-123",
		"agent_instance_id": "agent-456",
		"agent_type":        "test-agent",
		"exit_code":         exitCode,
	})

	_ = eventBus.Publish(context.Background(), events.AgentCompleted, event)

	// Wait for handler to be called
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Error("OnAgentCompleted handler was not called")
	}
	if receivedData.TaskID != "task-123" {
		t.Errorf("expected task_id = 'task-123', got %s", receivedData.TaskID)
	}
	if receivedData.AgentInstanceID != "agent-456" {
		t.Errorf("expected agent_instance_id = 'agent-456', got %s", receivedData.AgentInstanceID)
	}
}

func TestACPMessageHandling(t *testing.T) {
	eventBus := newMockEventBus()

	var receivedMsg *protocol.Message
	var receivedTaskID string
	var received bool
	var mu sync.Mutex

	handlers := EventHandlers{
		OnACPMessage: func(ctx context.Context, taskID string, msg *protocol.Message) {
			mu.Lock()
			receivedTaskID = taskID
			receivedMsg = msg
			received = true
			mu.Unlock()
		},
	}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)
	_ = w.Start(context.Background())
	defer w.Stop()

	// Simulate publishing an ACP message
	event := bus.NewEvent(events.BuildACPSubject("task-789"), "test", map[string]interface{}{
		"type":      "progress",
		"task_id":   "task-789",
		"agent_id":  "agent-123",
		"timestamp": time.Now().Format(time.RFC3339),
		"data": map[string]interface{}{
			"progress": 50,
		},
	})

	_ = eventBus.Publish(context.Background(), events.BuildACPWildcardSubject(), event)

	// Wait for handler to be called
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Error("OnACPMessage handler was not called")
	}
	if receivedTaskID != "task-789" {
		t.Errorf("expected task_id = 'task-789', got %s", receivedTaskID)
	}
	if receivedMsg == nil {
		t.Error("received message should not be nil")
	}
}

func TestNoHandlersRegistered(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{} // No handlers registered
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)
	err := w.Start(context.Background())

	if err != nil {
		t.Fatalf("Start with no handlers should not error: %v", err)
	}

	// Should have no subscriptions since no handlers
	if len(w.subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions with no handlers, got %d", len(w.subscriptions))
	}

	_ = w.Stop()
}

func TestPartialHandlers(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{
		OnTaskCreated: func(ctx context.Context, data TaskEventData) {},
		OnAgentFailed: func(ctx context.Context, data AgentEventData) {},
	}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)
	err := w.Start(context.Background())

	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should only subscribe to the handlers that were provided
	if len(w.subscriptions) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(w.subscriptions))
	}

	_ = w.Stop()
}

func TestIsRunning(t *testing.T) {
	eventBus := newMockEventBus()
	handlers := EventHandlers{}
	log := createTestLogger()

	w := NewWatcher(eventBus, handlers, log)

	if w.IsRunning() {
		t.Error("watcher should not be running before Start")
	}

	_ = w.Start(context.Background())
	if !w.IsRunning() {
		t.Error("watcher should be running after Start")
	}

	_ = w.Stop()
	if w.IsRunning() {
		t.Error("watcher should not be running after Stop")
	}
}

