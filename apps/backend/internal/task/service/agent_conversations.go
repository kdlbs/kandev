package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
	metaKeyPluginID                = "kandev.plugin_id"
	metaKeyWorkspaceID             = "kandev.workspace_id"
	metaKeyConversationKey         = "kandev.conversation_key"
	metaKeyEphemeral               = "kandev.ephemeral"
	metaKeyManagedByPlugin         = models.MetaKeyManagedByPlugin
	metaKeyInstructionVer          = "kandev.instruction_version"
	metaKeyManagedRetained         = models.MetaKeyManagedRetained
	metaKeyManagedInstall          = models.MetaKeyManagedInstallationID
	metaKeyManagedInstance         = models.MetaKeyManagedInstanceKey
	metaKeyManagedRevision         = models.MetaKeyManagedConversationRevision
	metaKeyManagedPaused           = models.MetaKeyManagedConversationPaused
	metaKeyManagedDetached         = models.MetaKeyManagedConversationDetached
	metaKeyManagedApprovalRevision = models.MetaKeyManagedApprovalRevision
	metaKeyManagedManifestDigest   = models.MetaKeyManagedManifestDigest
	metaKeyManagedToolNames        = models.MetaKeyManagedAgentToolNames
	metaKeyManagedOperation        = "kandev.last_exact_operation"
	metaKeyManagedPayload          = "kandev.last_exact_payload"
	metaKeyRetentionMode           = models.MetaKeyManagedRetentionMode
)

const managedConversationRetentionMode = "retain_on_uninstall"

// defaultAgentConversationTitle is the title for managed conversation
// backing tasks. Visible only in diagnostic/admin views, never on the
// task board.
const defaultAgentConversationTitle = "Managed Conversation"

// agentConversationOccurrenceScope namespaces occurrence-key claims within
// the shared state store, separate from any plugin_state key a plugin
// writes itself through the ordinary State() RPCs.
const agentConversationOccurrenceScope = "agent_conversation_occurrence"

const (
	agentConversationOccurrencePending  = "pending"
	agentConversationOccurrenceAccepted = "accepted"
)

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
	AgentConversationStatusAlreadyApplied        = "already_applied"
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
	Get(ctx context.Context, pluginID, scope, scopeID, key string) (json.RawMessage, bool, error)
	Set(ctx context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) error
	Claim(ctx context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) (claimed bool, err error)
	Delete(ctx context.Context, pluginID, scope, scopeID, key string) error
}

