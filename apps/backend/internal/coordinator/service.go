package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// DecisionTaskService is the narrow task-service surface the approve/reject
// decisions (task-07) need: reading a candidate spec's referenced workflow,
// repository and source task for validation, and creating/settling/looking
// up the task an approval produces. Reached through a narrow interface, like
// WorkspaceAuthorizer, so this package does not depend on task/service's
// full surface. Satisfied by *taskservice.Service.
type DecisionTaskService interface {
	GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error)
	GetRepository(ctx context.Context, id string) (*taskmodels.Repository, error)
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	CreateTask(ctx context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error)
	SettleExternalID(ctx context.Context, taskID, externalID string) (bool, *taskmodels.Task, error)
	GetTaskByExternalID(ctx context.Context, workspaceID, externalID string) (*taskmodels.Task, error)
}

// WorkspaceAuthorizer is the workspace-scope check every coordinator route
// needs (docs/specs/coordinator/system-design/coordinators.md#routes):
// workspace.read for reads, workspace.manage for writes. Reached through a
// narrow interface so this package does not depend on task/service's full
// surface. Satisfied by the task service.
type WorkspaceAuthorizer interface {
	AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error
}

// ConversationClearedHook is invoked after a PATCH commits a change that
// cleared conversation_task_id, naming the coordinator and the task id that
// was cleared (coordinators.md#routes, Build decision 7). nil by default:
// WP-1 registers no hook, so the call is a no-op until a later work package
// wires one through SetConversationHooks.
type ConversationClearedHook func(ctx context.Context, coordinatorID, oldConversationTaskID string)

// CoordinatorDeletedHook is invoked after a coordinator and its proposals are
// deleted (Build decision 8), naming the workspace so a registered hook can
// enumerate and delete the coordinator's conversation tasks
// (copilot.md#conversation-cleanup).
type CoordinatorDeletedHook func(ctx context.Context, workspaceID, coordinatorID string)

// CoordinatorWithOpenProposals pairs a coordinator with its open proposal
// count, as the list route needs (coordinators.md#routes, Build decision 9).
type CoordinatorWithOpenProposals struct {
	Coordinator   *Coordinator
	OpenProposals int
}

// Service implements the coordinator CRUD, proposals-read and stalls-read
// routes (docs/plans/workspace-coordinator/task-01-shared-interface.md). The
// conversation, approve/reject, and subscriber routes are added by later work
// packages on the same Store.
type Service struct {
	// kinds is the registry of non-create proposal kinds; executeTimeout bounds one Execute.
	kinds          map[string]KindExecutor
	kindDeps       KindDeps
	executeTimeout time.Duration
	store          *Store
	validator      *Validator
	authz          WorkspaceAuthorizer
	logger         *logger.Logger

	onConversationCleared ConversationClearedHook
	onCoordinatorDeleted  CoordinatorDeletedHook

	proposalWorkflows    WorkflowReader
	proposalRepositories RepositoryReader
	proposalTasks        SourceTaskReader
	proposalSteps        WorkflowStepReader

	conversationTasks    ConversationTaskManager
	conversationSessions SessionEnsurer

	// decisionTasks, decisionSteps and eventBus back Approve and Reject
	// (task-07). Wired by SetDecisionDeps; nil until the decisions
	// registration function calls it. See docs/specs/coordinator/
	// system-design/proposals.md#approve.
	decisionTasks DecisionTaskService
	decisionSteps WorkflowStepReader
	eventBus      bus.EventBus

	// undoTasks is the task-service seam undo and the activity list read
	// through; nil until SetUndoDeps.
	undoTasks UndoTaskService
	undoLocks keyedLock

	retentionWG      sync.WaitGroup
	retentionRunning atomic.Bool

	// sweepMu guards sweepStarted against concurrent StartApprovalSweep
	// calls; sweepWG lets Stop (and tests) wait for the loop to drain. See
	// docs/specs/coordinator/system-design/proposal-recovery.md#recovery.
	sweepMu      sync.Mutex
	sweepStarted bool
	sweepWG      sync.WaitGroup

	// launchWG tracks resume launches that may outlive their Execute deadline.
	launchWG sync.WaitGroup

	// afterSweepPass is a test-only hook invoked once at the end of every
	// approval-sweep pass (including a pass with nothing to recover). nil in
	// production; only tests in this package set it, to join on a pass
	// completing instead of sleeping.
	afterSweepPass func()

	// phase2 is true when the control surface is on. It gates every phase-2
	// behavior of the service; false leaves the phase-1 product unchanged.
	phase2 bool
	// policyErrLogged holds one entry per "coordinatorID:policy_revision" whose
	// unreadable stored policy has been logged.
	policyErrLogged sync.Map

	// phase3 is true when phase 3 is effective (features.coordinator, phase 2
	// and phase 3 all on). It gates the autonomy settings.
	phase3 bool
	// kick asks the wake ticker to re-evaluate one coordinator; nil means no call.
	kick func(ctx context.Context, coordinatorID string) error

	// afterApproveRecheck is a test-only hook run between the approve policy
	// re-check and the claim.
	afterApproveRecheck func()
}

