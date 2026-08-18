package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Managed conversation metadata keys, stamped server-side on the backing
// task's Metadata map.
const (
	metaKeyPluginID        = "kandev.plugin_id"
	metaKeyWorkspaceID     = "kandev.workspace_id"
	metaKeyConversationKey = "kandev.conversation_key"
	metaKeyEphemeral       = "kandev.ephemeral"
	metaKeyManagedByPlugin = "kandev.managed_by_plugin"
	metaKeyInstructionVer  = "kandev.instruction_version"
)

// defaultAgentConversationTitle is the title for managed conversation
// backing tasks. Visible only in diagnostic/admin views, never on the
// task board.
const defaultAgentConversationTitle = "Managed Conversation"

// agentConversationTaskRepo is the narrow task-repository interface for
// managed conversation operations.
type agentConversationTaskRepo interface {
	ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error)
	CreateTask(ctx context.Context, task *models.Task) error
	DeleteTask(ctx context.Context, taskID string) error
}

// agentConversationSessionRepo is the narrow session-repository interface
// for creating and querying primary sessions on managed conversation tasks.
type agentConversationSessionRepo interface {
	GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
}

// agentConversationMessageRepo is the narrow message-repository interface
// for sending messages to managed conversation sessions.
type agentConversationMessageRepo interface {
	CreateMessage(ctx context.Context, msg *models.Message) error
}

// agentConversationStateRepo is the narrow state-storage interface for
// occurrence-key deduplication.
type agentConversationStateRepo interface {
	Get(ctx context.Context, scope, scopeID, key string) ([]byte, bool, error)
	Set(ctx context.Context, scope, scopeID, key string, value []byte) error
}

// agentConversationEventBus is the narrow event-bus interface for
// publishing task.created events. Matches bus.EventBus.Publish.
type agentConversationEventBus interface {
	Publish(ctx context.Context, subject string, event *bus.Event) error
}

// AgentConversationService implements the managed conversation lifecycle:
// Ensure (create-or-repair), Dispatch, and Delete. It uses narrow
// repository interfaces to avoid depending on the full task/Service.
//
// Ensure creates or repairs exactly one hidden, workflowless, ephemeral
// backing task and primary session per (pluginID, workspaceID, conversationKey).
// Dispatch sends prompt text to an ensured session with stable
// occurrence-key idempotency and busy-session coalescing.
// Delete removes all conversations for the given (pluginID, workspaceID, key).
type AgentConversationService struct {
	tasks   agentConversationTaskRepo
	sess    agentConversationSessionRepo
	msgs    agentConversationMessageRepo
	state   agentConversationStateRepo
	eventer agentConversationEventBus
}

// NewAgentConversationService creates a new service with the given dependencies.
func NewAgentConversationService(
	tasks agentConversationTaskRepo,
	sess agentConversationSessionRepo,
	msgs agentConversationMessageRepo,
	state agentConversationStateRepo,
	eventer agentConversationEventBus,
) *AgentConversationService {
	return &AgentConversationService{
		tasks:   tasks,
		sess:    sess,
		msgs:    msgs,
		state:   state,
		eventer: eventer,
	}
}

// Ensure creates or repairs exactly one managed conversation per
// (pluginID, workspaceID, conversationKey). The status string returned
// is one of "created", "exists", or "configuration_required".
func (s *AgentConversationService) Ensure(ctx context.Context, pluginID string, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	if pluginID == "" || spec.WorkspaceID == "" || spec.ConversationKey == "" {
		return pluginsdk.AgentConversationDescriptor{}, "", status.Error(codes.InvalidArgument, "plugin_id, workspace_id, and conversation_key are required")
	}

	// 1. Check for an existing conversation.
	existing, err := s.findManagedConversation(ctx, pluginID, spec.WorkspaceID, spec.ConversationKey)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	if existing != nil {
		return s.repairIfNeeded(ctx, existing, spec)
	}

	// 2. Profile resolution is handled by the backendapp adapter that
	// checks profile existence/status before calling Ensure. A missing
	// or disabled profile returns configuration_required from the caller.

	// 3. Create the backing task.
	metadata := map[string]interface{}{
		metaKeyPluginID:        pluginID,
		metaKeyWorkspaceID:     spec.WorkspaceID,
		metaKeyConversationKey: spec.ConversationKey,
		metaKeyEphemeral:       true,
		metaKeyManagedByPlugin: pluginID,
	}
	if spec.AgentProfileID != "" {
		metadata[models.MetaKeyAgentProfileID] = spec.AgentProfileID
	}
	if spec.BasePrompt != "" {
		metadata["kandev.base_prompt"] = spec.BasePrompt
	}

	task := &models.Task{
		ID:          uuid.New().String(),
		WorkspaceID: spec.WorkspaceID,
		Title:       defaultAgentConversationTitle + " - " + spec.ConversationKey,
		State:       v1.TaskStateCreated,
		Priority:    "medium",
		IsEphemeral: true,
		Origin:      models.TaskOriginManual,
		Metadata:    metadata,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := s.tasks.CreateTask(ctx, task); err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", fmt.Errorf("failed to insert conversation task: %w", err)
	}

	// 4. Create the primary session.
	primary, err := s.createPrimarySession(ctx, task.ID)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", fmt.Errorf("failed to create conversation session: %w", err)
	}

	// 5. Publish task.created (best-effort, non-blocking).
	s.publishTaskCreated(ctx, task)

	return pluginsdk.AgentConversationDescriptor{
		TaskID:          task.ID,
		SessionID:       primary.ID,
		WorkspaceID:     spec.WorkspaceID,
		ConversationKey: spec.ConversationKey,
		AgentProfileID:  spec.AgentProfileID,
	}, "created", nil
}