// agentConversationTaskDeleter performs lifecycle-aware task deletion. The
// task repository's bare DeleteTask only removes rows; managed conversation
// cleanup must use the full task-service path when it is available so agent
// processes, worktrees, and cleanup jobs are handled consistently.
type agentConversationTaskDeleter interface {
	DeleteTask(ctx context.Context, id string) error
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

// agentConversationImmediateDispatcher is the optional race-safe direct
// prompt path used by exact managed dispatch. It must never add queue work.
type agentConversationImmediateDispatcher interface {
	DispatchImmediate(
		ctx context.Context,
		taskID string,
		session *models.TaskSession,
		text, source, idempotencyID string,
	) (pluginsdk.ManagedAgentDispatchStatus, error)
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
	deleter agentConversationTaskDeleter

	// dispatcher delivers Dispatch's text to the real agent runtime. It is
	// wired late (SetDispatcher), after the orchestrator exists — mirroring
	// pluginHost's writeDeps/utilityDeps "read live, not snapshotted"
	// pattern (internal/plugins/host.go) for the same boot-ordering reason:
	// the orchestrator is constructed after this service. Guarded by mu so
	// concurrent Dispatch calls observe a consistent value.
	mu                           sync.RWMutex
	dispatcher                   agentConversationDispatcher
	managedInputStorage          messagequeue.ManagedInputStorage
	managedInputIdentityResolver func(context.Context, string, string) (messagequeue.QueueSessionIdentity, error)
	managedInputMaxPerSession    func() int
	managedInputQueueNotifier    func(context.Context, string, string)
	managedInputExecutionStopper func(context.Context, string, string, string) (bool, error)

	dispatchLocksMu sync.Mutex
	dispatchLocks   map[string]*sync.Mutex

	// ensureLocksMu/ensureLocks serialize same-process Ensure calls while the
	// deterministic task id provides the cross-process uniqueness boundary.
	// Entries are never evicted — the key space is bounded by the number of
	// distinct managed conversations, which is small and long-lived for the
	// lifetime of the process.
	ensureLocksMu sync.Mutex
	ensureLocks   map[string]*sync.Mutex

	managedExecutionStopper func(context.Context, string) error
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

// SetTaskDeleter wires the lifecycle-aware delete path used by the main task
// service. When unset, tests and bare setups fall back to the repository's
// DeleteTask.
func (s *AgentConversationService) SetTaskDeleter(d agentConversationTaskDeleter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleter = d
}

// SetManagedExecutionStopper wires lifecycle cancellation after the shared
// orchestrator exists. The callback receives only a host-owned task identity.
func (s *AgentConversationService) SetManagedExecutionStopper(stop func(context.Context, string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedExecutionStopper = stop
}

// SetManagedInputStorage wires the durable input receipt store and the queue
// repository's authoritative session-identity resolver. maxPerSession is read
// at admission time so managed inputs use the same configured queue limit.
func (s *AgentConversationService) SetManagedInputStorage(
	storage messagequeue.ManagedInputStorage,
	resolveIdentity func(context.Context, string, string) (messagequeue.QueueSessionIdentity, error),
	maxPerSession func() int,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedInputStorage = storage
	s.managedInputIdentityResolver = resolveIdentity
	s.managedInputMaxPerSession = maxPerSession
}

// SetManagedInputNotifier wires the orchestrator wake used after durable
// admission and when a paused conversation resumes.
func (s *AgentConversationService) SetManagedInputNotifier(notify func(context.Context, string, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedInputQueueNotifier = notify
}

// SetManagedInputExecutionStopper wires exact-generation cancellation.
func (s *AgentConversationService) SetManagedInputExecutionStopper(
	stop func(context.Context, string, string, string) (bool, error),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedInputExecutionStopper = stop
}

func (s *AgentConversationService) getDispatcher() agentConversationDispatcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dispatcher
}

func (s *AgentConversationService) getTaskDeleter() agentConversationTaskDeleter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deleter
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

// lockDispatchSession keeps the observation that a conversation is idle and
// its delivery in one ownership window. The runtime changes session state
// asynchronously, so observing an idle row alone cannot prevent two distinct
// occurrences from both starting a turn.
func (s *AgentConversationService) lockDispatchSession(sessionID string) func() {
	s.dispatchLocksMu.Lock()
	if s.dispatchLocks == nil {
		s.dispatchLocks = make(map[string]*sync.Mutex)
	}
	l, ok := s.dispatchLocks[sessionID]
	if !ok {
		l = &sync.Mutex{}
		s.dispatchLocks[sessionID] = l
	}
	s.dispatchLocksMu.Unlock()
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
		ID:          conversationTaskID(pluginID, spec.WorkspaceID, spec.ConversationKey),
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
		// The deterministic backing-task id is the cross-process uniqueness
		// boundary. A competing backend can win the insert after this call's
		// metadata scan but before its insert; reload and repair that row rather
		// than surfacing a duplicate-key error or creating a second conversation.
		existing, findErr := s.findManagedConversation(ctx, pluginID, spec.WorkspaceID, spec.ConversationKey)
		if findErr == nil && existing != nil {
			return s.repairIfNeeded(ctx, existing, spec)
		}
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

// EnsureManaged creates or reconciles a retained conversation keyed by the
// host-minted installation identity. Its storage and deletion rules are
// separate from the legacy plugin-id keyed conversation lifecycle.
func (s *AgentConversationService) EnsureManaged(
	ctx context.Context,
	pluginID, installationID string,
	spec pluginsdk.ManagedAgentConversationSpec,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	spec.AgentToolNames = append([]string(nil), spec.AgentToolNames...)
	sort.Strings(spec.AgentToolNames)
	if err := validateManagedConversationIdentity(pluginID, installationID, spec, operationID, payloadDigest); err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	key := managedConversationIdentity(installationID, spec.WorkspaceID, spec.InstanceKey)
	unlock := s.lockEnsureKey(key)
	defer unlock()
	prepared, usable, err := s.prepareManagedConversationSpec(ctx, spec)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	if !usable {
		return pluginsdk.ManagedAgentConversationDescriptor{}, AgentConversationStatusConfigurationRequired, nil
	}
	spec = prepared
	existing, err := s.findRetainedManagedConversation(ctx, installationID, spec.WorkspaceID, spec.InstanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	if existing != nil {
		return s.reconcileManagedConversation(ctx, pluginID, installationID, existing, spec, operationID, payloadDigest)
	}
	return s.createManagedConversation(ctx, pluginID, installationID, spec, operationID, payloadDigest)
}

func validateManagedConversationIdentity(
	pluginID, installationID string,
	spec pluginsdk.ManagedAgentConversationSpec,
	operationID, payloadDigest string,
) error {
	if pluginID == "" || installationID == "" || spec.WorkspaceID == "" || spec.InstanceKey == "" ||
		strings.TrimSpace(spec.InstanceKey) != spec.InstanceKey || len(spec.InstanceKey) > 128 || operationID == "" || payloadDigest == "" {
		return status.Error(codes.InvalidArgument, "installation, workspace, and instance key are required")
	}
	if spec.ApprovalRevision == 0 || len(spec.ManifestDigest) != 64 || mcpprofile.ValidateManagedToolNames(spec.AgentToolNames) != nil {
		return status.Error(codes.InvalidArgument, "managed conversation tool policy is invalid")
	}
	return nil
}

func (s *AgentConversationService) prepareManagedConversationSpec(
	ctx context.Context,
	spec pluginsdk.ManagedAgentConversationSpec,
) (pluginsdk.ManagedAgentConversationSpec, bool, error) {
	workspace, err := s.tasks.GetWorkspace(ctx, spec.WorkspaceID)
	if err != nil {
		return spec, false, fmt.Errorf("failed to resolve managed conversation workspace: %w", err)
	}
	if workspace == nil {
		return spec, false, status.Error(codes.NotFound, "workspace not found")
	}
	profileID, usable, err := s.resolveEffectiveProfile(ctx, spec.WorkspaceID, spec.AgentProfileID)
	spec.AgentProfileID = profileID
	return spec, usable, err
}

func (s *AgentConversationService) createManagedConversation(
	ctx context.Context,
	pluginID, installationID string,
	spec pluginsdk.ManagedAgentConversationSpec,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	if spec.ExpectedRevision != 0 {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", status.Error(codes.Aborted, "managed conversation revision is stale")
	}
	conversation := s.newManagedConversationTask(pluginID, installationID, spec, operationID, payloadDigest)
	if err := s.tasks.CreateTask(ctx, conversation); err != nil {
		existing, findErr := s.findRetainedManagedConversation(ctx, installationID, spec.WorkspaceID, spec.InstanceKey)
		if findErr == nil && existing != nil {
			return s.reconcileManagedConversation(ctx, pluginID, installationID, existing, spec, operationID, payloadDigest)
		}
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", fmt.Errorf("failed to insert managed conversation task: %w", err)
	}
	primary, err := s.createManagedPrimarySession(ctx, conversation.ID, spec)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", fmt.Errorf("failed to create managed conversation session: %w", err)
	}
	s.publishTaskCreated(ctx, conversation)
	return managedConversationDescriptor(installationID, conversation, primary), AgentConversationStatusCreated, nil
}

// GetManaged returns a conversation only when both its installation and
// workspace match the request.
func (s *AgentConversationService) GetManaged(
	ctx context.Context, installationID, workspaceID, instanceKey string,
) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if task == nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	if managedConversationDetached(task) {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	return managedConversationDescriptor(installationID, task, primary), nil
}

// ListManaged returns installation-owned conversations for one workspace in
// stable task-id order.
func (s *AgentConversationService) ListManaged(
	ctx context.Context, installationID, workspaceID string,
) ([]pluginsdk.ManagedAgentConversationDescriptor, error) {
	if installationID == "" || workspaceID == "" {
		return nil, status.Error(codes.InvalidArgument, "installation and workspace are required")
	}
	tasks, err := s.listRetainedManagedConversations(ctx, installationID, workspaceID)
	if err != nil {
		return nil, err
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	out := make([]pluginsdk.ManagedAgentConversationDescriptor, 0, len(tasks))
	for _, task := range tasks {
		if managedConversationDetached(task) {
			continue
		}
		primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
		if errors.Is(err, taskrepo.ErrNoPrimarySession) {
			primary = nil
		} else if err != nil {
			return nil, err
		}
		out = append(out, managedConversationDescriptor(installationID, task, primary))
	}
	return out, nil
}

// SetManagedPaused changes only the desired admission state. Stopping a live
// generation is a separate execution-control operation.
//
//nolint:cyclop // Pause changes update desired state and conversation revision as one command.
func (s *AgentConversationService) SetManagedPaused(
	ctx context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, paused bool, operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID, instanceKey))
	defer unlock()
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if task == nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	if managedConversationDetached(task) {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	if operationID == "" || payloadDigest == "" {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.InvalidArgument, "managed conversation operation identity is required")
	}
	if managedConversationOperationMatches(task, operationID, payloadDigest) {
		primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
		if errors.Is(err, taskrepo.ErrNoPrimarySession) || primary == nil {
			spec := managedConversationSpecFromTask(task)
			primary, err = s.createManagedPrimarySession(ctx, task.ID, spec)
		}
		if err != nil {
			return pluginsdk.ManagedAgentConversationDescriptor{}, err
		}
		descriptor := managedConversationDescriptor(installationID, task, primary)
		if !paused && primary != nil {
			s.notifyManagedInputQueue(ctx, task.ID, primary.ID)
		}
		return descriptor, nil
	}
	revision := managedConversationRevision(task)
	if revision != expectedRevision {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Aborted, "managed conversation revision is stale")
	}
	if managedConversationPaused(task) != paused {
		setManagedConversationValue(task, metaKeyManagedPaused, paused)
		setManagedConversationValue(task, metaKeyManagedRevision, strconv.FormatUint(revision+1, 10))
	}
	setManagedConversationValue(task, metaKeyManagedOperation, operationID)
	setManagedConversationValue(task, metaKeyManagedPayload, payloadDigest)
	task.UpdatedAt = time.Now().UTC()
	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, fmt.Errorf("failed to save managed conversation pause state: %w", err)
	}
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	descriptor := managedConversationDescriptor(installationID, task, primary)
	if !paused && primary != nil {
		s.notifyManagedInputQueue(ctx, task.ID, primary.ID)
	}
	return descriptor, nil
}

// DeleteManaged removes one retained conversation only at the expected
// revision. Host lifecycle cleanup deliberately does not call this method.
func (s *AgentConversationService) DeleteManaged(
	ctx context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, _, _ string,
) error {
	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID, instanceKey))
	defer unlock()
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return err
	}
	if task == nil {
		return status.Error(codes.NotFound, "managed conversation not found")
	}
	if managedConversationDetached(task) {
		return status.Error(codes.NotFound, "managed conversation not found")
	}
	if managedConversationRevision(task) != expectedRevision {
		return status.Error(codes.Aborted, "managed conversation revision is stale")
	}
	if err := s.deleteManagedConversationTask(ctx, s.getTaskDeleter(), task.ID); err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete managed conversation: %w", err)
	}
	return nil
}