// ServiceOption configures optional Service behavior.
type ServiceOption func(*Service)

// WithPhase2 turns the phase-2 control surface on or off. Off is the default.
func WithPhase2(on bool) ServiceOption {
	return func(s *Service) { s.phase2 = on }
}

// WithPhase3 turns the phase 3 autonomy surface on or off. Off is the default;
// callers pass the effective condition, never the raw flag.
func WithPhase3(on bool) ServiceOption {
	return func(s *Service) { s.phase3 = on }
}

// Phase3Enabled reports whether the phase 3 autonomy surface is effective.
func (s *Service) Phase3Enabled() bool { return s.phase3 }

// SetKick registers the post-commit Kick the autonomy PATCH calls; nil clears it.
func (s *Service) SetKick(kick func(ctx context.Context, coordinatorID string) error) {
	s.kick = kick
}

// Phase2Enabled reports whether the phase-2 control surface is on.
func (s *Service) Phase2Enabled() bool { return s.phase2 }

// NewService builds a Service over store, validator, the workspace
// authorizer and a logger.
func NewService(store *Store, validator *Validator, authorizer WorkspaceAuthorizer, log *logger.Logger, opts ...ServiceOption) *Service {
	s := &Service{
		store:     store,
		validator: validator,
		authz:     authorizer,
		logger:    log.WithFields(zap.String("component", "coordinator-service")),
	}
	s.registerKinds()
	s.executeTimeout = executeDeadline
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetConversationHooks registers the conversation-lifecycle hooks a later
// work package uses to archive or clean up conversation tasks. Both are
// no-ops (nil) by default, which is WP-1's contract.
func (s *Service) SetConversationHooks(cleared ConversationClearedHook, deleted CoordinatorDeletedHook) {
	s.onConversationCleared = cleared
	s.onCoordinatorDeleted = deleted
}

// SetDecisionDeps wires the task service and step-graph reader Approve and
// Reject need, and the event bus coordinator.updated publishes on
// (docs/plans/workspace-coordinator/task-07-proposals-backend.md). Called
// once by the decisions registration function in backendapp/coordinator.go;
// nil until then, matching SetConversationHooks's contract.
func (s *Service) SetDecisionDeps(tasks DecisionTaskService, steps WorkflowStepReader, eventBus bus.EventBus) {
	s.decisionTasks = tasks
	s.decisionSteps = steps
	s.eventBus = eventBus
}

// SetUndoDeps wires the task-service seam behind undo and the list's task
// identifiers.
func (s *Service) SetUndoDeps(tasks UndoTaskService) { s.undoTasks = tasks }

// publishCoordinatorUpdated recomputes coordinatorID's open-proposal count
// and publishes events.CoordinatorUpdated (proposals.md#events). A nil
// eventBus (SetDecisionDeps not called, e.g. in a store-only test) makes
// this a no-op; a count read failure is logged at warn and swallowed, since
// a stale badge count is not worth failing the caller's write over.
func (s *Service) publishCoordinatorUpdated(ctx context.Context, workspaceID, coordinatorID string) {
	s.publishCoordinatorUpdatedWith(ctx, workspaceID, coordinatorID, false)
}

// publishCoordinatorUpdatedWith is publishCoordinatorUpdated with the
// autonomy_changed field set as given.
func (s *Service) publishCoordinatorUpdatedWith(ctx context.Context, workspaceID, coordinatorID string, autonomyChanged bool) {
	if s.eventBus == nil {
		return
	}
	open, err := s.store.CountOpenProposals(ctx, coordinatorID, s.phase2)
	if err != nil {
		s.logger.Warn("failed to count open proposals for coordinator.updated",
			zap.String("coordinator_id", coordinatorID), zap.Error(err))
		return
	}
	payload := NewCoordinatorUpdatedPayload(workspaceID, coordinatorID, open)
	payload.AutonomyChanged = autonomyChanged
	event := bus.NewEvent(events.CoordinatorUpdated, "coordinator-service", payload)
	if err := s.eventBus.Publish(ctx, events.CoordinatorUpdated, event); err != nil {
		s.logger.Warn("failed to publish coordinator.updated",
			zap.String("coordinator_id", coordinatorID), zap.Error(err))
	}
}

// CreateCoordinator validates and inserts a new coordinator
// (coordinators.md#routes, Build decisions 5 and 6).
func (s *Service) CreateCoordinator(ctx context.Context, workspaceID string, req CreateCoordinatorRequest) (*Coordinator, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	name, err := ValidateName(req.Name)
	if err != nil {
		return nil, err
	}
	coordinatorContext, err := ValidateContext(req.Context)
	if err != nil {
		return nil, err
	}
	if err := s.validator.ValidateAgentProfile(ctx, workspaceID, req.AgentProfileID); err != nil {
		return nil, err
	}
	if err := s.validator.ValidateExecutorProfile(ctx, req.ExecutorProfileID); err != nil {
		return nil, err
	}

	created := &Coordinator{
		WorkspaceID:       workspaceID,
		Name:              name,
		AgentProfileID:    req.AgentProfileID,
		ExecutorProfileID: req.ExecutorProfileID,
		Context:           coordinatorContext,
	}
	if err := s.store.CreateCoordinator(ctx, created); err != nil {
		return nil, fmt.Errorf("create coordinator: %w", err)
	}
	s.logger.Info("coordinator created",
		zap.String("workspace_id", workspaceID), zap.String("coordinator_id", created.ID))
	return created, nil
}

// GetCoordinator returns a coordinator and its two profile statuses
// (coordinators.md#validation).
func (s *Service) GetCoordinator(ctx context.Context, workspaceID, id string) (*Coordinator, ProfileStatus, ProfileStatus, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, "", "", err
	}
	found, err := s.store.GetCoordinator(ctx, workspaceID, id)
	if err != nil {
		return nil, "", "", err
	}
	agentStatus, executorStatus, err := s.validator.ProfileStatus(ctx, workspaceID, found.AgentProfileID, found.ExecutorProfileID)
	if err != nil {
		return nil, "", "", fmt.Errorf("compute profile status: %w", err)
	}
	return found, agentStatus, executorStatus, nil
}