// repairIfNeeded repairs a managed conversation whose primary session is
// missing (e.g. after a partial creation failure) and returns "exists".
func (s *AgentConversationService) repairIfNeeded(ctx context.Context, existing *models.Task, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, existing.ID)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", fmt.Errorf("failed to check existing conversation session: %w", err)
	}
	if primary == nil {
		primary, err = s.createPrimarySession(ctx, existing.ID)
		if err != nil {
			return pluginsdk.AgentConversationDescriptor{}, "", fmt.Errorf("failed to repair conversation session: %w", err)
		}
	}

	profileID := ""
	if existing.Metadata != nil {
		if p, ok := existing.Metadata[models.MetaKeyAgentProfileID].(string); ok {
			profileID = p
		}
	}

	return pluginsdk.AgentConversationDescriptor{
		TaskID:          existing.ID,
		SessionID:       primary.ID,
		WorkspaceID:     existing.WorkspaceID,
		ConversationKey: spec.ConversationKey,
		AgentProfileID:  profileID,
	}, "exists", nil
}

// createPrimarySession creates a primary session row for the given task.
func (s *AgentConversationService) createPrimarySession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	primary := &models.TaskSession{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		State:     models.TaskSessionStateCreated,
		IsPrimary: true,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.sess.CreateTaskSession(ctx, primary); err != nil {
		return nil, err
	}
	return primary, nil
}

// publishTaskCreated publishes a task.created event for the managed
// conversation, best-effort and non-blocking.
func (s *AgentConversationService) publishTaskCreated(ctx context.Context, task *models.Task) {
	if s.eventer == nil {
		return
	}
	eventCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	event := bus.NewEvent(events.TaskCreated, "agent-conversation-service", map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
		"title":        task.Title,
		"is_ephemeral": true,
	})
	_ = s.eventer.Publish(eventCtx, events.TaskCreated, event)
}

// Dispatch sends text to an ensured conversation. occurrenceKey provides
// stable idempotency. When the session is mid-turn, returns "skipped_busy".
func (s *AgentConversationService) Dispatch(ctx context.Context, pluginID, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error) {
	if pluginID == "" || workspaceID == "" || conversationKey == "" {
		return pluginsdk.AgentConversationDispatch{}, status.Error(codes.InvalidArgument, "plugin_id, workspace_id, and conversation_key are required")
	}

	// 1. Claim the occurrence key for idempotency.
	if occurrenceKey != "" {
		alreadyClaimed, err := s.claimOccurrenceKey(ctx, pluginID, workspaceID, conversationKey, occurrenceKey)
		if err != nil {
			return pluginsdk.AgentConversationDispatch{}, fmt.Errorf("failed to claim occurrence key: %w", err)
		}
		if alreadyClaimed {
			return pluginsdk.AgentConversationDispatch{
				Status: "duplicate_occurrence",
			}, nil
		}
	}

	// 2. Find the conversation.
	existing, err := s.findManagedConversation(ctx, pluginID, workspaceID, conversationKey)
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	if existing == nil {
		return pluginsdk.AgentConversationDispatch{}, status.Error(codes.NotFound, "conversation not found, call Ensure first")
	}

	// 3. Get the primary session and check if it's mid-turn.
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, existing.ID)
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	if primary != nil && primary.State == models.TaskSessionStateRunning {
		return pluginsdk.AgentConversationDispatch{
			SessionID: primary.ID,
			Status:    "skipped_busy",
			Descriptor: pluginsdk.AgentConversationDescriptor{
				TaskID:          existing.ID,
				SessionID:       primary.ID,
				WorkspaceID:     workspaceID,
				ConversationKey: conversationKey,
			},
		}, nil
	}

	// 4. Create the user message.
	sessionID := ""
	if primary != nil {
		sessionID = primary.ID
	}

	msg := createDispatchMessage(sessionID, existing.ID, text, pluginID, occurrenceKey)
	if err := s.msgs.CreateMessage(ctx, msg); err != nil {
		return pluginsdk.AgentConversationDispatch{}, fmt.Errorf("failed to create message: %w", err)
	}

	return pluginsdk.AgentConversationDispatch{
		SessionID: sessionID,
		Status:    "sent",
		Descriptor: pluginsdk.AgentConversationDescriptor{
			TaskID:          existing.ID,
			SessionID:       sessionID,
			WorkspaceID:     workspaceID,
			ConversationKey: conversationKey,
		},
	}, nil
}