// PauseManagedForInstallation blocks new turns and stops any current execution
// while preserving queued work and the host-owned transcript.
func (s *AgentConversationService) PauseManagedForInstallation(ctx context.Context, installationID string) error {
	if installationID == "" {
		return status.Error(codes.InvalidArgument, "installation_id is required")
	}
	tasks, err := s.tasks.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to list retained conversations for pause: %w", err)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	for _, candidate := range tasks {
		if candidate == nil || candidate.Metadata == nil {
			continue
		}
		workspaceID, _ := candidate.Metadata[metaKeyWorkspaceID].(string)
		if !isRetainedManagedConversation(candidate, installationID, workspaceID, "") {
			continue
		}
		unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID,
			models.StringFromAny(candidate.Metadata[metaKeyManagedInstance])))
		task, findErr := s.findRetainedManagedConversation(ctx, installationID, workspaceID,
			models.StringFromAny(candidate.Metadata[metaKeyManagedInstance]))
		if findErr != nil {
			unlock()
			return findErr
		}
		if task == nil || managedConversationDetached(task) {
			unlock()
			continue
		}
		if !managedConversationPaused(task) {
			setManagedConversationValue(task, metaKeyManagedPaused, true)
			setManagedConversationValue(task, metaKeyManagedRevision,
				strconv.FormatUint(managedConversationRevision(task)+1, 10))
			task.UpdatedAt = time.Now().UTC()
			if err := s.tasks.UpdateTask(ctx, task); err != nil {
				unlock()
				return fmt.Errorf("failed to pause managed conversation %s: %w", task.ID, err)
			}
		}
		stopErr := s.stopManagedConversationExecution(ctx, task)
		unlock()
		if stopErr != nil {
			return stopErr
		}
	}
	return nil
}