// ListCoordinators returns every coordinator of a workspace, each paired with
// its open proposal count (Build decision 9). Never nil. Fetches every
// coordinator's count with one grouped query (CountOpenProposalsByWorkspace)
// rather than one query per coordinator.
func (s *Service) ListCoordinators(ctx context.Context, workspaceID string) ([]CoordinatorWithOpenProposals, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	found, err := s.store.ListCoordinators(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	counts, err := s.store.CountOpenProposalsByWorkspace(ctx, workspaceID, s.phase2)
	if err != nil {
		return nil, err
	}
	result := make([]CoordinatorWithOpenProposals, len(found))
	for i, c := range found {
		result[i] = CoordinatorWithOpenProposals{Coordinator: c, OpenProposals: counts[c.ID]}
	}
	return result, nil
}

// PatchCoordinator applies a partial update, validating any changed field
// (coordinators.md#routes, Build decision 7). If the change clears
// conversation_task_id, the registered ConversationClearedHook (if any) is
// called with the old task id after commit.
func (s *Service) PatchCoordinator(ctx context.Context, workspaceID, id string, req PatchCoordinatorRequest) (*Coordinator, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	patch, err := s.buildCoordinatorPatch(req)
	if err != nil {
		return nil, err
	}

	validate := func(ctx context.Context, merged *Coordinator) error {
		if err := s.validator.ValidateAgentProfile(ctx, workspaceID, merged.AgentProfileID); err != nil {
			return err
		}
		if err := s.validator.ValidateExecutorProfile(ctx, merged.ExecutorProfileID); err != nil {
			return err
		}
		return checkAutonomyInterlock(patch, merged)
	}
	result, err := s.store.PatchCoordinatorResult(ctx, workspaceID, id, patch, validate)
	if err != nil {
		return nil, err
	}
	updated, clearedConversationTaskID := result.Coordinator, result.ClearedConversationTaskID
	if result.AutonomyChanged {
		s.afterAutonomyChange(ctx, workspaceID, id)
	}
	if clearedConversationTaskID != nil && s.onConversationCleared != nil {
		s.onConversationCleared(ctx, id, *clearedConversationTaskID)
	}
	s.logger.Info("coordinator updated",
		zap.String("workspace_id", workspaceID), zap.String("coordinator_id", id),
		zap.Bool("context_changed", clearedConversationTaskID != nil))
	return updated, nil
}

// afterAutonomyChange runs after a PATCH that changed autonomy or the ceiling
// committed: it publishes coordinator.updated with autonomy_changed and calls
// Kick. Both are best effort; a failure or a Kick panic never changes the
// PATCH result.
func (s *Service) afterAutonomyChange(ctx context.Context, workspaceID, id string) {
	s.publishCoordinatorUpdatedWith(ctx, workspaceID, id, true)
	if s.kick == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("coordinator kick panicked", zap.String("coordinator_id", id), zap.Any("panic", r))
		}
	}()
	if err := s.kick(ctx, id); err != nil {
		s.logger.Warn("coordinator kick failed", zap.String("coordinator_id", id), zap.Error(err))
	}
}

