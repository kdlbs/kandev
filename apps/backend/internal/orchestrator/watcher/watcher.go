// Package watcher provides event subscription and dispatching for the Orchestrator.
package watcher

import (
	"context"
	"encoding/json"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskEventData contains data from task events
type TaskEventData struct {
	TaskID   string        `json:"task_id"`
	Task     *v1.Task      `json:"task,omitempty"`
	OldState *v1.TaskState `json:"old_state,omitempty"`
	NewState *v1.TaskState `json:"new_state,omitempty"`
}

// AgentEventData contains data from agent events
type AgentEventData struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	AgentExecutionID string `json:"agent_execution_id"`
	AgentProfileID   string `json:"agent_profile_id"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// ACPSessionEventData contains data from ACP session events
type ACPSessionEventData struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	AgentExecutionID string `json:"agent_execution_id"`
	ACPSessionID     string `json:"acp_session_id"`
}

// PermissionRequestData contains data from permission_request events
type PermissionRequestData struct {
	TaskID        string                   `json:"task_id"`
	TaskSessionID string                   `json:"session_id"`
	AgentID       string                   `json:"agent_id"`
	PendingID     string                   `json:"pending_id"`
	ToolCallID    string                   `json:"tool_call_id"`
	Title         string                   `json:"title"`
	Options       []map[string]interface{} `json:"options"`
	ActionType    string                   `json:"action_type"`
	ActionDetails map[string]interface{}   `json:"action_details"`
}

// GitEventData is an alias for lifecycle.GitEventPayload.
// Kept for backwards compatibility with existing handler signatures.
type GitEventData = lifecycle.GitEventPayload

// ContextWindowData contains data from context window events
type ContextWindowData struct {
	TaskID                 string  `json:"task_id"`
	TaskSessionID          string  `json:"session_id"`
	AgentID                string  `json:"agent_id"`
	ContextWindowSize      int64   `json:"context_window_size"`
	ContextWindowUsed      int64   `json:"context_window_used"`
	ContextWindowRemaining int64   `json:"context_window_remaining"`
	ContextEfficiency      float64 `json:"context_efficiency"`
	Timestamp              string  `json:"timestamp"`
}

// EventHandlers contains callbacks for different event types
type EventHandlers struct {
	// Task events
	OnTaskCreated      func(ctx context.Context, data TaskEventData)
	OnTaskUpdated      func(ctx context.Context, data TaskEventData)
	OnTaskStateChanged func(ctx context.Context, data TaskEventData)
	OnTaskDeleted      func(ctx context.Context, data TaskEventData)

	// Agent events
	OnAgentStarted      func(ctx context.Context, data AgentEventData)
	OnAgentRunning      func(ctx context.Context, data AgentEventData)
	OnAgentReady        func(ctx context.Context, data AgentEventData)
	OnAgentCompleted    func(ctx context.Context, data AgentEventData)
	OnAgentFailed       func(ctx context.Context, data AgentEventData)
	OnAgentStopped      func(ctx context.Context, data AgentEventData)
	OnACPSessionCreated func(ctx context.Context, data ACPSessionEventData)

	// Agent stream events (tool calls, message chunks, complete, etc.)
	OnAgentStreamEvent func(ctx context.Context, payload *lifecycle.AgentStreamEventPayload)

	// Permission request events
	OnPermissionRequest func(ctx context.Context, data PermissionRequestData)

	// Unified git events (status, commit, reset, snapshot)
	OnGitEvent func(ctx context.Context, data GitEventData)

	// Context window events
	OnContextWindowUpdated func(ctx context.Context, data ContextWindowData)
}

// Watcher subscribes to events and dispatches to handlers
type Watcher struct {
	eventBus bus.EventBus
	handlers EventHandlers
	logger   *logger.Logger

	subscriptions []bus.Subscription
	mu            sync.Mutex
	running       bool
}

// queueName is the queue group for load balancing across orchestrator instances
const queueName = "orchestrator"

// NewWatcher creates a new event watcher
func NewWatcher(eventBus bus.EventBus, handlers EventHandlers, log *logger.Logger) *Watcher {
	return &Watcher{
		eventBus:      eventBus,
		handlers:      handlers,
		logger:        log.WithFields(zap.String("component", "watcher")),
		subscriptions: make([]bus.Subscription, 0),
	}
}

// Start begins watching for events
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return nil
	}

	w.logger.Info("Starting event watcher")

	// Subscribe to task events
	if err := w.subscribeToTaskEvents(); err != nil {
		return err
	}

	// Subscribe to agent events
	if err := w.subscribeToAgentEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to ACP session events
	if err := w.subscribeToACPSessionEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to agent stream events
	if err := w.subscribeToAgentStreamEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to permission request events
	if err := w.subscribeToPermissionRequestEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to unified git events
	if err := w.subscribeToGitEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to context window events
	if err := w.subscribeToContextWindowEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	w.running = true
	w.logger.Info("Event watcher started", zap.Int("subscriptions", len(w.subscriptions)))
	return nil
}

// Stop stops watching for events
func (w *Watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	w.logger.Info("Stopping event watcher")
	w.unsubscribeAll()
	w.running = false
	w.logger.Info("Event watcher stopped")
	return nil
}

// IsRunning returns true if the watcher is active
func (w *Watcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// unsubscribeAll removes all subscriptions (must be called with lock held)
func (w *Watcher) unsubscribeAll() {
	for _, sub := range w.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			w.logger.Error("Failed to unsubscribe", zap.Error(err))
		}
	}
	w.subscriptions = make([]bus.Subscription, 0)
}

// subscribeToTaskEvents subscribes to all task events
func (w *Watcher) subscribeToTaskEvents() error {
	taskEvents := []struct {
		subject string
		handler func(ctx context.Context, data TaskEventData)
	}{
		{events.TaskCreated, w.handlers.OnTaskCreated},
		{events.TaskUpdated, w.handlers.OnTaskUpdated},
		{events.TaskStateChanged, w.handlers.OnTaskStateChanged},
		{events.TaskDeleted, w.handlers.OnTaskDeleted},
	}

	for _, te := range taskEvents {
		if te.handler == nil {
			continue
		}
		handler := te.handler // capture for closure
		sub, err := w.eventBus.QueueSubscribe(te.subject, queueName, w.createTaskEventHandler(handler))
		if err != nil {
			w.logger.Error("Failed to subscribe to task event",
				zap.String("subject", te.subject),
				zap.Error(err))
			return err
		}
		w.subscriptions = append(w.subscriptions, sub)
	}
	return nil
}

// subscribeToAgentEvents subscribes to all agent events
func (w *Watcher) subscribeToAgentEvents() error {
	agentEvents := []struct {
		subject string
		handler func(ctx context.Context, data AgentEventData)
	}{
		{events.AgentStarted, w.handlers.OnAgentStarted},
		{events.AgentRunning, w.handlers.OnAgentRunning},
		{events.AgentReady, w.handlers.OnAgentReady},
		{events.AgentCompleted, w.handlers.OnAgentCompleted},
		{events.AgentFailed, w.handlers.OnAgentFailed},
		{events.AgentStopped, w.handlers.OnAgentStopped},
	}

	for _, ae := range agentEvents {
		if ae.handler == nil {
			continue
		}
		handler := ae.handler // capture for closure
		sub, err := w.eventBus.QueueSubscribe(ae.subject, queueName, w.createAgentEventHandler(handler))
		if err != nil {
			w.logger.Error("Failed to subscribe to agent event",
				zap.String("subject", ae.subject),
				zap.Error(err))
			return err
		}
		w.subscriptions = append(w.subscriptions, sub)
	}
	return nil
}

// subscribeToACPSessionEvents subscribes to ACP session lifecycle events
func (w *Watcher) subscribeToACPSessionEvents() error {
	if w.handlers.OnACPSessionCreated == nil {
		return nil
	}

	sub, err := w.eventBus.QueueSubscribe(events.AgentACPSessionCreated, queueName, w.createACPSessionEventHandler(w.handlers.OnACPSessionCreated))
	if err != nil {
		w.logger.Error("Failed to subscribe to ACP session event",
			zap.String("subject", events.AgentACPSessionCreated),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// subscribeToAgentStreamEvents subscribes to agent stream events using wildcard
func (w *Watcher) subscribeToAgentStreamEvents() error {
	if w.handlers.OnAgentStreamEvent == nil {
		return nil
	}

	// Use wildcard to subscribe to all agent stream events (agent.stream.*)
	subject := events.BuildAgentStreamWildcardSubject()

	// Use regular subscription (each instance needs all messages for WebSocket streaming)
	sub, err := w.eventBus.Subscribe(subject, w.createAgentStreamEventHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to agent stream events",
			zap.String("subject", subject),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// createTaskEventHandler creates a bus.EventHandler for task events
func (w *Watcher) createTaskEventHandler(handler func(ctx context.Context, data TaskEventData)) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data TaskEventData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse task event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		w.logger.Debug("Handling task event",
			zap.String("event_type", event.Type),
			zap.String("task_id", data.TaskID))

		handler(ctx, data)
		return nil
	}
}

// createAgentEventHandler creates a bus.EventHandler for agent events
func (w *Watcher) createAgentEventHandler(handler func(ctx context.Context, data AgentEventData)) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data AgentEventData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse agent event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		w.logger.Debug("Handling agent event",
			zap.String("event_type", event.Type),
			zap.String("task_id", data.TaskID),
			zap.String("agent_execution_id", data.AgentExecutionID))

		handler(ctx, data)
		return nil
	}
}

// createACPSessionEventHandler creates a bus.EventHandler for ACP session events
func (w *Watcher) createACPSessionEventHandler(handler func(ctx context.Context, data ACPSessionEventData)) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data ACPSessionEventData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse ACP session event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil
		}

		w.logger.Debug("Handling ACP session event",
			zap.String("event_type", event.Type),
			zap.String("task_id", data.TaskID),
			zap.String("acp_session_id", data.ACPSessionID))

		handler(ctx, data)
		return nil
	}
}

// createAgentStreamEventHandler creates a bus.EventHandler for agent stream events
func (w *Watcher) createAgentStreamEventHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		// Parse the agent stream event payload
		var payload lifecycle.AgentStreamEventPayload
		if err := w.parseEventData(event.Data, &payload); err != nil {
			w.logger.Error("Failed to parse agent stream event",
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		// Validate required fields
		if payload.TaskID == "" {
			w.logger.Warn("Agent stream event missing task_id",
				zap.String("event_id", event.ID))
			return nil
		}

		eventType := ""
		if payload.Data != nil {
			eventType = payload.Data.Type
		}

		w.logger.Debug("Handling agent stream event",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("event_type", eventType))

		w.handlers.OnAgentStreamEvent(ctx, &payload)
		return nil
	}
}

// subscribeToPermissionRequestEvents subscribes to permission request events
func (w *Watcher) subscribeToPermissionRequestEvents() error {
	if w.handlers.OnPermissionRequest == nil {
		return nil
	}

	// Use wildcard to subscribe to all permission request events (permission_request.received.{session_id})
	subject := events.BuildPermissionRequestWildcardSubject()
	sub, err := w.eventBus.Subscribe(subject, w.createPermissionRequestHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to permission request events",
			zap.String("subject", subject),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// createPermissionRequestHandler creates a bus.EventHandler for permission request events
func (w *Watcher) createPermissionRequestHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data PermissionRequestData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse permission request event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil
		}

		w.logger.Debug("Handling permission request event",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID),
			zap.String("title", data.Title))

		w.handlers.OnPermissionRequest(ctx, data)
		return nil
	}
}

// subscribeToGitEvents subscribes to unified git events
func (w *Watcher) subscribeToGitEvents() error {
	if w.handlers.OnGitEvent == nil {
		return nil
	}

	subject := events.BuildGitEventWildcardSubject()

	sub, err := w.eventBus.Subscribe(subject, w.createGitEventHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to git events",
			zap.String("subject", subject),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// createGitEventHandler creates a bus.EventHandler for unified git events
func (w *Watcher) createGitEventHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data GitEventData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse git event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil
		}

		w.logger.Debug("Handling git event",
			zap.String("type", string(data.Type)),
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))

		w.handlers.OnGitEvent(ctx, data)
		return nil
	}
}

// subscribeToContextWindowEvents subscribes to context window events
func (w *Watcher) subscribeToContextWindowEvents() error {
	if w.handlers.OnContextWindowUpdated == nil {
		return nil
	}

	// Use wildcard to subscribe to all context window events (context_window.updated.{session_id})
	subject := events.BuildContextWindowWildcardSubject()

	// Use regular subscription (each instance needs all messages)
	sub, err := w.eventBus.Subscribe(subject, w.createContextWindowHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to context window events",
			zap.String("subject", subject),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// createContextWindowHandler creates a bus.EventHandler for context window events
func (w *Watcher) createContextWindowHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data ContextWindowData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse context window event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		w.logger.Debug("Handling context window event",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.TaskSessionID),
			zap.Int64("size", data.ContextWindowSize),
			zap.Int64("used", data.ContextWindowUsed))

		w.handlers.OnContextWindowUpdated(ctx, data)
		return nil
	}
}

// parseEventData converts event data (map or struct) to a typed struct
func (w *Watcher) parseEventData(data interface{}, target interface{}) error {
	// Marshal to JSON and unmarshal to target type
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonData, target)
}