// InvalidateManagedForInstallationWorkspace blocks new turns for every
// retained conversation whose approval revision has changed, and stops any
// live generation through the normal runtime lifecycle.
func (s *AgentConversationService) InvalidateManagedForInstallationWorkspace(ctx context.Context, installationID, workspaceID string) error {
	if installationID == "" || workspaceID == "" {
		return status.Error(codes.InvalidArgument, "installation_id and workspace_id are required")
	}
	tasks, err := s.tasks.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to list retained conversations for policy invalidation: %w", err)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	var invalidationErrors []error
	for _, candidate := range tasks {
		if candidate == nil || candidate.Metadata == nil || models.StringFromAny(candidate.Metadata[metaKeyWorkspaceID]) != workspaceID {
			continue
		}
		instanceKey := models.StringFromAny(candidate.Metadata[metaKeyManagedInstance])
		if !isRetainedManagedConversation(candidate, installationID, workspaceID, instanceKey) {
			continue
		}
		unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID, instanceKey))
		current, findErr := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
		if findErr != nil {
			unlock()
			invalidationErrors = append(invalidationErrors, findErr)
			continue
		}
		if current == nil || managedConversationDetached(current) {
			unlock()
			continue
		}
		if invalidated, _ := current.Metadata[models.MetaKeyManagedPolicyInvalidated].(bool); !invalidated {
			setManagedConversationValue(current, models.MetaKeyManagedPolicyInvalidated, true)
			setManagedConversationValue(current, metaKeyManagedRevision,
				strconv.FormatUint(managedConversationRevision(current)+1, 10))
			setManagedConversationValue(current, metaKeyManagedOperation, "")
			setManagedConversationValue(current, metaKeyManagedPayload, "")
			current.UpdatedAt = time.Now().UTC()
			if updateErr := s.tasks.UpdateTask(ctx, current); updateErr != nil {
				invalidationErrors = append(invalidationErrors, fmt.Errorf("failed to invalidate managed conversation %s: %w", current.ID, updateErr))
			}
		}
		if stopErr := s.stopManagedConversationExecution(ctx, current); stopErr != nil {
			invalidationErrors = append(invalidationErrors, stopErr)
		}
		unlock()
	}
	return errors.Join(invalidationErrors...)
}

// DetachManagedForInstallation revokes execution ownership after uninstall
// while preserving a paused host-owned transcript for native read-only access.
func (s *AgentConversationService) DetachManagedForInstallation(ctx context.Context, installationID string) error {
	if installationID == "" {
		return status.Error(codes.InvalidArgument, "installation_id is required")
	}
	tasks, err := s.tasks.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to list retained conversations for uninstall: %w", err)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	for _, task := range tasks {
		if task == nil || task.Metadata == nil {
			continue
		}
		workspaceID, _ := task.Metadata[metaKeyWorkspaceID].(string)
		if !isRetainedManagedConversation(task, installationID, workspaceID, "") {
			continue
		}
		instanceKey := models.StringFromAny(task.Metadata[metaKeyManagedInstance])
		unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID, instanceKey))
		current, findErr := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
		if findErr != nil {
			unlock()
			return findErr
		}
		if current == nil {
			unlock()
			continue
		}
		if !managedConversationDetached(current) {
			setManagedConversationValue(current, metaKeyManagedDetached, true)
			setManagedConversationValue(current, metaKeyManagedPaused, true)
			setManagedConversationValue(current, metaKeyManagedRevision,
				strconv.FormatUint(managedConversationRevision(current)+1, 10))
			current.UpdatedAt = time.Now().UTC()
			if err := s.tasks.UpdateTask(ctx, current); err != nil {
				unlock()
				return fmt.Errorf("failed to detach managed conversation %s: %w", current.ID, err)
			}
		}
		stopErr := s.stopManagedConversationExecution(ctx, current)
		unlock()
		if stopErr != nil {
			return stopErr
		}
	}
	return nil
}

func (s *AgentConversationService) stopManagedConversationExecution(ctx context.Context, task *models.Task) error {
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
	if errors.Is(err, taskrepo.ErrNoPrimarySession) || (err == nil && primary == nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to inspect managed conversation session %s: %w", task.ID, err)
	}
	if primary.State != models.TaskSessionStateRunning && primary.State != models.TaskSessionStateStarting && primary.AgentExecutionID == "" {
		return nil
	}
	s.mu.RLock()
	stop := s.managedExecutionStopper
	s.mu.RUnlock()
	if stop == nil {
		return status.Error(codes.Unavailable, "managed conversation execution stopper is unavailable")
	}
	if err := stop(ctx, task.ID); err != nil {
		return fmt.Errorf("failed to stop managed conversation execution %s: %w", task.ID, err)
	}
	return nil
}

