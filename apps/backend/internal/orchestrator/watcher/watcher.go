// Package watcher provides event subscription and dispatching for the Orchestrator.
package watcher

import (
	"context"
	"encoding/json"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/pkg/acp/protocol"
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
	AgentExecutionID string `json:"agent_execution_id"`
	AgentProfileID   string `json:"agent_profile_id"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// ACPSessionEventData contains data from ACP session events
type ACPSessionEventData struct {
	TaskID           string `json:"task_id"`
	AgentExecutionID string `json:"agent_execution_id"`
	ACPSessionID     string `json:"acp_session_id"`
}

// PromptCompleteData contains data from prompt_complete events
type PromptCompleteData struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
	AgentID       string `json:"agent_id"`
	AgentMessage  string `json:"agent_message"`
}

// ToolCallData contains data from tool_call events
type ToolCallData struct {
	TaskID        string                 `json:"task_id"`
	TaskSessionID string                 `json:"session_id"`
	AgentID       string                 `json:"agent_id"`
	ToolCallID    string                 `json:"tool_call_id"`
	Title         string                 `json:"title"`
	Status        string                 `json:"status"`
	Args          map[string]interface{} `json:"args,omitempty"` // Tool call arguments (kind, path, locations, raw_input)
}

// ToolCallCompleteData contains data from tool_call_complete events
type ToolCallCompleteData struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
	AgentID       string `json:"agent_id"`
	ToolCallID    string `json:"tool_call_id"`
	Status        string `json:"status"` // "complete" or "error"
	Result        string `json:"result,omitempty"`
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

// GitStatusData contains data from git status events
type GitStatusData struct {
	TaskID        string                 `json:"task_id"`
	TaskSessionID string                 `json:"session_id"`
	AgentID       string                 `json:"agent_id"`
	Branch        string                 `json:"branch"`
	RemoteBranch  string                 `json:"remote_branch"`
	Modified      []string               `json:"modified"`
	Added         []string               `json:"added"`
	Deleted       []string               `json:"deleted"`
	Untracked     []string               `json:"untracked"`
	Renamed       []string               `json:"renamed"`
	Ahead         int                    `json:"ahead"`
	Behind        int                    `json:"behind"`
	Files         map[string]interface{} `json:"files"`
	Timestamp     string                 `json:"timestamp"`
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
	OnAgentReady        func(ctx context.Context, data AgentEventData)
	OnAgentCompleted    func(ctx context.Context, data AgentEventData)
	OnAgentFailed       func(ctx context.Context, data AgentEventData)
	OnAgentStopped      func(ctx context.Context, data AgentEventData)
	OnACPSessionCreated func(ctx context.Context, data ACPSessionEventData)

	// ACP messages
	OnACPMessage func(ctx context.Context, taskID string, msg *protocol.Message)

	// Prompt events
	OnPromptComplete func(ctx context.Context, data PromptCompleteData)

	// Tool call events
	OnToolCallStarted  func(ctx context.Context, data ToolCallData)
	OnToolCallComplete func(ctx context.Context, data ToolCallCompleteData)

	// Permission request events
	OnPermissionRequest func(ctx context.Context, data PermissionRequestData)

	// Git status events
	OnGitStatusUpdated func(ctx context.Context, data GitStatusData)
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

	// Subscribe to ACP messages
	if err := w.subscribeToACPMessages(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to prompt complete events
	if err := w.subscribeToPromptCompleteEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to tool call events
	if err := w.subscribeToToolCallEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to permission request events
	if err := w.subscribeToPermissionRequestEvents(); err != nil {
		w.unsubscribeAll()
		return err
	}

	// Subscribe to git status events
	if err := w.subscribeToGitStatusEvents(); err != nil {
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

// subscribeToACPMessages subscribes to ACP messages using wildcard
func (w *Watcher) subscribeToACPMessages() error {
	if w.handlers.OnACPMessage == nil {
		return nil
	}

	// Use wildcard to subscribe to all ACP messages (acp.message.*)
	subject := events.BuildACPWildcardSubject()

	// Use regular subscription for ACP messages (each instance needs all messages for WebSocket streaming)
	sub, err := w.eventBus.Subscribe(subject, w.createACPMessageHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to ACP messages",
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

// createACPMessageHandler creates a bus.EventHandler for ACP messages
func (w *Watcher) createACPMessageHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		// Parse the ACP message from event data
		// Note: session_id in the payload is the task session ID (not ACP session ID)
		var msg protocol.Message
		if err := w.parseEventData(event.Data, &msg); err != nil {
			w.logger.Error("Failed to parse ACP message",
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		// Extract task ID from payload
		taskID := msg.TaskID
		if taskID == "" {
			w.logger.Warn("ACP message missing task_id",
				zap.String("event_id", event.ID))
			return nil
		}

		w.logger.Debug("Handling ACP message",
			zap.String("task_id", taskID),
			zap.String("session_id", msg.SessionID),
			zap.String("message_type", string(msg.Type)))

		w.handlers.OnACPMessage(ctx, taskID, &msg)
		return nil
	}
}

// subscribeToPromptCompleteEvents subscribes to prompt complete events
func (w *Watcher) subscribeToPromptCompleteEvents() error {
	if w.handlers.OnPromptComplete == nil {
		return nil
	}

	// Use wildcard to subscribe to all prompt complete events (prompt.complete.{session_id})
	subject := events.BuildPromptCompleteWildcardSubject()

	// Use regular subscription (each instance needs all messages for comment creation)
	sub, err := w.eventBus.Subscribe(subject, w.createPromptCompleteHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to prompt complete events",
			zap.String("subject", subject),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// createPromptCompleteHandler creates a bus.EventHandler for prompt complete events
func (w *Watcher) createPromptCompleteHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data PromptCompleteData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse prompt complete event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		w.logger.Debug("Handling prompt complete event",
			zap.String("task_id", data.TaskID),
			zap.Int("message_length", len(data.AgentMessage)))

		w.handlers.OnPromptComplete(ctx, data)
		return nil
	}
}

// subscribeToToolCallEvents subscribes to tool call events
func (w *Watcher) subscribeToToolCallEvents() error {
	// Subscribe to tool call started events (tool_call.started.{session_id})
	if w.handlers.OnToolCallStarted != nil {
		subject := events.BuildToolCallStartedWildcardSubject()
		sub, err := w.eventBus.Subscribe(subject, w.createToolCallStartedHandler())
		if err != nil {
			w.logger.Error("Failed to subscribe to tool call started events",
				zap.String("subject", subject),
				zap.Error(err))
			return err
		}
		w.subscriptions = append(w.subscriptions, sub)
	}

	// Subscribe to tool call complete events (tool_call.complete.{session_id})
	if w.handlers.OnToolCallComplete != nil {
		subject := events.BuildToolCallCompleteWildcardSubject()
		sub, err := w.eventBus.Subscribe(subject, w.createToolCallCompleteHandler())
		if err != nil {
			w.logger.Error("Failed to subscribe to tool call complete events",
				zap.String("subject", subject),
				zap.Error(err))
			return err
		}
		w.subscriptions = append(w.subscriptions, sub)
	}

	return nil
}

// createToolCallStartedHandler creates a bus.EventHandler for tool call started events
func (w *Watcher) createToolCallStartedHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data ToolCallData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse tool call event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		w.logger.Debug("Handling tool call started event",
			zap.String("task_id", data.TaskID),
			zap.String("tool_call_id", data.ToolCallID),
			zap.String("title", data.Title))

		w.handlers.OnToolCallStarted(ctx, data)
		return nil
	}
}

// createToolCallCompleteHandler creates a bus.EventHandler for tool call complete events
func (w *Watcher) createToolCallCompleteHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data ToolCallCompleteData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse tool call complete event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil
		}

		w.logger.Debug("Handling tool call complete event",
			zap.String("task_id", data.TaskID),
			zap.String("tool_call_id", data.ToolCallID),
			zap.String("status", data.Status))

		w.handlers.OnToolCallComplete(ctx, data)
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

// subscribeToGitStatusEvents subscribes to git status events
func (w *Watcher) subscribeToGitStatusEvents() error {
	if w.handlers.OnGitStatusUpdated == nil {
		return nil
	}

	// Use wildcard to subscribe to all git status events (git.status.updated.{session_id})
	subject := events.BuildGitStatusWildcardSubject()

	// Use regular subscription (each instance needs all messages)
	sub, err := w.eventBus.Subscribe(subject, w.createGitStatusHandler())
	if err != nil {
		w.logger.Error("Failed to subscribe to git status events",
			zap.String("subject", subject),
			zap.Error(err))
		return err
	}
	w.subscriptions = append(w.subscriptions, sub)
	return nil
}

// createGitStatusHandler creates a bus.EventHandler for git status events
func (w *Watcher) createGitStatusHandler() bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		var data GitStatusData
		if err := w.parseEventData(event.Data, &data); err != nil {
			w.logger.Error("Failed to parse git status event data",
				zap.String("event_type", event.Type),
				zap.String("event_id", event.ID),
				zap.Error(err))
			return nil // Don't return error to continue processing other events
		}

		w.logger.Debug("Handling git status event",
			zap.String("task_id", data.TaskID),
			zap.String("branch", data.Branch),
			zap.Int("modified", len(data.Modified)))

		w.handlers.OnGitStatusUpdated(ctx, data)
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