// buildCoordinatorPatch parses req's four known fields into a
// CoordinatorPatch, trimming and length-validating name and context (Build
// decisions 5 and 7). A field absent from req is left nil (unchanged); a
// field sent as JSON null or the wrong type surfaces req.StringField's
// *FieldError.
func (s *Service) buildCoordinatorPatch(req PatchCoordinatorRequest) (CoordinatorPatch, error) {
	var patch CoordinatorPatch

	name, present, err := req.StringField(PatchFieldName)
	if err != nil {
		return patch, err
	}
	if present {
		trimmed, err := ValidateName(*name)
		if err != nil {
			return patch, err
		}
		patch.Name = &trimmed
	}

	coordinatorContext, present, err := req.StringField(PatchFieldContext)
	if err != nil {
		return patch, err
	}
	if present {
		trimmed, err := ValidateContext(*coordinatorContext)
		if err != nil {
			return patch, err
		}
		patch.Context = &trimmed
	}

	agentProfileID, present, err := req.StringField(PatchFieldAgentProfileID)
	if err != nil {
		return patch, err
	}
	if present {
		patch.AgentProfileID = agentProfileID
	}

	executorProfileID, present, err := req.StringField(PatchFieldExecutorProfileID)
	if err != nil {
		return patch, err
	}
	if present {
		patch.ExecutorProfileID = executorProfileID
	}

	if s.phase3 {
		if err := applyAutonomyFields(req, &patch); err != nil {
			return patch, err
		}
	}
	return patch, nil
}

// DeleteCoordinator deletes a coordinator and its proposals (Build decision
// 8). The registered CoordinatorDeletedHook (if any) is called after commit.
func (s *Service) DeleteCoordinator(ctx context.Context, workspaceID, id string) error {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return err
	}
	if err := s.store.DeleteCoordinator(ctx, workspaceID, id); err != nil {
		return err
	}
	if s.onCoordinatorDeleted != nil {
		s.onCoordinatorDeleted(ctx, workspaceID, id)
	}
	s.logger.Info("coordinator deleted",
		zap.String("workspace_id", workspaceID), zap.String("coordinator_id", id))
	return nil
}