func (s *AgentConversationService) newManagedConversationTask(
	pluginID, installationID string, spec pluginsdk.ManagedAgentConversationSpec, operationID, payloadDigest string,
) *models.Task {
	metadata := map[string]interface{}{
		metaKeyPluginID: pluginID, metaKeyWorkspaceID: spec.WorkspaceID,
		metaKeyConversationKey: spec.InstanceKey, metaKeyEphemeral: true,
		metaKeyManagedByPlugin: pluginID, metaKeyManagedRetained: true,
		metaKeyManagedInstall: installationID, metaKeyManagedInstance: spec.InstanceKey,
		metaKeyManagedRevision: "1", metaKeyManagedPaused: false, models.MetaKeyManagedConversationDetached: false,
		metaKeyManagedApprovalRevision:  strconv.FormatUint(spec.ApprovalRevision, 10),
		metaKeyManagedManifestDigest:    spec.ManifestDigest,
		metaKeyManagedToolNames:         append([]string(nil), spec.AgentToolNames...),
		metaKeyRetentionMode:            managedConversationRetentionMode,
		metaKeyManagedOperation:         operationID,
		metaKeyManagedPayload:           payloadDigest,
		models.MetaKeyAgentProfileID:    spec.AgentProfileID,
		models.MetaKeyExecutorID:        spec.ExecutorID,
		models.MetaKeyExecutorProfileID: spec.ExecutorProfileID,
		metaKeyInstructionVer:           spec.InstructionVersion,
	}
	if spec.BasePrompt != "" {
		metadata["kandev.base_prompt"] = spec.BasePrompt
	}
	return &models.Task{
		ID:          managedConversationTaskID(installationID, spec.WorkspaceID, spec.InstanceKey),
		WorkspaceID: spec.WorkspaceID,
		Title:       defaultAgentConversationTitle + " - " + spec.InstanceKey,
		State:       v1.TaskStateCreated, Priority: "medium", IsEphemeral: true,
		Origin: models.TaskOriginManual, Metadata: metadata,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func (s *AgentConversationService) createManagedPrimarySession(
	ctx context.Context, taskID string, spec pluginsdk.ManagedAgentConversationSpec,
) (*models.TaskSession, error) {
	primary := &models.TaskSession{
		ID: conversationPrimarySessionID(taskID), TaskID: taskID,
		AgentProfileID: spec.AgentProfileID, ExecutorID: spec.ExecutorID,
		ExecutorProfileID: spec.ExecutorProfileID, State: models.TaskSessionStateCreated,
		IsPrimary: true, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.sess.CreateTaskSession(ctx, primary); err != nil {
		return nil, err
	}
	return primary, nil
}

func (s *AgentConversationService) reconcileManagedConversation(
	ctx context.Context,
	pluginID, installationID string,
	task *models.Task,
	spec pluginsdk.ManagedAgentConversationSpec,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	if !isCurrentManagedConversation(task, installationID, spec) {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", status.Error(codes.NotFound, "managed conversation not found")
	}
	currentRevision := managedConversationRevision(task)
	primary, err := s.managedConversationPrimary(ctx, task.ID)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	if managedConversationOperationMatches(task, operationID, payloadDigest) {
		primary, err = s.repairManagedConversationSession(ctx, task, spec, primary)
		if err != nil {
			return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
		}
		return managedConversationDescriptor(installationID, task, primary), AgentConversationStatusAlreadyApplied, nil
	}
	if currentRevision != spec.ExpectedRevision {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", status.Error(codes.Aborted, "managed conversation revision is stale")
	}
	changed := managedConversationConfigChanged(task, spec)
	if changed && !managedConversationSessionIdle(primary) {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", status.Error(codes.FailedPrecondition, "managed conversation launch settings can change only while idle")
	}
	primary, created, err := s.ensureManagedConversationSession(ctx, task, spec, primary, changed)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	changed = changed || created
	if changed {
		if err := s.persistManagedConversationSpec(ctx, task, pluginID, spec, currentRevision, operationID, payloadDigest); err != nil {
			return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
		}
	}
	return managedConversationDescriptor(installationID, task, primary), AgentConversationStatusExists, nil
}

func isCurrentManagedConversation(task *models.Task, installationID string, spec pluginsdk.ManagedAgentConversationSpec) bool {
	return isRetainedManagedConversation(task, installationID, spec.WorkspaceID, spec.InstanceKey) &&
		!managedConversationDetached(task)
}

func (s *AgentConversationService) managedConversationPrimary(ctx context.Context, taskID string) (*models.TaskSession, error) {
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, taskID)
	if errors.Is(err, taskrepo.ErrNoPrimarySession) {
		return nil, nil
	}
	return primary, err
}

func (s *AgentConversationService) repairManagedConversationSession(
	ctx context.Context,
	task *models.Task,
	spec pluginsdk.ManagedAgentConversationSpec,
	primary *models.TaskSession,
) (*models.TaskSession, error) {
	if primary != nil {
		return primary, nil
	}
	primary, err := s.createManagedPrimarySession(ctx, task.ID, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to repair managed conversation session: %w", err)
	}
	return primary, nil
}

func managedConversationSessionIdle(primary *models.TaskSession) bool {
	return primary == nil || (primary.State != models.TaskSessionStateRunning &&
		primary.State != models.TaskSessionStateStarting && primary.AgentExecutionID == "")
}

func (s *AgentConversationService) ensureManagedConversationSession(
	ctx context.Context,
	task *models.Task,
	spec pluginsdk.ManagedAgentConversationSpec,
	primary *models.TaskSession,
	changed bool,
) (*models.TaskSession, bool, error) {
	if primary == nil {
		created, err := s.createManagedPrimarySession(ctx, task.ID, spec)
		if err != nil {
			return nil, false, fmt.Errorf("failed to repair managed conversation session: %w", err)
		}
		return created, true, nil
	}
	if !changed {
		return primary, false, nil
	}
	primary.AgentProfileID, primary.ExecutorID, primary.ExecutorProfileID = spec.AgentProfileID, spec.ExecutorID, spec.ExecutorProfileID
	primary.UpdatedAt = time.Now().UTC()
	if err := s.sess.UpdateTaskSession(ctx, primary); err != nil {
		return nil, false, fmt.Errorf("failed to update managed conversation launch settings: %w", err)
	}
	return primary, false, nil
}

func (s *AgentConversationService) persistManagedConversationSpec(
	ctx context.Context,
	task *models.Task,
	pluginID string,
	spec pluginsdk.ManagedAgentConversationSpec,
	currentRevision uint64,
	operationID, payloadDigest string,
) error {
	setManagedConversationValue(task, models.MetaKeyAgentProfileID, spec.AgentProfileID)
	setManagedConversationValue(task, models.MetaKeyExecutorID, spec.ExecutorID)
	setManagedConversationValue(task, models.MetaKeyExecutorProfileID, spec.ExecutorProfileID)
	setManagedConversationValue(task, "kandev.base_prompt", spec.BasePrompt)
	setManagedConversationValue(task, metaKeyInstructionVer, spec.InstructionVersion)
	setManagedConversationValue(task, metaKeyManagedByPlugin, pluginID)
	setManagedConversationValue(task, metaKeyManagedApprovalRevision, strconv.FormatUint(spec.ApprovalRevision, 10))
	setManagedConversationValue(task, metaKeyManagedManifestDigest, spec.ManifestDigest)
	setManagedConversationValue(task, metaKeyManagedToolNames, append([]string(nil), spec.AgentToolNames...))
	setManagedConversationValue(task, models.MetaKeyManagedPolicyInvalidated, false)
	setManagedConversationValue(task, metaKeyManagedRevision, strconv.FormatUint(currentRevision+1, 10))
	setManagedConversationValue(task, metaKeyManagedOperation, operationID)
	setManagedConversationValue(task, metaKeyManagedPayload, payloadDigest)
	task.UpdatedAt = time.Now().UTC()
	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to persist managed conversation settings: %w", err)
	}
	return nil
}

func (s *AgentConversationService) findRetainedManagedConversation(
	ctx context.Context, installationID, workspaceID, instanceKey string,
) (*models.Task, error) {
	var found *models.Task
	err := s.eachEphemeralTask(ctx, workspaceID, func(task *models.Task) bool {
		if isRetainedManagedConversation(task, installationID, workspaceID, instanceKey) {
			found = task
			return false
		}
		return true
	})
	return found, err
}

func (s *AgentConversationService) listRetainedManagedConversations(
	ctx context.Context, installationID, workspaceID string,
) ([]*models.Task, error) {
	var found []*models.Task
	err := s.eachEphemeralTask(ctx, workspaceID, func(task *models.Task) bool {
		if isRetainedManagedConversation(task, installationID, workspaceID, "") {
			found = append(found, task)
		}
		return true
	})
	return found, err
}

func managedConversationOperationMatches(task *models.Task, operationID, payloadDigest string) bool {
	if task == nil || task.Metadata == nil || operationID == "" || payloadDigest == "" {
		return false
	}
	return models.StringFromAny(task.Metadata[metaKeyManagedOperation]) == operationID &&
		models.StringFromAny(task.Metadata[metaKeyManagedPayload]) == payloadDigest
}

func managedConversationSpecFromTask(task *models.Task) pluginsdk.ManagedAgentConversationSpec {
	metadata := task.Metadata
	return pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: task.WorkspaceID, InstanceKey: models.StringFromAny(metadata[metaKeyManagedInstance]),
		ApprovalRevision:   managedConversationUint(metadata[metaKeyManagedApprovalRevision]),
		ManifestDigest:     models.StringFromAny(metadata[metaKeyManagedManifestDigest]),
		AgentToolNames:     managedConversationToolNames(metadata[metaKeyManagedToolNames]),
		AgentProfileID:     models.StringFromAny(metadata[models.MetaKeyAgentProfileID]),
		ExecutorID:         models.StringFromAny(metadata[models.MetaKeyExecutorID]),
		ExecutorProfileID:  models.StringFromAny(metadata[models.MetaKeyExecutorProfileID]),
		BasePrompt:         models.StringFromAny(metadata["kandev.base_prompt"]),
		InstructionVersion: models.StringFromAny(metadata[metaKeyInstructionVer]),
	}
}

