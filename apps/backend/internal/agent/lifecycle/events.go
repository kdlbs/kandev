// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// EventPublisher handles publishing agent lifecycle and session events to the event bus.
type EventPublisher struct {
	eventBus bus.EventBus
	logger   *logger.Logger
}

// NewEventPublisher creates a new EventPublisher with the given event bus and logger.
func NewEventPublisher(eventBus bus.EventBus, log *logger.Logger) *EventPublisher {
	return &EventPublisher{
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "event-publisher")),
	}
}

// PublishAgentEvent publishes an agent lifecycle event (started, stopped, ready, completed, failed).
func (p *EventPublisher) PublishAgentEvent(ctx context.Context, eventType string, execution *AgentExecution) {
	if p.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"instance_id":      execution.ID,
		"task_id":          execution.TaskID,
		"agent_profile_id": execution.AgentProfileID,
		"container_id":     execution.ContainerID,
		"status":           string(execution.Status),
		"started_at":       execution.StartedAt,
		"progress":         execution.Progress,
		"error_message":    execution.ErrorMessage,
	}

	if execution.FinishedAt != nil {
		data["finished_at"] = *execution.FinishedAt
	}
	if execution.ExitCode != nil {
		data["exit_code"] = *execution.ExitCode
	}

	event := bus.NewEvent(eventType, "agent-manager", data)

	if err := p.eventBus.Publish(ctx, eventType, event); err != nil {
		p.logger.Error("failed to publish event",
			zap.String("event_type", eventType),
			zap.String("instance_id", execution.ID),
			zap.Error(err))
	} else {
		p.logger.Debug("published agent event",
			zap.String("event_type", eventType),
			zap.String("instance_id", execution.ID))
	}
}

// PublishAgentctlEvent publishes an agentctl lifecycle event (starting, ready, error).
func (p *EventPublisher) PublishAgentctlEvent(ctx context.Context, eventType string, execution *AgentExecution, errMsg string) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	data := map[string]interface{}{
		"task_id":            execution.TaskID,
		"session_id":         sessionID,
		"agent_execution_id": execution.ID,
	}

	if errMsg != "" {
		data["error_message"] = errMsg
	}

	event := bus.NewEvent(eventType, "agent-manager", data)
	if err := p.eventBus.Publish(ctx, eventType, event); err != nil {
		p.logger.Error("failed to publish agentctl event",
			zap.String("event_type", eventType),
			zap.String("instance_id", execution.ID),
			zap.Error(err))
	}
}

// PublishACPSessionCreated publishes an event when an ACP session is created.
func (p *EventPublisher) PublishACPSessionCreated(execution *AgentExecution, sessionID string) {
	if p.eventBus == nil || sessionID == "" {
		return
	}

	data := map[string]interface{}{
		"task_id":           execution.TaskID,
		"agent_instance_id": execution.ID,
		"acp_session_id":    sessionID,
	}

	event := bus.NewEvent(events.AgentACPSessionCreated, "agent-manager", data)
	if err := p.eventBus.Publish(context.Background(), events.AgentACPSessionCreated, event); err != nil {
		p.logger.Error("failed to publish ACP session event",
			zap.String("event_type", events.AgentACPSessionCreated),
			zap.String("instance_id", execution.ID),
			zap.Error(err))
	}
}

// PublishAgentStreamEvent publishes an agent stream event to the event bus for WebSocket streaming.
// This is different from PublishAgentEvent which publishes lifecycle events (started, stopped, etc.).
func (p *EventPublisher) PublishAgentStreamEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	if p.eventBus == nil {
		return
	}

	// Build the event data - our AgentEvent type marshals cleanly
	eventData := map[string]interface{}{
		"type": event.Type,
	}

	if event.SessionID != "" {
		eventData["session_id"] = event.SessionID
	}
	if event.Text != "" {
		eventData["text"] = event.Text
	}
	if event.ToolCallID != "" {
		eventData["tool_call_id"] = event.ToolCallID
	}
	if event.ToolName != "" {
		eventData["tool_name"] = event.ToolName
	}
	if event.ToolTitle != "" {
		eventData["tool_title"] = event.ToolTitle
	}
	if event.ToolStatus != "" {
		eventData["tool_status"] = event.ToolStatus
	}
	if event.ToolArgs != nil {
		eventData["tool_args"] = event.ToolArgs
	}
	if event.ToolResult != nil {
		eventData["tool_result"] = event.ToolResult
	}
	if event.Error != "" {
		eventData["error"] = event.Error
	}
	if event.Data != nil {
		eventData["data"] = event.Data
	}

	// Build agent event message data
	data := map[string]interface{}{
		"type":       "agent/event",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":   execution.ID,
		"task_id":    execution.TaskID,
		"session_id": event.SessionID,
		"data":       eventData,
	}

	busEvent := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(execution.TaskID)

	if err := p.eventBus.Publish(context.Background(), subject, busEvent); err != nil {
		p.logger.Error("failed to publish agent stream event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	}
}

// PublishGitStatus publishes a git status update to the event bus.
func (p *EventPublisher) PublishGitStatus(execution *AgentExecution, update *agentctl.GitStatusUpdate) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	data := map[string]interface{}{
		"task_id":       execution.TaskID,
		"session_id":    sessionID,
		"agent_id":      execution.ID,
		"branch":        update.Branch,
		"remote_branch": update.RemoteBranch,
		"modified":      update.Modified,
		"added":         update.Added,
		"deleted":       update.Deleted,
		"untracked":     update.Untracked,
		"renamed":       update.Renamed,
		"ahead":         update.Ahead,
		"behind":        update.Behind,
		"files":         update.Files,
		"timestamp":     update.Timestamp.Format(time.RFC3339Nano),
	}

	event := bus.NewEvent(events.GitStatusUpdated, "agent-manager", data)
	subject := events.BuildGitStatusSubject(execution.TaskID)

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish git status event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	}
}