// GetProposal returns one proposal scoped to workspaceID and coordinatorID
// (proposals.md#routes: a proposal of another coordinator or workspace is
// ErrNotFound).
func (s *Service) GetProposal(ctx context.Context, workspaceID, coordinatorID, id string) (*Proposal, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.store.GetProposal(ctx, workspaceID, coordinatorID, id, s.phase2)
}

// ListProposals returns a coordinator's proposals per status (Build decision
// 3). Never nil.
func (s *Service) ListProposals(ctx context.Context, workspaceID, coordinatorID string, status ListProposalsStatus) ([]*Proposal, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.store.ListProposals(ctx, workspaceID, coordinatorID, status, s.phase2)
}

// CoordinatorForConversationTask returns the id of the coordinator whose
// current conversation_task_id equals taskID, for the mcp/scope resolver's
// principalSurface and the executor's fail-closed session-start checks
// (docs/specs/coordinator/system-design/copilot.md#principal-and-mode). It
// carries no workspace scope of its own: the caller is server-side task/mode
// resolution, not a user-scoped request.
func (s *Service) CoordinatorForConversationTask(ctx context.Context, taskID string) (string, bool, error) {
	return s.store.CoordinatorForConversationTask(ctx, taskID)
}

// CoordinatorProfilesReady reports whether coordinatorID's agent and executor
// profiles are both usable (agent present and not passthrough, executor
// present), for the executor's fail-closed session-start check
// (docs/specs/coordinator/system-design/copilot.md#fail-closed).
func (s *Service) CoordinatorProfilesReady(ctx context.Context, coordinatorID string) (bool, error) {
	found, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return false, err
	}
	agentStatus, executorStatus, err := s.validator.ProfileStatus(ctx, found.WorkspaceID, found.AgentProfileID, found.ExecutorProfileID)
	if err != nil {
		return false, fmt.Errorf("compute profile status: %w", err)
	}
	return agentStatus == ProfileStatusOK && executorStatus == ProfileStatusOK, nil
}

// CoordinatorStandingInstructionsData returns coordinatorID's name and
// standing context for the Standing Instructions system-prompt block
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions). It
// carries no workspace scope of its own: the caller is server-side prompt
// construction for an already-permitted session, not a user-scoped request.
func (s *Service) CoordinatorStandingInstructionsData(ctx context.Context, coordinatorID string) (string, string, error) {
	found, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return "", "", err
	}
	return found.Name, found.Context, nil
}

// ListStalls returns a workspace's stall records
// (needs-you.md#stall-records).
func (s *Service) ListStalls(ctx context.Context, workspaceID string) ([]*Stall, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.store.ListStalls(ctx, workspaceID)
}

// GetStall returns the one stall record for taskID in workspaceID
// (docs/specs/coordinator/system-design/copilot-tools.md#item-read).
// ErrNotFound if the task never stalled or its row was cleared.
func (s *Service) GetStall(ctx context.Context, workspaceID, taskID string) (*Stall, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.store.GetStall(ctx, workspaceID, taskID)
}

// PruneStalls deletes stall records for a missing or archived task, and
// records older than 30 days (needs-you.md#stall-records). Unauthorized: it
// is only ever called from the coordinator startup pass, never from a
// workspace-scoped request.
func (s *Service) PruneStalls(ctx context.Context, now time.Time) (int64, error) {
	return s.store.PruneStalls(ctx, now)
}

// PruneWakeState is the phase 3 retention pass, returning the turn and wake
// rows deleted. Unauthorized: only the coordinator startup pass calls it.
func (s *Service) PruneWakeState(ctx context.Context, now time.Time) (turns, wakes int64, err error) {
	return s.store.PruneWakeState(ctx, now)
}

// DeleteWorkspaceState deletes a workspace's coordinators, proposals and
// stall records (coordinators.md#workspace-deletion). Unauthorized: callers
// are the workspace.deleted subscriber and the E2E reset endpoint, neither of
// which carries a workspace-scoped request to authorize.
func (s *Service) DeleteWorkspaceState(ctx context.Context, workspaceID string) error {
	return s.store.DeleteWorkspaceState(ctx, workspaceID)
}