func (s *AgentConversationService) eachEphemeralTask(ctx context.Context, workspaceID string, visit func(*models.Task) bool) error {
	if workspaceID == "" {
		return status.Error(codes.InvalidArgument, "workspace_id is required")
	}
	for page := 1; ; page++ {
		tasks, total, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, "", "", "", page, managedConversationPageSize, "", false, true, true, false)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if !visit(task) {
				return nil
			}
		}
		if len(tasks) < managedConversationPageSize || page*managedConversationPageSize >= total {
			return nil
		}
	}
}

func managedConversationConfigChanged(task *models.Task, spec pluginsdk.ManagedAgentConversationSpec) bool {
	metadata := task.Metadata
	return models.StringFromAny(metadata[models.MetaKeyAgentProfileID]) != spec.AgentProfileID ||
		models.StringFromAny(metadata[models.MetaKeyExecutorID]) != spec.ExecutorID ||
		models.StringFromAny(metadata[models.MetaKeyExecutorProfileID]) != spec.ExecutorProfileID ||
		models.StringFromAny(metadata["kandev.base_prompt"]) != spec.BasePrompt ||
		models.StringFromAny(metadata[metaKeyInstructionVer]) != spec.InstructionVersion ||
		managedConversationUint(metadata[metaKeyManagedApprovalRevision]) != spec.ApprovalRevision ||
		models.StringFromAny(metadata[metaKeyManagedManifestDigest]) != spec.ManifestDigest ||
		!slices.Equal(managedConversationToolNames(metadata[metaKeyManagedToolNames]), spec.AgentToolNames) ||
		metadata[models.MetaKeyManagedPolicyInvalidated] == true
}

