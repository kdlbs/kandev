package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type TaskEventBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
}

func RegisterTaskNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *TaskEventBroadcaster {
	b := &TaskEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-task-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	b.subscribe(eventBus, events.WorkspaceCreated, ws.ActionWorkspaceCreated)
	b.subscribe(eventBus, events.WorkspaceUpdated, ws.ActionWorkspaceUpdated)
	b.subscribe(eventBus, events.WorkspaceDeleted, ws.ActionWorkspaceDeleted)
	b.subscribe(eventBus, events.WorkflowCreated, ws.ActionWorkflowCreated)
	b.subscribe(eventBus, events.WorkflowUpdated, ws.ActionWorkflowUpdated)
	b.subscribe(eventBus, events.WorkflowDeleted, ws.ActionWorkflowDeleted)
	b.subscribe(eventBus, events.WorkflowStepCreated, ws.ActionWorkflowStepCreated)
	b.subscribe(eventBus, events.WorkflowStepUpdated, ws.ActionWorkflowStepUpdated)
	b.subscribe(eventBus, events.WorkflowStepDeleted, ws.ActionWorkflowStepDeleted)
	b.subscribe(eventBus, events.AgentProfileCreated, ws.ActionAgentProfileCreated)
	b.subscribe(eventBus, events.AgentProfileUpdated, ws.ActionAgentProfileUpdated)
	b.subscribe(eventBus, events.AgentProfileDeleted, ws.ActionAgentProfileDeleted)
	b.subscribe(eventBus, events.TaskCreated, ws.ActionTaskCreated)
	b.subscribe(eventBus, events.TaskUpdated, ws.ActionTaskUpdated)
	b.subscribe(eventBus, events.SessionWorkspaceSourcesUpdated, ws.ActionSessionWorkspaceSourcesUpdated)
	b.subscribe(eventBus, events.TaskDeleted, ws.ActionTaskDeleted)
	b.subscribeLifecycleStateEvents(eventBus)
	b.subscribe(eventBus, events.TaskPlanCreated, ws.ActionTaskPlanCreated)
	b.subscribe(eventBus, events.TaskPlanUpdated, ws.ActionTaskPlanUpdated)
	b.subscribe(eventBus, events.TaskPlanDeleted, ws.ActionTaskPlanDeleted)
	b.subscribe(eventBus, events.TaskPlanRevisionCreated, ws.ActionTaskPlanRevisionCreated)
	b.subscribe(eventBus, events.TaskPlanReverted, ws.ActionTaskPlanReverted)
	b.subscribe(eventBus, events.TaskWalkthroughCreated, ws.ActionTaskWalkthroughCreated)
	b.subscribe(eventBus, events.TaskWalkthroughUpdated, ws.ActionTaskWalkthroughUpdated)
	b.subscribe(eventBus, events.TaskWalkthroughDeleted, ws.ActionTaskWalkthroughDeleted)
	b.subscribe(eventBus, events.TaskReviewRunUpdated, ws.ActionTaskReviewRunUpdated)
	b.subscribe(eventBus, events.TaskReviewFindingsPublished, ws.ActionTaskReviewFindingsPublished)
	b.subscribe(eventBus, events.TaskReviewFindingUpdated, ws.ActionTaskReviewFindingUpdated)
	b.subscribe(eventBus, events.TaskReviewCleared, ws.ActionTaskReviewCleared)
	b.subscribe(eventBus, events.RepositoryCreated, ws.ActionRepositoryCreated)
	b.subscribe(eventBus, events.RepositoryUpdated, ws.ActionRepositoryUpdated)
	b.subscribe(eventBus, events.RepositoryDeleted, ws.ActionRepositoryDeleted)
	b.subscribe(eventBus, events.RepositoryScriptCreated, ws.ActionRepositoryScriptCreated)
	b.subscribe(eventBus, events.RepositoryScriptUpdated, ws.ActionRepositoryScriptUpdated)
	b.subscribe(eventBus, events.RepositoryScriptDeleted, ws.ActionRepositoryScriptDeleted)
	b.subscribe(eventBus, events.ExecutorCreated, ws.ActionExecutorCreated)
	b.subscribe(eventBus, events.ExecutorUpdated, ws.ActionExecutorUpdated)
	b.subscribe(eventBus, events.ExecutorDeleted, ws.ActionExecutorDeleted)
	b.subscribe(eventBus, events.ExecutorProfileCreated, ws.ActionExecutorProfileCreated)
	b.subscribe(eventBus, events.ExecutorProfileUpdated, ws.ActionExecutorProfileUpdated)
	b.subscribe(eventBus, events.ExecutorProfileDeleted, ws.ActionExecutorProfileDeleted)
	b.subscribe(eventBus, events.ExecutorPrepareProgress, ws.ActionExecutorPrepareProgress)
	b.subscribe(eventBus, events.ExecutorPrepareCompleted, ws.ActionExecutorPrepareCompleted)
	b.subscribe(eventBus, events.EnvironmentCreated, ws.ActionEnvironmentCreated)
	b.subscribe(eventBus, events.EnvironmentUpdated, ws.ActionEnvironmentUpdated)
	b.subscribe(eventBus, events.EnvironmentDeleted, ws.ActionEnvironmentDeleted)
	b.subscribe(eventBus, events.TaskSessionActivityChanged, ws.ActionSessionActivityChanged)
	b.subscribe(eventBus, events.TaskStatusSummaryUpdated, ws.ActionTaskStatusSummaryUpdated)
	b.subscribe(eventBus, events.MessageAdded, ws.ActionSessionMessageAdded)
	b.subscribe(eventBus, events.MessageUpdated, ws.ActionSessionMessageUpdated)
	b.subscribe(eventBus, events.MessageDeleted, ws.ActionSessionMessageDeleted)
	b.subscribe(eventBus, events.AgentctlStarting, ws.ActionSessionAgentctlStarting)
	b.subscribe(eventBus, events.AgentctlReady, ws.ActionSessionAgentctlReady)
	b.subscribe(eventBus, events.AgentctlError, ws.ActionSessionAgentctlError)
	b.subscribe(eventBus, events.TurnStarted, ws.ActionSessionTurnStarted)
	b.subscribe(eventBus, events.TurnCompleted, ws.ActionSessionTurnCompleted)
	b.subscribe(eventBus, events.MessageQueueStatusChanged, ws.ActionMessageQueueStatusChanged)
	b.subscribe(eventBus, events.GitHubTaskPRUpdated, ws.ActionGitHubTaskPRUpdated)
	b.subscribe(eventBus, events.GitHubTaskPRDeleted, ws.ActionGitHubTaskPRDeleted)
	b.subscribe(eventBus, events.GitHubTaskCIOptionsUpdated, ws.ActionGitHubTaskCIOptionsUpdated)
	b.subscribe(eventBus, events.GitHubRateLimitUpdated, ws.ActionGitHubRateLimitUpdated)
	b.subscribe(eventBus, events.GitLabTaskMRUpdated, ws.ActionGitLabTaskMRUpdated)

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

func (b *TaskEventBroadcaster) Close() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	b.subscriptions = nil
}