// createDispatchMessage builds a user message for a managed conversation
// dispatch, recording provenance and occurrence key in metadata.
func createDispatchMessage(sessionID, taskID, text, pluginID, occurrenceKey string) *models.Message {
	msg := &models.Message{
		ID:            uuid.New().String(),
		TaskSessionID: sessionID,
		TaskID:        taskID,
		AuthorType:    models.MessageAuthorUser,
		Type:          models.MessageTypeMessage,
		Content:       text,
		Metadata: map[string]interface{}{
			"source": "plugin:" + pluginID,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if occurrenceKey != "" {
		msg.Metadata["occurrence_key"] = occurrenceKey
	}
	return msg
}

// Delete removes all managed conversations owned by pluginID matching
// workspaceID and conversationKey. Returns the count of deleted tasks.
func (s *AgentConversationService) Delete(ctx context.Context, pluginID, workspaceID, conversationKey string) (int32, error) {
	tasks, err := s.listManagedConversations(ctx, pluginID, workspaceID, conversationKey)
	if err != nil {
		return 0, err
	}
	var count int32
	for _, t := range tasks {
		if err := s.tasks.DeleteTask(ctx, t.ID); err != nil {
			return count, fmt.Errorf("failed to delete managed conversation task %s: %w", t.ID, err)
		}
		count++
	}
	return count, nil
}

// findManagedConversation returns the first managed conversation matching
// the given (pluginID, workspaceID, conversationKey), or nil when none
// exists.
func (s *AgentConversationService) findManagedConversation(ctx context.Context, pluginID, workspaceID, conversationKey string) (*models.Task, error) {
	tasks, _, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, "", "", "", 1, 100, "", false, true, true, false)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if isManagedConversation(task, pluginID, workspaceID, conversationKey) {
			return task, nil
		}
	}
	return nil, nil
}

// listManagedConversations returns all managed conversations matching the
// given (pluginID, workspaceID, conversationKey).
func (s *AgentConversationService) listManagedConversations(ctx context.Context, pluginID, workspaceID, conversationKey string) ([]*models.Task, error) {
	tasks, _, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, "", "", "", 1, 100, "", false, true, true, false)
	if err != nil {
		return nil, err
	}
	var out []*models.Task
	for _, task := range tasks {
		if isManagedConversation(task, pluginID, workspaceID, conversationKey) {
			out = append(out, task)
		}
	}
	return out, nil
}

// isManagedConversation checks whether task's metadata identifies it as a
// managed conversation owned by pluginID for the given workspace and key.
func isManagedConversation(task *models.Task, pluginID, workspaceID, conversationKey string) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	pID, _ := task.Metadata[metaKeyPluginID].(string)
	wID, _ := task.Metadata[metaKeyWorkspaceID].(string)
	cKey, _ := task.Metadata[metaKeyConversationKey].(string)
	ephemeral, _ := task.Metadata[metaKeyEphemeral].(bool)
	return pID == pluginID && wID == workspaceID && cKey == conversationKey && ephemeral
}

// claimOccurrenceKey atomically claims an occurrence key for
// (pluginID, workspaceID, conversationKey). Returns true when the key was
// already claimed (duplicate), false when this call claimed it.
func (s *AgentConversationService) claimOccurrenceKey(ctx context.Context, pluginID, workspaceID, conversationKey, occurrenceKey string) (bool, error) {
	stateKey := "occurrence:" + occurrenceKey
	scope := "plugin:" + pluginID
	scopeID := workspaceID + "/" + conversationKey

	_, found, err := s.state.Get(ctx, scope, scopeID, stateKey)
	if err != nil {
		return false, err
	}
	if found {
		return true, nil
	}
	value, _ := json.Marshal(map[string]interface{}{
		"claimed":          true,
		"plugin_id":        pluginID,
		"workspace_id":     workspaceID,
		"conversation_key": conversationKey,
	})
	if err := s.state.Set(ctx, scope, scopeID, stateKey, value); err != nil {
		return false, err
	}
	return false, nil
}

// IsManagedConversationTask is a public predicate that reports whether a
// task is a managed conversation backing task. Used by Quick Chat
// restoration and expiration to exclude these conversations.
func IsManagedConversationTask(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	_, hasPlugin := task.Metadata[metaKeyPluginID].(string)
	isEphemeral := task.IsEphemeral
	if !isEphemeral {
		if e, ok := task.Metadata[metaKeyEphemeral].(bool); ok {
			isEphemeral = e
		}
	}
	return isEphemeral && hasPlugin
}