// PublishFileChange publishes a file change notification to the event bus.
func (p *EventPublisher) PublishFileChange(execution *AgentExecution, notification *agentctl.FileChangeNotification) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	data := map[string]interface{}{
		"task_id":    execution.TaskID,
		"session_id": sessionID,
		"agent_id":   execution.ID,
		"path":       notification.Path,
		"operation":  notification.Operation,
		"timestamp":  notification.Timestamp.Format(time.RFC3339Nano),
	}

	event := bus.NewEvent(events.FileChangeNotified, "agent-manager", data)
	subject := events.BuildFileChangeSubject(execution.TaskID)

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish file change event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	}
}

// PublishPromptComplete publishes a prompt_complete event when an agent finishes responding.
// This is used to save the agent's response as a comment on the task.
func (p *EventPublisher) PublishPromptComplete(execution *AgentExecution, agentMessage, reasoning, summary string) {
	if p.eventBus == nil {
		return
	}

	// Only publish if there's actual content
	if agentMessage == "" {
		return
	}

	sessionID := execution.SessionID

	data := map[string]interface{}{
		"type":          "prompt_complete",
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":      execution.ID,
		"task_id":       execution.TaskID,
		"session_id":    sessionID,
		"agent_message": agentMessage,
	}
	if reasoning != "" {
		data["reasoning"] = reasoning
	}
	if summary != "" {
		data["summary"] = summary
	}

	event := bus.NewEvent(events.PromptComplete, "agent-manager", data)
	// Publish on task-specific subject so orchestrator can subscribe
	subject := events.PromptComplete + "." + execution.TaskID

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish prompt_complete event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	}
}

// PublishToolCall publishes a tool call start event (to be saved as a comment).
func (p *EventPublisher) PublishToolCall(execution *AgentExecution, toolCallID, title, status string, args map[string]interface{}) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	data := map[string]interface{}{
		"type":         "tool_call",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     execution.ID,
		"task_id":      execution.TaskID,
		"session_id":   sessionID,
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	if args != nil {
		data["args"] = args
	}

	event := bus.NewEvent(events.ToolCallStarted, "agent-manager", data)
	subject := events.ToolCallStarted + "." + execution.TaskID

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish tool_call event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	} else {
		p.logger.Debug("published tool_call event",
			zap.String("task_id", execution.TaskID),
			zap.String("tool_call_id", toolCallID),
			zap.String("title", title))
	}
}

// PublishToolCallComplete publishes a tool call completion event from an AgentEvent.
func (p *EventPublisher) PublishToolCallComplete(execution *AgentExecution, event agentctl.AgentEvent) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	data := map[string]interface{}{
		"type":         "tool_call_complete",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     execution.ID,
		"task_id":      execution.TaskID,
		"session_id":   sessionID,
		"tool_call_id": event.ToolCallID,
		"status":       event.ToolStatus,
	}

	busEvent := bus.NewEvent(events.ToolCallComplete, "agent-manager", data)
	subject := events.ToolCallComplete + "." + execution.TaskID

	if err := p.eventBus.Publish(context.Background(), subject, busEvent); err != nil {
		p.logger.Error("failed to publish tool_call_complete event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	}
}

// PublishPermissionRequest publishes a permission request event to the event bus.
func (p *EventPublisher) PublishPermissionRequest(execution *AgentExecution, event agentctl.AgentEvent) {
	if p.eventBus == nil {
		return
	}

	// Convert options to a serializable format
	options := make([]map[string]interface{}, len(event.PermissionOptions))
	for i, opt := range event.PermissionOptions {
		options[i] = map[string]interface{}{
			"option_id": opt.OptionID,
			"name":      opt.Name,
			"kind":      opt.Kind,
		}
	}

	data := map[string]interface{}{
		"type":           "permission_request",
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":       execution.ID,
		"task_id":        execution.TaskID,
		"session_id":     execution.SessionID,
		"pending_id":     event.PendingID,
		"tool_call_id":   event.ToolCallID,
		"title":          event.PermissionTitle,
		"options":        options,
		"action_type":    event.ActionType,
		"action_details": event.ActionDetails,
	}

	busEvent := bus.NewEvent(events.PermissionRequestReceived, "agent-manager", data)
	subject := events.PermissionRequestReceived + "." + execution.TaskID

	if err := p.eventBus.Publish(context.Background(), subject, busEvent); err != nil {
		p.logger.Error("failed to publish permission_request event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	} else {
		p.logger.Debug("published permission_request event",
			zap.String("task_id", execution.TaskID),
			zap.String("pending_id", event.PendingID),
			zap.String("title", event.PermissionTitle))
	}
}