func (b *TaskEventBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	b.subscribeWithResolver(eventBus, subject, func(*bus.Event) string { return action })
}

// subscribeLifecycleStateEvents uses one NATS subscription so task and session
// state notifications remain ordered for clients when the event bus is remote.
func (b *TaskEventBroadcaster) subscribeLifecycleStateEvents(eventBus bus.EventBus) {
	b.subscribeWithResolver(eventBus, ">", func(event *bus.Event) string {
		switch event.Type {
		case events.TaskStateChanged:
			return ws.ActionTaskStateChanged
		case events.TaskSessionStateChanged:
			return ws.ActionSessionStateChanged
		default:
			return ""
		}
	})
}

func (b *TaskEventBroadcaster) subscribeWithResolver(
	eventBus bus.EventBus,
	subject string,
	resolveAction func(*bus.Event) string,
) {
	sub, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
		action := resolveAction(event)
		if action == "" {
			return nil
		}
		return b.broadcastEvent(ctx, event, action)
	})
	if err != nil {
		b.logger.Error("failed to subscribe to events", zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}

func (b *TaskEventBroadcaster) broadcastEvent(ctx context.Context, event *bus.Event, action string) error {
	msg, err := ws.NewNotification(action, event.Data)
	if err != nil {
		b.logger.Error("failed to build websocket notification", zap.String("action", action), zap.Error(err))
		return nil
	}
	sessionID := extractSessionID(event.Data)
	b.logSessionStateMetadata(action, sessionID, event.Data)
	if data, ok := event.Data.(map[string]interface{}); ok {
		b.logLifecycleBroadcast(action, data, sessionID)
	}

	return b.routeBroadcast(action, event.Data, sessionID, extractWorkspaceID(event.Data), msg)
}

func (b *TaskEventBroadcaster) logSessionStateMetadata(action, sessionID string, data interface{}) {
	if action != ws.ActionSessionStateChanged {
		return
	}
	payload, ok := data.(map[string]interface{})
	if !ok {
		return
	}
	metadata, ok := payload["metadata"]
	if !ok {
		return
	}
	b.logger.Debug("received session.state_changed with metadata",
		zap.String("action", action),
		zap.String("session_id", sessionID),
		zap.Any("metadata", metadata))
}

func (b *TaskEventBroadcaster) routeBroadcast(
	action string,
	data interface{},
	sessionID string,
	workspaceID string,
	msg *ws.Message,
) error {
	switch action {
	case ws.ActionWorkspaceCreated, ws.ActionWorkspaceUpdated, ws.ActionWorkspaceDeleted:
		// Workspace event payloads are the workspace DTO itself: the
		// workspace ID lives under "id", not "workspace_id". Without this
		// the extract comes back empty and the event would broadcast to
		// every user (caught by the auth E2E segregation spec).
		if workspaceID == "" {
			workspaceID = extractStringField(data, "id")
		}
		b.hub.BroadcastToWorkspace(workspaceID, msg)
		return nil
	case ws.ActionSessionAgentctlStarting, ws.ActionSessionAgentctlReady, ws.ActionSessionAgentctlError:
		if sessionID != "" {
			b.hub.BroadcastToSession(sessionID, msg)
			return nil
		}
	case ws.ActionSessionStateChanged:
		// Broadcast beyond the session subscribers so the sidebar task
		// switcher can track state changes for all tasks — but scoped to
		// the owning workspace's user when auth is enabled.
		b.hub.BroadcastToWorkspace(workspaceID, msg)
		return nil
	case ws.ActionSessionMessageAdded, ws.ActionSessionMessageUpdated, ws.ActionSessionMessageDeleted:
		if sessionID != "" {
			b.hub.BroadcastToSession(sessionID, msg)
			return nil
		}
	case ws.ActionSessionWorkspaceSourcesUpdated:
		if sessionID != "" {
			b.hub.BroadcastToSession(sessionID, msg)
			return nil
		}
	case ws.ActionMessageQueueStatusChanged:
		if sessionID != "" {
			b.hub.BroadcastToSession(sessionID, msg)
			return nil
		}
	case ws.ActionExecutorPrepareProgress, ws.ActionExecutorPrepareCompleted:
		// Broadcast to the owning workspace's clients so prepare
		// progress/warnings are available when the user navigates to the
		// session page after task creation.
		b.hub.BroadcastToWorkspace(workspaceID, msg)
		return nil
	}
	// Workspace-carrying events (task/workflow/repository/…) route to the
	// owner's clients; events without workspace context (executors,
	// environments, agent profiles — instance-wide resources) stay global.
	b.hub.BroadcastToWorkspace(workspaceID, msg)
	return nil
}

// extractWorkspaceID pulls a workspace ID from event payloads (map- or
// struct-shaped). Empty means "no workspace context" — the event is treated
// as instance-wide and broadcast to everyone.
func extractWorkspaceID(data interface{}) string {
	if id := extractStringField(data, "workspace_id"); id != "" {
		return id
	}
	if provider, ok := data.(interface{ GetWorkspaceID() string }); ok {
		return provider.GetWorkspaceID()
	}
	return ""
}

func extractStringField(data interface{}, key string) string {
	if m, ok := data.(map[string]interface{}); ok {
		if value, ok := m[key].(string); ok {
			return value
		}
	}
	return ""
}

func (b *TaskEventBroadcaster) logLifecycleBroadcast(action string, data map[string]interface{}, sessionID string) {
	switch action {
	case ws.ActionTaskCreated, ws.ActionTaskUpdated, ws.ActionTaskStateChanged,
		ws.ActionTaskDeleted, ws.ActionSessionStateChanged:
	default:
		return
	}
	b.logger.Debug("ws lifecycle broadcast",
		zap.String("action", action),
		zap.Any("task_id", data["task_id"]),
		zap.String("session_id", sessionID),
		zap.Any("state", data["state"]),
		zap.Any("old_state", data["old_state"]),
		zap.Any("new_state", data["new_state"]),
		zap.Any("primary_session_id", data["primary_session_id"]),
		zap.Any("primary_session_state", data["primary_session_state"]),
		zap.Any("updated_at", data["updated_at"]),
		zap.Int("connected_clients", b.hub.GetClientCount()),
	)
}
