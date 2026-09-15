package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

const webAppEventValidationTimeout = 2 * time.Second

// subscribeWebAppEvents connects the public web-app transport to the existing
// Kandev event bus. The hub remains an in-process bounded projection, so a
// restart creates a new generation and never pretends that old events are
// replayable.
func (s *Service) subscribeWebAppEvents() {
	if s == nil || s.eventBus == nil {
		return
	}
	s.mu.Lock()
	if s.eventSubscription != nil {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	subscription, err := s.eventBus.Subscribe(">", s.forwardWebAppEvent)
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.eventSubscription == nil {
		s.eventSubscription = subscription
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	_ = subscription.Unsubscribe()
}

func (s *Service) closeWebAppEvents() {
	if s == nil {
		return
	}
	s.mu.Lock()
	subscription := s.eventSubscription
	s.eventSubscription = nil
	hub := s.eventHub
	s.mu.Unlock()
	if subscription != nil {
		_ = subscription.Unsubscribe()
	}
	if hub != nil {
		hub.Close()
	}
}

func (s *Service) forwardWebAppEvent(ctx context.Context, source *bus.Event) error {
	if source == nil {
		return nil
	}
	projected, ok := projectWebAppEvent(source)
	if !ok {
		return nil
	}
	hub := s.WebAppEventHub()
	store := s.Instances()
	if hub == nil || store == nil {
		return nil
	}
	projected.Scope = s.resolveWebAppEventScope(ctx, projected.Scope)
	if projected.Scope.WorkspaceID == "" {
		return nil
	}
	return publishWebAppEventToInstances(ctx, hub, store, projected)
}

func (s *Service) resolveWebAppEventScope(ctx context.Context, scope webapp.EventScope) webapp.EventScope {
	if scope.WorkspaceID != "" || scope.TaskID == "" || s.taskData == nil {
		return scope
	}
	task, err := s.taskData.GetTask(ctx, scope.TaskID)
	if err == nil && task != nil {
		scope.WorkspaceID = task.WorkspaceID
	}
	return scope
}

func publishWebAppEventToInstances(ctx context.Context, hub *webapp.EventHub, store *instances.Store, projected webapp.EventInput) error {
	items, err := store.List(ctx, projected.Scope.WorkspaceID, false)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !webAppBusEventMatchesInstance(item, projected.Scope) {
			continue
		}
		projected.Scope.InstanceID = item.ID
		if _, err := hub.Publish(item.ID, projected); err != nil && !errors.Is(err, webapp.ErrEventHubClosed) {
			return err
		}
	}
	return nil
}

// projectWebAppEvent converts an internal bus event into the bounded public
// event DTO. Event payloads often contain persistence records, diagnostics, or
// source metadata that must never be copied into a browser stream. Keeping the
// allowlist here also makes adding a new event an explicit review decision.
func projectWebAppEvent(source *bus.Event) (webapp.EventInput, bool) {
	if source == nil || !isProjectableWebAppEvent(source.Type) {
		return webapp.EventInput{}, false
	}
	value, err := marshalEventMap(source.Data)
	if err != nil {
		return webapp.EventInput{}, false
	}
	allowed := map[string]struct{}{}
	for _, key := range publicEventFields(source.Type) {
		allowed[key] = struct{}{}
	}
	data := make(map[string]any, len(allowed))
	for key := range allowed {
		if item, exists := value[key]; exists {
			data[key] = item
			continue
		}
		camel := snakeToCamel(key)
		if item, exists := value[camel]; exists {
			data[key] = item
		}
	}
	scope := eventScopeFromPublicData(data)
	return webapp.EventInput{Type: source.Type, Scope: scope, Data: data}, true
}

func isProjectableWebAppEvent(eventType string) bool {
	switch eventType {
	case events.CanvasCreated, events.CanvasReleaseActivated, events.CanvasReleasePermissionRequired,
		events.CanvasPromoted, events.CanvasArchived, events.CanvasRestored, events.CanvasRemoved,
		events.TaskCreated, events.TaskUpdated, events.TaskStateChanged, events.TaskDeleted, events.TaskMoved,
		events.TaskQueuePromoted, events.TaskDependenciesResolved, events.TaskDependencyFailed,
		events.WorkspaceCreated, events.WorkspaceUpdated, events.WorkspaceDeleted,
		events.WorkflowCreated, events.WorkflowUpdated, events.WorkflowDeleted,
		events.WorkflowStepCreated, events.WorkflowStepUpdated, events.WorkflowStepDeleted,
		events.MessageAdded, events.MessageUpdated, events.MessageDeleted,
		events.TaskSessionStateChanged, events.TaskSessionActivityChanged, events.TaskSessionCancellationChanged,
		events.TaskSessionErrorChanged, events.TaskStatusSummaryUpdated:
		return true
	default:
		return false
	}
}

func publicEventFields(eventType string) []string {
	canvasFields := []string{
		"canvas_id", "workspace_id", "task_id", "scope_kind", "status", "title", "plugin_id", "plugin_instance_id",
		"release_id", "active_release_id", "validation_status", "validation_error", "source_actor_kind", "source_user_id",
		"source_task_id", "source_session_id", "placement", "protocol_version",
	}
	commonFields := []string{
		"workspace_id", "task_id", "session_id", "repository_id", "workflow_id", "workflow_step_id",
		"state", "status", "title", "name", "message_id", "task_session_id", "created_at", "updated_at",
	}
	if strings.HasPrefix(eventType, "canvas.") {
		return canvasFields
	}
	return commonFields
}

func marshalEventMap(data any) (map[string]any, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	if value == nil {
		value = make(map[string]any)
	}
	return value, nil
}

func snakeToCamel(value string) string {
	result := strings.Builder{}
	upper := false
	for _, character := range value {
		if character == '_' {
			upper = true
			continue
		}
		if upper {
			if character >= 'a' && character <= 'z' {
				character -= 'a' - 'A'
			}
			result.WriteRune(character)
			upper = false
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}

func eventScopeFromPublicData(data map[string]any) webapp.EventScope {
	return webapp.EventScope{
		WorkspaceID:  publicEventString(data, "workspace_id"),
		TaskID:       publicEventString(data, "task_id"),
		SessionID:    publicEventString(data, "session_id"),
		RepositoryID: publicEventString(data, "repository_id"),
	}
}

func publicEventString(data map[string]any, key string) string {
	value, ok := data[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func webAppBusEventMatchesInstance(item instances.Instance, scope webapp.EventScope) bool {
	if item.Status == instances.StatusRemoved {
		return false
	}
	if scope.WorkspaceID != "" && item.WorkspaceID != scope.WorkspaceID {
		return false
	}
	switch item.ScopeKind {
	case instances.ScopeInstance:
		return true
	case instances.ScopeWorkspace:
		return scope.WorkspaceID == item.WorkspaceID
	case instances.ScopeTask:
		return scope.TaskID == item.TaskID
	case instances.ScopeSession:
		return scope.SessionID == item.SessionID
	case instances.ScopeRepository:
		return scope.RepositoryID == item.RepositoryID
	default:
		return false
	}
}

func (s *Service) webAppEventFilter(binding webapp.CapabilityBinding) webapp.EventFilter {
	return func(event webapp.Event) bool {
		if !webAppEventMatchesBinding(event.Scope, binding) {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), webAppEventValidationTimeout)
		defer cancel()
		if s.validateWebAppBinding(ctx, binding) != nil {
			return false
		}
		return s.webAppEventDeclared(ctx, binding.ReleaseID, event.Type)
	}
}

// webAppEventDeclared keeps the event stream bound to the release manifest.
// Durable grant validation alone is not sufficient: an operator grant must
// never turn an undeclared event name into a subscription.
func (s *Service) webAppEventDeclared(ctx context.Context, releaseID, eventType string) bool {
	if strings.TrimSpace(releaseID) == "" || strings.TrimSpace(eventType) == "" {
		return false
	}
	store := s.Instances()
	if store == nil {
		return false
	}
	release, err := store.GetRelease(ctx, releaseID)
	if err != nil {
		return false
	}
	var m manifest.Manifest
	if err := json.Unmarshal(release.ManifestJSON, &m); err != nil {
		return false
	}
	return m.HasEvent(eventType)
}

func webAppEventMatchesBinding(scope webapp.EventScope, binding webapp.CapabilityBinding) bool {
	if scope.InstanceID != "" && scope.InstanceID != binding.InstanceID {
		return false
	}
	switch binding.ScopeKind {
	case instances.ScopeInstance:
		return true
	case instances.ScopeWorkspace:
		return scope.WorkspaceID == binding.WorkspaceID
	case instances.ScopeTask:
		return scope.TaskID == binding.TaskID
	case instances.ScopeSession:
		return scope.SessionID == binding.SessionID
	case instances.ScopeRepository:
		return scope.RepositoryID == binding.RepositoryID
	default:
		return false
	}
}
