package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
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

// agentConversationOccurrenceScope namespaces occurrence-key claims within
// the shared state store, separate from any plugin_state key a plugin
// writes itself through the ordinary State() RPCs.
const agentConversationOccurrenceScope = "agent_conversation_occurrence"

// Ensure's status results.
const (
	// AgentConversationStatusCreated is returned when Ensure created a brand
	// new managed conversation.
	AgentConversationStatusCreated = "created"
	// AgentConversationStatusExists is returned when Ensure found (and, if
	// needed, repaired) an already-managed conversation.
	AgentConversationStatusExists = "exists"
	// AgentConversationStatusConfigurationRequired is returned from Ensure
	// when the requested agent profile is missing, disabled, deleted, or
	// otherwise cannot back a new conversation. No task or session row is
	// created.
	AgentConversationStatusConfigurationRequired = "configuration_required"
)

// agentConversationTaskRepo is the narrow task-repository interface for
// managed conversation operations.
type agentConversationTaskRepo interface {
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
	ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error)
	// ListEphemeralTasksAllWorkspaces returns every ephemeral task across
	// every workspace, unpaginated. Used only by DeleteAllForPlugin (plugin
	// uninstall) — ListTasksByWorkspace cannot answer a cross-workspace query
	// because it always scopes to one workspace_id.
	ListEphemeralTasksAllWorkspaces(ctx context.Context) ([]*models.Task, error)
	CreateTask(ctx context.Context, task *models.Task) error
	UpdateTask(ctx context.Context, task *models.Task) error
	DeleteTask(ctx context.Context, taskID string) error
}

// agentConversationSessionRepo is the narrow session-repository interface
// for creating and querying primary sessions on managed conversation tasks.
type agentConversationSessionRepo interface {
	GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
}

// AgentConversationProfileInfo is the minimal agent-profile information
// Ensure needs to gate hidden-conversation creation on a usable profile.
type AgentConversationProfileInfo struct {
	Enabled bool
}

// agentConversationProfileRepo is the narrow read-only interface Ensure uses
// to validate a plugin-configured agent profile before creating (or
// repairing) any hidden task/session. Implemented by a backendapp adapter
// over the agent settings repository, avoiding an
// internal/task/service → internal/agent/settings import. ok is false when
// profileID does not resolve to any usable profile (never existed, or
// soft-deleted — the settings repository's GetAgentProfile already filters
// deleted_at IS NULL, so "not found" covers both "missing" and "deleted").
type agentConversationProfileRepo interface {
	GetProfile(ctx context.Context, profileID string) (AgentConversationProfileInfo, bool, error)
}

// agentConversationStateRepo is the narrow state-storage interface for
// durable, atomic occurrence-key deduplication. Claim must be backed by a
// real uniqueness constraint (not a check-then-write pair) so it stays
// correct across concurrent callers, a backend restart, and — since the
// backing store is a shared SQLite/Postgres table, not a process-local
// map — multiple backend instances.
type agentConversationStateRepo interface {
	Claim(ctx context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) (claimed bool, err error)
	Delete(ctx context.Context, pluginID, scope, scopeID, key string) error
}

// agentConversationEventBus is the narrow event-bus interface for
// publishing task.created events. Matches bus.EventBus.Publish.
type agentConversationEventBus interface {
	Publish(ctx context.Context, subject string, event *bus.Event) error
}