func managedConversationDescriptor(
	installationID string, task *models.Task, session *models.TaskSession,
) pluginsdk.ManagedAgentConversationDescriptor {
	metadata := task.Metadata
	descriptor := pluginsdk.ManagedAgentConversationDescriptor{
		InstallationID: installationID, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		InstanceKey:        models.StringFromAny(metadata[metaKeyManagedInstance]),
		Revision:           managedConversationRevision(task),
		AgentProfileID:     models.StringFromAny(metadata[models.MetaKeyAgentProfileID]),
		ExecutorID:         models.StringFromAny(metadata[models.MetaKeyExecutorID]),
		ExecutorProfileID:  models.StringFromAny(metadata[models.MetaKeyExecutorProfileID]),
		BasePrompt:         models.StringFromAny(metadata["kandev.base_prompt"]),
		InstructionVersion: models.StringFromAny(metadata[metaKeyInstructionVer]),
		DesiredPaused:      managedConversationPaused(task),
		RetentionMode:      models.StringFromAny(metadata[metaKeyRetentionMode]),
		Detached:           managedConversationDetached(task),
		AgentToolNames:     managedConversationToolNames(metadata[metaKeyManagedToolNames]),
	}
	if session != nil {
		descriptor.SessionID = session.ID
	}
	return descriptor
}

func managedConversationUint(value any) uint64 {
	parsed, _ := strconv.ParseUint(models.StringFromAny(value), 10, 64)
	return parsed
}

func managedConversationToolNames(value any) []string {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(encoded, &names); err != nil {
		return nil
	}
	sort.Strings(names)
	return names
}

func managedConversationRevision(task *models.Task) uint64 {
	revision, err := strconv.ParseUint(models.StringFromAny(task.Metadata[metaKeyManagedRevision]), 10, 64)
	if err != nil || revision == 0 {
		return 1
	}
	return revision
}

func managedConversationPaused(task *models.Task) bool {
	paused, _ := task.Metadata[metaKeyManagedPaused].(bool)
	return paused
}

func managedConversationDetached(task *models.Task) bool {
	detached, _ := task.Metadata[metaKeyManagedDetached].(bool)
	return detached
}

func setManagedConversationValue(task *models.Task, key string, value interface{}) {
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	if value == "" {
		delete(task.Metadata, key)
		return
	}
	task.Metadata[key] = value
}

func managedConversationTaskID(installationID, workspaceID, instanceKey string) string {
	return conversationIdentity("managed-task", installationID, workspaceID, instanceKey)
}

