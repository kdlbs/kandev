package coordinator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

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
// deleted (Build decision 8), before its conversation tasks are cleaned up by
// a later work package.
type CoordinatorDeletedHook func(ctx context.Context, coordinatorID string)

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
	store     *Store
	validator *Validator
	authz     WorkspaceAuthorizer
	logger    *logger.Logger

	onConversationCleared ConversationClearedHook
	onCoordinatorDeleted  CoordinatorDeletedHook

	proposalWorkflows    WorkflowReader
	proposalRepositories RepositoryReader
	proposalTasks        SourceTaskReader
	proposalSteps        WorkflowStepReader
}

// NewService builds a Service over store, validator, the workspace
// authorizer and a logger.
func NewService(store *Store, validator *Validator, authorizer WorkspaceAuthorizer, log *logger.Logger) *Service {
	return &Service{
		store:     store,
		validator: validator,
		authz:     authorizer,
		logger:    log.WithFields(zap.String("component", "coordinator-service")),
	}
}

// SetConversationHooks registers the conversation-lifecycle hooks a later
// work package uses to archive or clean up conversation tasks. Both are
// no-ops (nil) by default, which is WP-1's contract.
func (s *Service) SetConversationHooks(cleared ConversationClearedHook, deleted CoordinatorDeletedHook) {
	s.onConversationCleared = cleared
	s.onCoordinatorDeleted = deleted
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
// its open proposal count (Build decision 9). Never nil.
func (s *Service) ListCoordinators(ctx context.Context, workspaceID string) ([]CoordinatorWithOpenProposals, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	found, err := s.store.ListCoordinators(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]CoordinatorWithOpenProposals, len(found))
	for i, c := range found {
		count, err := s.store.CountOpenProposals(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		result[i] = CoordinatorWithOpenProposals{Coordinator: c, OpenProposals: count}
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
		return s.validator.ValidateExecutorProfile(ctx, merged.ExecutorProfileID)
	}
	updated, clearedConversationTaskID, err := s.store.PatchCoordinator(ctx, workspaceID, id, patch, validate)
	if err != nil {
		return nil, err
	}
	if clearedConversationTaskID != nil && s.onConversationCleared != nil {
		s.onConversationCleared(ctx, id, *clearedConversationTaskID)
	}
	s.logger.Info("coordinator updated",
		zap.String("workspace_id", workspaceID), zap.String("coordinator_id", id),
		zap.Bool("context_changed", clearedConversationTaskID != nil))
	return updated, nil
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
		s.onCoordinatorDeleted(ctx, id)
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
	return s.store.GetProposal(ctx, workspaceID, coordinatorID, id)
}

// ListProposals returns a coordinator's proposals per status (Build decision
// 3). Never nil.
func (s *Service) ListProposals(ctx context.Context, workspaceID, coordinatorID string, status ListProposalsStatus) ([]*Proposal, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.store.ListProposals(ctx, workspaceID, coordinatorID, status)
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

// ListStalls returns a workspace's stall records
// (needs-you.md#stall-records).
func (s *Service) ListStalls(ctx context.Context, workspaceID string) ([]*Stall, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.store.ListStalls(ctx, workspaceID)
}
