package service

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	completionGateActorSystem = "system"
	completionGateActorHuman  = "human"
)

// TaskCompletionHumanConfirmation authorizes one revision-bound weakening or
// removal of currently unmet criteria from the native task UI.
type TaskCompletionHumanConfirmation struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

// SetTaskCompletionCriteriaRequest replaces the criterion set under an
// observed revision. Plugins cannot provide HumanConfirmation because exact
// Host adapters use the plugin actor identity and reject that field.
type SetTaskCompletionCriteriaRequest struct {
	ExpectedRevision  int64                            `json:"expected_revision"`
	Criteria          []models.TaskCompletionCriterion `json:"criteria"`
	HumanConfirmation *TaskCompletionHumanConfirmation `json:"human_confirmation,omitempty"`
}

// VerifyTaskCompletionCriterionRequest submits typed evidence for one current
// criterion under its observed set revision.
type VerifyTaskCompletionCriterionRequest struct {
	ExpectedRevision int64                         `json:"expected_revision"`
	CriterionID      string                        `json:"criterion_id"`
	Evidence         models.TaskCompletionEvidence `json:"evidence"`
}

// TaskCompletionMoveOverrideRequest confirms one intended move in the native
// task UI. The authenticated identity, source, target, and set revision are
// bound by MoveTaskWithOptions before repository admission.
type TaskCompletionMoveOverrideRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

// ExactTaskCompletionCriteriaRequest is the task-service command surface used
// only by the authorized plugin Host adapter.
type ExactTaskCompletionCriteriaRequest struct {
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  repository.TaskManagementClaimFence
	ActorID                     string
	Criteria                    []models.TaskCompletionCriterion
}

// ExactTaskCompletionEvidenceRequest is the task-service command surface used
// only by the authorized plugin Host adapter.
type ExactTaskCompletionEvidenceRequest struct {
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  repository.TaskManagementClaimFence
	ActorID                     string
	CriterionID                 string
	Evidence                    models.TaskCompletionEvidence
}

// ExactTaskCompletionGateResult includes the domain receipt replay marker.
type ExactTaskCompletionGateResult struct {
	Snapshot       *models.TaskCompletionGateSnapshot
	AlreadyApplied bool
}

// GetTaskCompletionGate returns the canonical gate and current blocker list.
func (s *Service) GetTaskCompletionGate(ctx context.Context, taskID string) (*models.TaskCompletionGateSnapshot, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	return repo.GetTaskCompletionGate(ctx, taskID)
}

// SetTaskCompletionCriteria stores an exact, versioned criterion set. A human
// confirmation is checked against the same set revision and actor used by the
// repository transaction.
func (s *Service) SetTaskCompletionCriteria(ctx context.Context, taskID string, request SetTaskCompletionCriteriaRequest) (*models.TaskCompletionGateSnapshot, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	actorKind, actorID, human := taskCompletionActor(ctx)
	change := models.TaskCompletionCriteriaChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: request.ExpectedRevision,
		Criteria: sanitizeCompletionCriteria(request.Criteria), ActorKind: actorKind, ActorID: actorID,
	}
	if request.HumanConfirmation != nil {
		if !human || strings.TrimSpace(request.HumanConfirmation.Reason) == "" {
			return nil, repoerrors.ErrTaskCompletionHumanConfirmationRequired
		}
		change.HumanConfirmationRevision = request.HumanConfirmation.ExpectedRevision
		change.HumanConfirmationReason = strings.TrimSpace(request.HumanConfirmation.Reason)
	}
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, change)
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, task.ID)
	return snapshot, nil
}

// SetTaskCompletionCriteriaExact applies an approved plugin command with
// resource-version and management-generation fencing in the repository.
func (s *Service) SetTaskCompletionCriteriaExact(ctx context.Context, taskID string, request ExactTaskCompletionCriteriaRequest) (*ExactTaskCompletionGateResult, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.ExactTaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	snapshot, alreadyApplied, err := repo.SetTaskCompletionCriteriaExact(ctx, models.TaskCompletionCriteriaChange{
		TaskID: taskID, WorkspaceID: request.WorkspaceID,
		ExpectedTaskResourceVersion: request.ExpectedTaskResourceVersion,
		ExpectedRevision:            request.ExpectedRevision, OperationID: request.OperationID,
		PayloadDigest: request.PayloadDigest, ClaimFence: request.ClaimFence,
		Criteria: sanitizeCompletionCriteria(request.Criteria), ActorKind: "plugin", ActorID: request.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, taskID)
	return &ExactTaskCompletionGateResult{Snapshot: snapshot, AlreadyApplied: alreadyApplied}, nil
}

func sanitizeCompletionCriteria(criteria []models.TaskCompletionCriterion) []models.TaskCompletionCriterion {
	clean := make([]models.TaskCompletionCriterion, len(criteria))
	for index, criterion := range criteria {
		criterion.CriterionRevision = 0
		criterion.VerifiedRevision = 0
		criterion.Evidence = nil
		criterion.VerifierKind = ""
		criterion.VerifierID = ""
		criterion.VerifiedAt = nil
		criterion.EvidenceSubject.Revision = ""
		clean[index] = criterion
	}
	return clean
}

// VerifyTaskCompletionCriterion records verifier identity and checks the
// subject revision before accepting evidence.
func (s *Service) VerifyTaskCompletionCriterion(ctx context.Context, taskID string, request VerifyTaskCompletionCriterionRequest) (*models.TaskCompletionGateSnapshot, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	actorKind, actorID, _ := taskCompletionActor(ctx)
	snapshot, err := repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: request.ExpectedRevision,
		CriterionID: request.CriterionID, Evidence: request.Evidence, ActorKind: actorKind, ActorID: actorID,
	})
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, task.ID)
	return snapshot, nil
}

// VerifyTaskCompletionCriterionExact records a plugin verifier through the
// same typed evidence check and task-claim fence as native verification.
func (s *Service) VerifyTaskCompletionCriterionExact(ctx context.Context, taskID string, request ExactTaskCompletionEvidenceRequest) (*ExactTaskCompletionGateResult, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.ExactTaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	snapshot, alreadyApplied, err := repo.VerifyTaskCompletionCriterionExact(ctx, models.TaskCompletionEvidenceChange{
		TaskID: taskID, WorkspaceID: request.WorkspaceID,
		ExpectedTaskResourceVersion: request.ExpectedTaskResourceVersion,
		ExpectedRevision:            request.ExpectedRevision, OperationID: request.OperationID,
		PayloadDigest: request.PayloadDigest, ClaimFence: request.ClaimFence,
		CriterionID: request.CriterionID, Evidence: request.Evidence,
		ActorKind: "plugin", ActorID: request.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, taskID)
	return &ExactTaskCompletionGateResult{Snapshot: snapshot, AlreadyApplied: alreadyApplied}, nil
}

// ListTaskCompletionGateHistory returns the append-only native audit trail.
func (s *Service) ListTaskCompletionGateHistory(ctx context.Context, taskID string) ([]*models.TaskCompletionGateHistory, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	return repo.ListTaskCompletionGateHistory(ctx, taskID)
}

func taskCompletionActor(ctx context.Context) (kind, id string, human bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		return completionGateActorSystem, completionGateActorSystem, false
	}
	return completionGateActorHuman, identity.UserID, true
}

func (s *Service) publishCompletionGateTaskUpdate(ctx context.Context, taskID string) {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
}