// agentConversationDispatcher performs the real orchestrator-backed message
// delivery a scheduled wake needs: start a never-launched (CREATED) session,
// or prompt an idle one (resuming its agent process first if it has gone,
// e.g. after a backend restart) — the same delivery path the plugin Host's
// SendMessage RPC uses. Implemented by a backendapp adapter wrapping the
// task service and orchestrator, avoiding an
// internal/task/service → internal/orchestrator import.
//
// Dispatch only calls Deliver once it has confirmed the session is not
// RUNNING/STARTING (that check, and the "skipped_busy" result, stay in this
// package); Deliver itself is responsible for idempotent message recording
// keyed by idempotencyID, so a retried occurrence cannot create a second
// turn or double-dispatch to the agent.
type agentConversationDispatcher interface {
	Deliver(ctx context.Context, taskID string, session *models.TaskSession, text, source, idempotencyID string) (status string, err error)
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
	profile agentConversationProfileRepo
	state   agentConversationStateRepo
	eventer agentConversationEventBus

	// dispatcher delivers Dispatch's text to the real agent runtime. It is
	// wired late (SetDispatcher), after the orchestrator exists — mirroring
	// pluginHost's writeDeps/utilityDeps "read live, not snapshotted"
	// pattern (internal/plugins/host.go) for the same boot-ordering reason:
	// the orchestrator is constructed after this service. Guarded by mu so
	// concurrent Dispatch calls observe a consistent value.
	mu         sync.RWMutex
	dispatcher agentConversationDispatcher

	// ensureLocksMu/ensureLocks serialize Ensure per (pluginID, workspaceID,
	// conversationKey). The metadata-backed conversation lookup
	// (findManagedConversation) has no repository-level uniqueness
	// constraint to close a check-then-create race on its own — two
	// same-process callers (e.g. a browser open and the plugin's heartbeat
	// runner firing at the same moment) can both observe "no existing
	// conversation" and both create a task. This in-process lock closes that
	// window for the common single-instance deployment; it does not extend
	// across multiple backend processes sharing one database (the plan
	// defers a dedicated uniqueness table unless that is demonstrated
	// necessary). Entries are never evicted — the key space is bounded by
	// the number of distinct managed conversations, which is small and
	// long-lived for the lifetime of the process.
	ensureLocksMu sync.Mutex
	ensureLocks   map[string]*sync.Mutex
}

// NewAgentConversationService creates a new service with the given dependencies.
// Call SetDispatcher once the orchestrator-backed dispatcher is available;
// until then, Dispatch returns Unavailable.
func NewAgentConversationService(
	tasks agentConversationTaskRepo,
	sess agentConversationSessionRepo,
	profile agentConversationProfileRepo,
	state agentConversationStateRepo,
	eventer agentConversationEventBus,
) *AgentConversationService {
	return &AgentConversationService{
		tasks:   tasks,
		sess:    sess,
		profile: profile,
		state:   state,
		eventer: eventer,
	}
}

// SetDispatcher wires the live orchestrator-backed dispatcher. See the
// dispatcher field's doc comment for why this is deferred past construction.
func (s *AgentConversationService) SetDispatcher(d agentConversationDispatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatcher = d
}

func (s *AgentConversationService) getDispatcher() agentConversationDispatcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dispatcher
}

// lockEnsureKey serializes Ensure calls for one (pluginID, workspaceID,
// conversationKey) tuple. See the ensureLocks field doc comment.
func (s *AgentConversationService) lockEnsureKey(key string) func() {
	s.ensureLocksMu.Lock()
	if s.ensureLocks == nil {
		s.ensureLocks = make(map[string]*sync.Mutex)
	}
	l, ok := s.ensureLocks[key]
	if !ok {
		l = &sync.Mutex{}
		s.ensureLocks[key] = l
	}
	s.ensureLocksMu.Unlock()
	l.Lock()
	return l.Unlock
}

// Ensure creates or repairs exactly one managed conversation per
// (pluginID, workspaceID, conversationKey). The status string returned is
// one of "created", "exists", or AgentConversationStatusConfigurationRequired
// (a missing, disabled, or deleted agent profile — no task or session row is
// created or repaired in that case).
func (s *AgentConversationService) Ensure(ctx context.Context, pluginID string, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	if pluginID == "" || spec.WorkspaceID == "" || spec.ConversationKey == "" {
		return pluginsdk.AgentConversationDescriptor{}, "", status.Error(codes.InvalidArgument, "plugin_id, workspace_id, and conversation_key are required")
	}

	unlock := s.lockEnsureKey(pluginID + "/" + spec.WorkspaceID + "/" + spec.ConversationKey)
	defer unlock()

	// 1. Resolve and validate the effective agent profile BEFORE creating
	// any row. Plugins may deliberately leave AgentProfileID empty to use the
	// workspace default, but the managed session must still be born with a
	// concrete, usable profile. A missing, disabled, or deleted effective
	// profile returns configuration_required without partial task/session rows.
	effectiveProfileID, ok, err := s.resolveEffectiveProfile(ctx, spec.WorkspaceID, spec.AgentProfileID)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	if !ok {
		return pluginsdk.AgentConversationDescriptor{}, AgentConversationStatusConfigurationRequired, nil
	}
	spec.AgentProfileID = effectiveProfileID

	// 2. Check for an existing conversation.
	existing, err := s.findManagedConversation(ctx, pluginID, spec.WorkspaceID, spec.ConversationKey)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	if existing != nil {
		return s.repairIfNeeded(ctx, existing, spec)
	}

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
	primary, err := s.createPrimarySession(ctx, task.ID, spec.AgentProfileID)
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
	}, AgentConversationStatusCreated, nil
}

