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

// PublishSessionUpdate publishes a session update to the event bus for WebSocket streaming.
func (p *EventPublisher) PublishSessionUpdate(execution *AgentExecution, update agentctl.SessionUpdate) {
	if p.eventBus == nil {
		return
	}

	// Build the update data - our SessionUpdate type marshals cleanly
	updateData := map[string]interface{}{
		"type": update.Type,
	}

	if update.SessionID != "" {
		updateData["session_id"] = update.SessionID
	}
	if update.Text != "" {
		updateData["text"] = update.Text
	}
	if update.ToolCallID != "" {
		updateData["tool_call_id"] = update.ToolCallID
	}
	if update.ToolName != "" {
		updateData["tool_name"] = update.ToolName
	}
	if update.ToolTitle != "" {
		updateData["tool_title"] = update.ToolTitle
	}
	if update.ToolStatus != "" {
		updateData["tool_status"] = update.ToolStatus
	}
	if update.ToolArgs != nil {
		updateData["tool_args"] = update.ToolArgs
	}
	if update.ToolResult != nil {
		updateData["tool_result"] = update.ToolResult
	}
	if update.Error != "" {
		updateData["error"] = update.Error
	}
	if update.Data != nil {
		updateData["data"] = update.Data
	}

	// Build ACP message data
	data := map[string]interface{}{
		"type":       "session/update",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":   execution.ID,
		"task_id":    execution.TaskID,
		"session_id": update.SessionID,
		"data":       updateData,
	}

	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(execution.TaskID)

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish session update",
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

	data := map[string]interface{}{
		"task_id":       execution.TaskID,
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

	data := map[string]interface{}{
		"task_id":   execution.TaskID,
		"agent_id":  execution.ID,
		"path":      notification.Path,
		"operation": notification.Operation,
		"timestamp": notification.Timestamp.Format(time.RFC3339Nano),
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

	data := map[string]interface{}{
		"type":          "prompt_complete",
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":      execution.ID,
		"task_id":       execution.TaskID,
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

	data := map[string]interface{}{
		"type":         "tool_call",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     execution.ID,
		"task_id":      execution.TaskID,
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

// PublishToolCallComplete publishes a tool call completion event from a SessionUpdate.
func (p *EventPublisher) PublishToolCallComplete(execution *AgentExecution, update agentctl.SessionUpdate) {
	if p.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"type":         "tool_call_complete",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     execution.ID,
		"task_id":      execution.TaskID,
		"tool_call_id": update.ToolCallID,
		"status":       update.ToolStatus,
	}

	event := bus.NewEvent(events.ToolCallComplete, "agent-manager", data)
	subject := events.ToolCallComplete + "." + execution.TaskID

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish tool_call_complete event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	}
}

// PublishPermissionRequest publishes a permission request event.
// The event is structured as a protocol.Message so it can be parsed by the watcher.
func (p *EventPublisher) PublishPermissionRequest(execution *AgentExecution, notification *agentctl.PermissionNotification) {
	if p.eventBus == nil {
		return
	}

	// Convert options to a serializable format
	options := make([]map[string]interface{}, len(notification.Options))
	for i, opt := range notification.Options {
		options[i] = map[string]interface{}{
			"option_id": opt.OptionID,
			"name":      opt.Name,
			"kind":      opt.Kind,
		}
	}

	// Build payload data (goes into msg.Data when parsed as protocol.Message)
	payloadData := map[string]interface{}{
		"pending_id":     notification.PendingID,
		"session_id":     notification.SessionID,
		"tool_call_id":   notification.ToolCallID,
		"title":          notification.Title,
		"options":        options,
		"action_type":    notification.ActionType,
		"action_details": notification.ActionDetails,
		"created_at":     notification.CreatedAt,
	}

	// Structure as protocol.Message so watcher can parse it correctly
	data := map[string]interface{}{
		"type":      "permission_request",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":  execution.ID,
		"task_id":   execution.TaskID,
		"data":      payloadData,
	}

	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(execution.TaskID)

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish permission_request event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.Error(err))
	} else {
		p.logger.Debug("published permission_request event",
			zap.String("task_id", execution.TaskID),
			zap.String("pending_id", notification.PendingID),
			zap.String("title", notification.Title))
	}
}
