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

	payload := AgentEventPayload{
		InstanceID:     execution.ID,
		TaskID:         execution.TaskID,
		AgentProfileID: execution.AgentProfileID,
		ContainerID:    execution.ContainerID,
		Status:         string(execution.Status),
		StartedAt:      execution.StartedAt,
		FinishedAt:     execution.FinishedAt,
		Progress:       execution.Progress,
		ErrorMessage:   execution.ErrorMessage,
		ExitCode:       execution.ExitCode,
	}

	event := bus.NewEvent(eventType, "agent-manager", payload)

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

	payload := AgentctlEventPayload{
		TaskID:           execution.TaskID,
		SessionID:        execution.SessionID,
		AgentExecutionID: execution.ID,
		ErrorMessage:     errMsg,
	}

	event := bus.NewEvent(eventType, "agent-manager", payload)
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

	payload := ACPSessionCreatedPayload{
		TaskID:          execution.TaskID,
		AgentInstanceID: execution.ID,
		ACPSessionID:    sessionID,
	}

	event := bus.NewEvent(events.AgentACPSessionCreated, "agent-manager", payload)
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

	// Build the nested event data
	// event.SessionID is the ACP session ID (internal agent protocol session)
	eventData := &AgentStreamEventData{
		Type:          event.Type,
		ACPSessionID:  event.SessionID,
		Text:          event.Text,
		ToolCallID:    event.ToolCallID,
		ToolName:      event.ToolName,
		ToolTitle:     event.ToolTitle,
		ToolStatus:    event.ToolStatus,
		ToolArgs:      event.ToolArgs,
		ToolResult:    event.ToolResult,
		Error:         event.Error,
		SessionStatus: event.SessionStatus,
		Data:          event.Data,
	}

	// Build agent event message payload
	// session_id is the task session ID (execution.SessionID)
	// acp_session_id in eventData is the internal agent protocol session
	payload := AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:   execution.ID,
		TaskID:    execution.TaskID,
		SessionID: execution.SessionID,
		Data:      eventData,
	}

	busEvent := bus.NewEvent(events.AgentStream, "agent-manager", payload)
	subject := events.BuildAgentStreamSubject(execution.SessionID)

	if err := p.eventBus.Publish(context.Background(), subject, busEvent); err != nil {
		p.logger.Error("failed to publish agent stream event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.String("session_id", execution.SessionID),
			zap.Error(err))
	}
}

// PublishGitStatus publishes a git status update to the event bus.
func (p *EventPublisher) PublishGitStatus(execution *AgentExecution, update *agentctl.GitStatusUpdate) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	payload := GitStatusEventPayload{
		TaskID:       execution.TaskID,
		SessionID:    sessionID,
		AgentID:      execution.ID,
		Branch:       update.Branch,
		RemoteBranch: update.RemoteBranch,
		Modified:     update.Modified,
		Added:        update.Added,
		Deleted:      update.Deleted,
		Untracked:    update.Untracked,
		Renamed:      update.Renamed,
		Ahead:        update.Ahead,
		Behind:       update.Behind,
		Files:        update.Files,
		Timestamp:    update.Timestamp.Format(time.RFC3339Nano),
	}

	event := bus.NewEvent(events.GitStatusUpdated, "agent-manager", payload)
	subject := events.BuildGitStatusSubject(sessionID)

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish git status event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// PublishFileChange publishes a file change notification to the event bus.
func (p *EventPublisher) PublishFileChange(execution *AgentExecution, notification *agentctl.FileChangeNotification) {
	if p.eventBus == nil {
		return
	}

	sessionID := execution.SessionID

	payload := FileChangeEventPayload{
		TaskID:    execution.TaskID,
		SessionID: sessionID,
		AgentID:   execution.ID,
		Path:      notification.Path,
		Operation: notification.Operation,
		Timestamp: notification.Timestamp.Format(time.RFC3339Nano),
	}

	event := bus.NewEvent(events.FileChangeNotified, "agent-manager", payload)
	subject := events.BuildFileChangeSubject(sessionID)

	if err := p.eventBus.Publish(context.Background(), subject, event); err != nil {
		p.logger.Error("failed to publish file change event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// PublishPermissionRequest publishes a permission request event to the event bus.
func (p *EventPublisher) PublishPermissionRequest(execution *AgentExecution, event agentctl.AgentEvent) {
	if p.eventBus == nil {
		return
	}

	// Convert options to typed format
	options := make([]PermissionOption, len(event.PermissionOptions))
	for i, opt := range event.PermissionOptions {
		options[i] = PermissionOption{
			OptionID: opt.OptionID,
			Name:     opt.Name,
			Kind:     opt.Kind,
		}
	}

	payload := PermissionRequestEventPayload{
		Type:          "permission_request",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:       execution.ID,
		TaskID:        execution.TaskID,
		SessionID:     execution.SessionID,
		PendingID:     event.PendingID,
		ToolCallID:    event.ToolCallID,
		Title:         event.PermissionTitle,
		Options:       options,
		ActionType:    event.ActionType,
		ActionDetails: event.ActionDetails,
	}

	busEvent := bus.NewEvent(events.PermissionRequestReceived, "agent-manager", payload)
	subject := events.BuildPermissionRequestSubject(execution.SessionID)

	if err := p.eventBus.Publish(context.Background(), subject, busEvent); err != nil {
		p.logger.Error("failed to publish permission_request event",
			zap.String("instance_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.String("session_id", execution.SessionID),
			zap.Error(err))
	} else {
		p.logger.Debug("published permission_request event",
			zap.String("task_id", execution.TaskID),
			zap.String("session_id", execution.SessionID),
			zap.String("pending_id", event.PendingID),
			zap.String("title", event.PermissionTitle))
	}
}