// resolveEffectiveProfile converts an optional plugin profile into a concrete
// usable profile. An empty plugin value intentionally means the workspace
// default, not an unconfigured session: the real turn path requires an agent
// profile before it can start the primary session.
func (s *AgentConversationService) resolveEffectiveProfile(ctx context.Context, workspaceID, profileID string) (string, bool, error) {
	if profileID == "" {
		workspace, err := s.tasks.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return "", false, fmt.Errorf("failed to resolve workspace default agent profile: %w", err)
		}
		if workspace == nil || workspace.DefaultAgentProfileID == nil {
			return "", false, nil
		}
		profileID = *workspace.DefaultAgentProfileID
	}
	if profileID == "" {
		return "", false, nil
	}
	ok, err := s.validateProfile(ctx, profileID)
	if err != nil || !ok {
		return "", ok, err
	}
	return profileID, true, nil
}

// validateProfile reports whether a concrete profile can back a new or
// repaired conversation. It must resolve to an existing, enabled row.
func (s *AgentConversationService) validateProfile(ctx context.Context, profileID string) (bool, error) {
	if profileID == "" {
		return false, nil
	}
	if s.profile == nil {
		// No validator wired (e.g. a bare test service): fail open only when
		// the caller genuinely has no way to validate. Production wiring
		// (backendapp) always sets this.
		return true, nil
	}
	info, ok, err := s.profile.GetProfile(ctx, profileID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve agent profile: %w", err)
	}
	if !ok || !info.Enabled {
		return false, nil
	}
	return true, nil
}

// repairIfNeeded repairs a managed conversation whose primary session is
// missing (e.g. after a partial creation failure) and returns "exists". If
// the task's bound agent profile is no longer usable, repair is refused with
// AgentConversationStatusConfigurationRequired rather than creating a new
// session against an invalid profile.
func (s *AgentConversationService) repairIfNeeded(ctx context.Context, existing *models.Task, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	// Ensure resolved and validated this concrete profile before locating the
	// existing conversation. Treat it as authoritative on every call so a
	// changed plugin setting is reconciled instead of silently continuing with
	// an old (or subsequently disabled) binding.
	profileID := spec.AgentProfileID
	if err := s.reconcileConversationTask(ctx, existing, profileID, spec.BasePrompt); err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}

	primary, err := s.ensureConversationPrimarySession(ctx, existing.ID, profileID)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}

	return pluginsdk.AgentConversationDescriptor{
		TaskID:          existing.ID,
		SessionID:       primary.ID,
		WorkspaceID:     existing.WorkspaceID,
		ConversationKey: spec.ConversationKey,
		AgentProfileID:  profileID,
	}, AgentConversationStatusExists, nil
}

func (s *AgentConversationService) ensureConversationPrimarySession(ctx context.Context, taskID, profileID string) (*models.TaskSession, error) {
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing conversation session: %w", err)
	}
	if primary == nil {
		primary, err = s.createPrimarySession(ctx, taskID, profileID)
		if err != nil {
			return nil, fmt.Errorf("failed to repair conversation session: %w", err)
		}
		return primary, nil
	}
	if primary.AgentProfileID == profileID {
		return primary, nil
	}
	primary.AgentProfileID = profileID
	primary.UpdatedAt = time.Now().UTC()
	if err := s.sess.UpdateTaskSession(ctx, primary); err != nil {
		return nil, fmt.Errorf("failed to reconcile conversation session profile: %w", err)
	}
	return primary, nil
}

// reconcileConversationTask refreshes the mutable, server-owned config
// stamped on an existing backing task. BasePrompt is deliberately replaced
// even when it becomes empty so a settings save can remove a previously
// configured instruction.
func (s *AgentConversationService) reconcileConversationTask(ctx context.Context, task *models.Task, profileID, basePrompt string) error {
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	changed := false
	if current, _ := task.Metadata[models.MetaKeyAgentProfileID].(string); current != profileID {
		task.Metadata[models.MetaKeyAgentProfileID] = profileID
		changed = true
	}
	if current, _ := task.Metadata["kandev.base_prompt"].(string); current != basePrompt {
		if basePrompt == "" {
			delete(task.Metadata, "kandev.base_prompt")
		} else {
			task.Metadata["kandev.base_prompt"] = basePrompt
		}
		changed = true
	}
	if !changed {
		return nil
	}
	task.UpdatedAt = time.Now().UTC()
	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to reconcile conversation task configuration: %w", err)
	}
	return nil
}