func managedConversationIdentity(installationID, workspaceID, instanceKey string) string {
	return installationID + "/" + workspaceID + "/" + instanceKey
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
	primary, err := s.ensureConversationPrimarySession(ctx, existing.ID, profileID)
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	if err := s.reconcileConversationTask(ctx, existing, profileID, spec.BasePrompt); err != nil {
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
	if errors.Is(err, taskrepo.ErrNoPrimarySession) {
		// The task row can survive a partial creation failure. Treat the typed
		// repository result as a repairable absence, while preserving all other
		// repository errors as failures.
		primary = nil
	} else if err != nil {
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
	if primary.State == models.TaskSessionStateRunning || primary.State == models.TaskSessionStateStarting || primary.AgentExecutionID != "" {
		return nil, status.Error(codes.FailedPrecondition, "conversation session has a live agent execution; delete and recreate it before changing agent profile")
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
		ID:             conversationPrimarySessionID(taskID),
		TaskID:         taskID,
		AgentProfileID: agentProfileID,
		State:          models.TaskSessionStateCreated,
		IsPrimary:      true,
		StartedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := s.sess.CreateTaskSession(ctx, primary); err != nil {
		existing, findErr := s.sess.GetPrimarySessionByTaskID(ctx, taskID)
		if findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	return primary, nil
}

func conversationTaskID(pluginID, workspaceID, conversationKey string) string {
	return conversationIdentity("task", pluginID, workspaceID, conversationKey)
}

func conversationPrimarySessionID(taskID string) string {
	return conversationIdentity("primary-session", taskID)
}

func conversationIdentity(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return uuid.NewSHA1(uuid.NameSpaceOID, encoded).String()
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

	unlock := s.lockDispatchSession(pluginID + "/" + workspaceID + "/" + conversationKey)
	defer unlock()

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
		return agentConversationDispatch(existing, primary, workspaceID, conversationKey, "duplicate_occurrence"), nil
	}

	// 5. Deliver through the real agent runtime (start a never-launched
	// session, or prompt/resume an idle one).
	deliverStatus, err := s.deliverConversationPrompt(ctx, dispatcher, existing, primary, text, pluginID, workspaceID, conversationKey, idempotencyID, claimedOccurrence, occurrenceKey)
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	if claimedOccurrence {
		if err := s.acceptOccurrenceKey(ctx, pluginID, workspaceID, conversationKey, occurrenceKey); err != nil {
			return pluginsdk.AgentConversationDispatch{}, fmt.Errorf("conversation prompt delivered but failed to record accepted occurrence: %w", err)
		}
	}

	return agentConversationDispatch(existing, primary, workspaceID, conversationKey, deliverStatus), nil
}

func agentConversationDispatch(task *models.Task, session *models.TaskSession, workspaceID, conversationKey, dispatchStatus string) pluginsdk.AgentConversationDispatch {
	return pluginsdk.AgentConversationDispatch{
		SessionID: session.ID,
		Status:    dispatchStatus,
		Descriptor: pluginsdk.AgentConversationDescriptor{
			TaskID:          task.ID,
			SessionID:       session.ID,
			WorkspaceID:     workspaceID,
			ConversationKey: conversationKey,
		},
	}
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
	idempotencyID := deriveOccurrenceMessageID(pluginID, workspaceID, conversationKey, occurrenceKey)
	if !alreadyClaimed {
		return idempotencyID, true, false, nil
	}

	scopeID := workspaceID + "/" + conversationKey
	raw, found, err := s.state.Get(ctx, pluginID, agentConversationOccurrenceScope, scopeID, occurrenceKey)
	if err != nil {
		return "", false, false, fmt.Errorf("failed to inspect occurrence key: %w", err)
	}
	if !found {
		// A failed delivery may have released the row between Claim and Get.
		// Re-run the atomic insert once so that a retry can own the occurrence
		// without treating a transient absence as a duplicate.
		claimed, claimErr := s.claimOccurrenceKey(ctx, pluginID, workspaceID, conversationKey, occurrenceKey)
		if claimErr != nil {
			return "", false, false, fmt.Errorf("failed to reclaim occurrence key: %w", claimErr)
		}
		if claimed {
			return idempotencyID, true, false, nil
		}
		return "", false, true, nil
	}
	if occurrenceStatus(raw) == agentConversationOccurrencePending {
		// The previous owner persisted intent but did not persist acceptance.
		// Re-enter delivery with the same deterministic message identity. No
		// expiry is used, so an accepted occurrence cannot be reclaimed blindly.
		return idempotencyID, true, false, nil
	}
	return "", false, true, nil
}

func (s *AgentConversationService) deliverConversationPrompt(ctx context.Context, dispatcher agentConversationDispatcher, task *models.Task, primary *models.TaskSession, text, pluginID, workspaceID, conversationKey, idempotencyID string, claimedOccurrence bool, occurrenceKey string) (string, error) {
	deliverStatus, err := dispatcher.Deliver(ctx, task.ID, primary, composeConversationPrompt(task, text), "plugin:"+pluginID, idempotencyID)
	if err == nil {
		return deliverStatus, nil
	}
	if !claimedOccurrence {
		return "", fmt.Errorf("failed to deliver message: %w", err)
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if releaseErr := s.releaseOccurrenceKey(cleanupCtx, pluginID, workspaceID, conversationKey, occurrenceKey); releaseErr != nil {
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
	return conversationIdentity("occurrence-message", pluginID, workspaceID, conversationKey, occurrenceKey)
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
	deleter := s.getTaskDeleter()
	for _, t := range tasks {
		if err := s.deleteManagedConversationTask(ctx, deleter, t.ID); err != nil {
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
	deleter := s.getTaskDeleter()
	for _, task := range tasks {
		if !isManagedConversationOwnedByPlugin(task, pluginID) {
			continue
		}
		if err := s.deleteManagedConversationTask(ctx, deleter, task.ID); err != nil {
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

func (s *AgentConversationService) deleteManagedConversationTask(ctx context.Context, deleter agentConversationTaskDeleter, taskID string) error {
	if deleter != nil {
		return deleter.DeleteTask(ctx, taskID)
	}
	return s.tasks.DeleteTask(ctx, taskID)
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
	if retained, _ := task.Metadata[metaKeyManagedRetained].(bool); retained {
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
	if retained, _ := task.Metadata[metaKeyManagedRetained].(bool); retained {
		return false
	}
	pID, _ := task.Metadata[metaKeyPluginID].(string)
	ephemeral, _ := task.Metadata[metaKeyEphemeral].(bool)
	return pID == pluginID && ephemeral
}

func isRetainedManagedConversation(task *models.Task, installationID, workspaceID, instanceKey string) bool {
	if task == nil || task.Metadata == nil || installationID == "" || workspaceID == "" {
		return false
	}
	retained, _ := task.Metadata[metaKeyManagedRetained].(bool)
	ephemeral, _ := task.Metadata[metaKeyEphemeral].(bool)
	installedBy, _ := task.Metadata[metaKeyManagedInstall].(string)
	workspace, _ := task.Metadata[metaKeyWorkspaceID].(string)
	instance, _ := task.Metadata[metaKeyManagedInstance].(string)
	return retained && ephemeral && installedBy == installationID && workspace == workspaceID &&
		(instanceKey == "" || instance == instanceKey)
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
		"status":           agentConversationOccurrencePending,
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

func (s *AgentConversationService) acceptOccurrenceKey(ctx context.Context, pluginID, workspaceID, conversationKey, occurrenceKey string) error {
	if occurrenceKey == "" {
		return nil
	}
	scopeID := workspaceID + "/" + conversationKey
	value, err := json.Marshal(map[string]interface{}{
		"status":           agentConversationOccurrenceAccepted,
		"plugin_id":        pluginID,
		"workspace_id":     workspaceID,
		"conversation_key": conversationKey,
	})
	if err != nil {
		return err
	}
	return s.state.Set(ctx, pluginID, agentConversationOccurrenceScope, scopeID, occurrenceKey, value)
}

func occurrenceStatus(raw json.RawMessage) string {
	var value struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		// Rows written by older releases have no status. Treat malformed or
		// legacy rows as accepted so a migration never replays an occurrence.
		return agentConversationOccurrenceAccepted
	}
	if value.Status == "" {
		return agentConversationOccurrenceAccepted
	}
	return value.Status
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