// createPrimarySession creates a primary session row for the given task.
func (s *AgentConversationService) createPrimarySession(ctx context.Context, taskID, agentProfileID string) (*models.TaskSession, error) {
	primary := &models.TaskSession{
		ID:             uuid.New().String(),
		TaskID:         taskID,
		AgentProfileID: agentProfileID,
		State:          models.TaskSessionStateCreated,
		IsPrimary:      true,
		StartedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
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

// Dispatch delivers text to an ensured conversation through the real agent
// runtime. occurrenceKey, when non-empty, provides durable, atomic
// idempotency: a retried occurrence (same plugin/workspace/key/occurrence,
// whether re-sent by a concurrent caller or after a backend restart) is
// claimed exactly once and returns "duplicate_occurrence" for every other
// caller. When the session is mid-turn (RUNNING or STARTING), Dispatch
// returns "skipped_busy" without touching the runtime, so a busy session
// never accumulates queued heartbeats.
func (s *AgentConversationService) Dispatch(ctx context.Context, pluginID, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error) {
	if pluginID == "" || workspaceID == "" || conversationKey == "" {
		return pluginsdk.AgentConversationDispatch{}, status.Error(codes.InvalidArgument, "plugin_id, workspace_id, and conversation_key are required")
	}

	existing, primary, busy, err := s.dispatchTarget(ctx, pluginID, workspaceID, conversationKey)
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	if busy != nil {
		return *busy, nil
	}

	// 3. Resolve the runtime before consuming a durable occurrence key. A
	// restart may leave this dependency temporarily unavailable; that must not
	// turn a later retry into duplicate_occurrence.
	dispatcher := s.getDispatcher()
	if dispatcher == nil {
		return pluginsdk.AgentConversationDispatch{}, status.Error(codes.Unavailable, "agent conversation dispatch is not ready yet")
	}

	// 4. Atomically claim only after every precondition which cannot deliver
	// has passed. The claim is released if Deliver fails, so a transient
	// runtime error can safely retry the same scheduled occurrence.
	idempotencyID, claimedOccurrence, duplicate, err := s.claimDispatchOccurrence(ctx, pluginID, workspaceID, conversationKey, occurrenceKey)
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	if duplicate {
		return pluginsdk.AgentConversationDispatch{Status: "duplicate_occurrence"}, nil
	}

	// 5. Deliver through the real agent runtime (start a never-launched
	// session, or prompt/resume an idle one).
	deliverStatus, err := s.deliverConversationPrompt(ctx, dispatcher, existing, primary, text, pluginID, workspaceID, conversationKey, idempotencyID, claimedOccurrence, occurrenceKey)
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}

	return pluginsdk.AgentConversationDispatch{
		SessionID: primary.ID,
		Status:    deliverStatus,
		Descriptor: pluginsdk.AgentConversationDescriptor{
			TaskID:          existing.ID,
			SessionID:       primary.ID,
			WorkspaceID:     workspaceID,
			ConversationKey: conversationKey,
		},
	}, nil
}

func (s *AgentConversationService) dispatchTarget(ctx context.Context, pluginID, workspaceID, conversationKey string) (*models.Task, *models.TaskSession, *pluginsdk.AgentConversationDispatch, error) {
	existing, err := s.findManagedConversation(ctx, pluginID, workspaceID, conversationKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if existing == nil {
		return nil, nil, nil, status.Error(codes.NotFound, "conversation not found, call Ensure first")
	}
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, existing.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if primary == nil {
		return nil, nil, nil, status.Error(codes.NotFound, "conversation has no session, call Ensure first")
	}
	if primary.State != models.TaskSessionStateRunning && primary.State != models.TaskSessionStateStarting {
		return existing, primary, nil, nil
	}
	return nil, nil, &pluginsdk.AgentConversationDispatch{
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

func (s *AgentConversationService) claimDispatchOccurrence(ctx context.Context, pluginID, workspaceID, conversationKey, occurrenceKey string) (string, bool, bool, error) {
	if occurrenceKey == "" {
		return uuid.New().String(), false, false, nil
	}
	alreadyClaimed, err := s.claimOccurrenceKey(ctx, pluginID, workspaceID, conversationKey, occurrenceKey)
	if err != nil {
		return "", false, false, fmt.Errorf("failed to claim occurrence key: %w", err)
	}
	if alreadyClaimed {
		return "", false, true, nil
	}
	return deriveOccurrenceMessageID(pluginID, workspaceID, conversationKey, occurrenceKey), true, false, nil
}

func (s *AgentConversationService) deliverConversationPrompt(ctx context.Context, dispatcher agentConversationDispatcher, task *models.Task, primary *models.TaskSession, text, pluginID, workspaceID, conversationKey, idempotencyID string, claimedOccurrence bool, occurrenceKey string) (string, error) {
	deliverStatus, err := dispatcher.Deliver(ctx, task.ID, primary, composeConversationPrompt(task, text), "plugin:"+pluginID, idempotencyID)
	if err == nil {
		return deliverStatus, nil
	}
	if !claimedOccurrence {
		return "", fmt.Errorf("failed to deliver message: %w", err)
	}
	if releaseErr := s.releaseOccurrenceKey(ctx, pluginID, workspaceID, conversationKey, occurrenceKey); releaseErr != nil {
		return "", fmt.Errorf("failed to deliver message: %w; failed to release occurrence key: %v", err, releaseErr)
	}
	return "", fmt.Errorf("failed to deliver message: %w", err)
}

func composeConversationPrompt(task *models.Task, text string) string {
	if task == nil || task.Metadata == nil {
		return text
	}
	basePrompt, _ := task.Metadata["kandev.base_prompt"].(string)
	if basePrompt == "" {
		return text
	}
	if text == "" {
		return basePrompt
	}
	return basePrompt + "\n\n" + text
}

// deriveOccurrenceMessageID derives a stable, collision-free message id from
// the full occurrence coordinates, so the same occurrence always maps to the
// same message-table primary key (across plugins/workspaces/keys, whose
// occurrenceKey strings may otherwise collide) and repeated calls for the
// same occurrence are naturally idempotent at the storage layer too.
func deriveOccurrenceMessageID(pluginID, workspaceID, conversationKey, occurrenceKey string) string {
	name := pluginID + "|" + workspaceID + "|" + conversationKey + "|" + occurrenceKey
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

// Delete removes all managed conversations owned by pluginID matching
// workspaceID and conversationKey. Returns the count of deleted tasks.
func (s *AgentConversationService) Delete(ctx context.Context, pluginID, workspaceID, conversationKey string) (int32, error) {
	if pluginID == "" || workspaceID == "" || conversationKey == "" {
		return 0, status.Error(codes.InvalidArgument, "plugin_id, workspace_id, and conversation_key are required")
	}
	tasks, err := s.listManagedConversations(ctx, pluginID, workspaceID, conversationKey)
	if err != nil {
		return 0, err
	}
	var count int32
	for _, t := range tasks {
		if err := s.tasks.DeleteTask(ctx, t.ID); err != nil {
			// Already gone is the goal state here for the same reason it is in
			// DeleteAllForPlugin: the listing above is not held under a lock, so
			// an uninstall cleanup or a retried delete can remove the row in
			// between. A caller that asked for a conversation to be gone and
			// finds it gone has not failed, and a plugin already treats "no
			// conversation matched" as success.
			if errors.Is(err, taskrepo.ErrTaskNotFound) {
				continue
			}
			return count, fmt.Errorf("failed to delete managed conversation task %s: %w", t.ID, err)
		}
		count++
	}
	return count, nil
}

// DeleteAllForPlugin removes every managed conversation owned by pluginID,
// across every workspace and conversation key. Called by the plugin system's
// Uninstall lifecycle (never by the plugin's own gRPC requests — see the
// AgentConversationService interface doc in internal/plugins), so a plugin's
// hidden conversations are never orphaned after uninstall. Provenance-safe:
// isManagedConversationOwnedByPlugin only matches ephemeral tasks whose
// stamped kandev.plugin_id metadata equals pluginID, so ordinary user tasks
// and other plugins' managed conversations are never touched. Returns the
// number of conversations removed and the first deletion error encountered
// (deletion keeps going after an error so one bad row cannot block cleanup
// of the rest; the caller reports the error rather than treating a partial
// failure as success — see criterion 15's "failure is reported rather than
// silently orphaning data").
func (s *AgentConversationService) DeleteAllForPlugin(ctx context.Context, pluginID string) (int32, error) {
	if pluginID == "" {
		return 0, status.Error(codes.InvalidArgument, "plugin_id is required")
	}
	tasks, err := s.tasks.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list ephemeral tasks for plugin uninstall cleanup: %w", err)
	}
	var count int32
	var firstErr error
	for _, task := range tasks {
		if !isManagedConversationOwnedByPlugin(task, pluginID) {
			continue
		}
		if err := s.tasks.DeleteTask(ctx, task.ID); err != nil {
			// The conversation being gone already is the goal state, not a
			// failure: the listing above is not held under a lock, so a
			// concurrent cleanup (or a retried uninstall racing itself) can
			// remove a row between the list and this delete. Reporting that
			// as an error would abort an uninstall that actually succeeded,
			// since Service.Uninstall is fail-visible on this path.
			if errors.Is(err, taskrepo.ErrTaskNotFound) {
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to delete managed conversation task %s: %w", task.ID, err)
			}
			continue
		}
		count++
	}
	return count, firstErr
}

// managedConversationPageSize is the page size used when scanning a
// workspace's ephemeral tasks for managed conversations. The repository has
// no metadata predicate for the provenance keys, so the scan pages through
// the whole ephemeral set rather than sampling the first page: quick chats
// and other ephemeral tasks share this list, and a workspace that
// accumulates more than one page of them would otherwise hide an existing
// conversation, making Ensure create a duplicate and Dispatch report
// NotFound.
const managedConversationPageSize = 100

// findManagedConversation returns the first managed conversation matching
// the given (pluginID, workspaceID, conversationKey), or nil when none
// exists.
func (s *AgentConversationService) findManagedConversation(ctx context.Context, pluginID, workspaceID, conversationKey string) (*models.Task, error) {
	var found *models.Task
	err := s.eachManagedConversation(ctx, pluginID, workspaceID, conversationKey, func(task *models.Task) bool {
		found = task
		return false
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// listManagedConversations returns all managed conversations matching the
// given (pluginID, workspaceID, conversationKey).
func (s *AgentConversationService) listManagedConversations(ctx context.Context, pluginID, workspaceID, conversationKey string) ([]*models.Task, error) {
	var out []*models.Task
	err := s.eachManagedConversation(ctx, pluginID, workspaceID, conversationKey, func(task *models.Task) bool {
		out = append(out, task)
		return true
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// eachManagedConversation pages through the workspace's ephemeral tasks and
// invokes visit for every managed conversation matching the given
// coordinates. visit returns false to stop early.
func (s *AgentConversationService) eachManagedConversation(ctx context.Context, pluginID, workspaceID, conversationKey string, visit func(*models.Task) bool) error {
	for page := 1; ; page++ {
		tasks, total, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, "", "", "", page, managedConversationPageSize, "", false, true, true, false)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if isManagedConversation(task, pluginID, workspaceID, conversationKey) {
				if !visit(task) {
					return nil
				}
			}
		}
		scanned := page * managedConversationPageSize
		if len(tasks) < managedConversationPageSize || scanned >= total {
			return nil
		}
	}
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

// isManagedConversationOwnedByPlugin is isManagedConversation without the
// workspace/conversationKey narrowing — used by DeleteAllForPlugin, which
// must find every managed conversation a plugin owns regardless of which
// workspace or conversation key it was created under.
func isManagedConversationOwnedByPlugin(task *models.Task, pluginID string) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	pID, _ := task.Metadata[metaKeyPluginID].(string)
	ephemeral, _ := task.Metadata[metaKeyEphemeral].(bool)
	return pID == pluginID && ephemeral
}

// claimOccurrenceKey atomically claims an occurrence key for
// (pluginID, workspaceID, conversationKey), durably (survives a backend
// restart) and atomically (safe under concurrent callers, including
// multiple backend instances sharing the same store). Returns true when the
// key was already claimed by a prior call (duplicate), false when this call
// claimed it.
func (s *AgentConversationService) claimOccurrenceKey(ctx context.Context, pluginID, workspaceID, conversationKey, occurrenceKey string) (bool, error) {
	scopeID := workspaceID + "/" + conversationKey
	value, _ := json.Marshal(map[string]interface{}{
		"claimed":          true,
		"plugin_id":        pluginID,
		"workspace_id":     workspaceID,
		"conversation_key": conversationKey,
	})
	claimed, err := s.state.Claim(ctx, pluginID, agentConversationOccurrenceScope, scopeID, occurrenceKey, value)
	if err != nil {
		return false, err
	}
	return !claimed, nil
}

func (s *AgentConversationService) releaseOccurrenceKey(ctx context.Context, pluginID, workspaceID, conversationKey, occurrenceKey string) error {
	scopeID := workspaceID + "/" + conversationKey
	return s.state.Delete(ctx, pluginID, agentConversationOccurrenceScope, scopeID, occurrenceKey)
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
