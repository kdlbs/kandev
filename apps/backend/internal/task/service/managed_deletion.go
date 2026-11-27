package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ManagedDeletionError keeps admitted failure distinct from effect-free rejection.
// Outcome is also consumed by the Host without importing this service package.
type ManagedDeletionError struct {
	Outcome string
	Cause   error
}

func (e *ManagedDeletionError) Error() string {
	return fmt.Sprintf("managed deletion %s: %v", e.Outcome, e.Cause)
}
func (e *ManagedDeletionError) Unwrap() error                  { return e.Cause }
func (e *ManagedDeletionError) ManagedDeletionOutcome() string { return e.Outcome }
func (e *ManagedDeletionError) GRPCStatus() *status.Status {
	code := codes.Unavailable
	if e.Outcome == "committed" {
		code = codes.NotFound
	}
	return status.New(code, e.Error())
}

func (s *Service) DeleteManagedConversationTask(ctx context.Context, request managed.DeleteRequest) error {
	return s.deleteManagedConversationTaskWithOptions(ctx, request, "", DeleteTaskOptions{})
}

func (s *Service) deleteManagedConversationTaskWithOptions(ctx context.Context, request managed.DeleteRequest, reason string, options DeleteTaskOptions) error {
	native, ok := s.tasks.(managed.DeletionRepository)
	if !ok || s.resourceCleanups == nil {
		return managedAdmissionError(managed.ErrUnavailable)
	}
	deadline := archivecascade.ArchiveDeadline(ctx)
	operationCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	previous, job, err := native.InspectManagedDeletion(operationCtx, request)
	if err != nil {
		return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: err}
	}
	if previous != nil {
		if previous.Phase == managed.DeleteCommitted {
			if err := s.StartPreparedTaskResourceCleanup(operationCtx, job.OperationID); err != nil {
				return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: err}
			}
			return &ManagedDeletionError{Outcome: "committed", Cause: managed.ErrNotFound}
		}
		if job.State != models.TaskResourceCleanupStateCancelled {
			return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: managed.ErrDeletionOwned}
		}
	}
	if err := s.authorizeTaskScope(operationCtx, request.TaskID, authz.ScopeTaskWrite); err != nil {
		return managedHistoryError(previous, err)
	}
	owner := uuid.NewString()
	claim, err := native.AdmitManagedDeletion(operationCtx, request, owner)
	if err != nil {
		if claim != nil {
			if claim.Owner != owner {
				return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: err}
			}
			return s.reconcileManagedDeletionFailure(operationCtx, native, *claim, err, false)
		}
		return managedHistoryError(previous, managedAdmissionError(err))
	}
	_, err = s.deleteTaskWithManagedClaim(operationCtx, request.TaskID, reason, models.TaskResourceCleanupTriggerDelete, options,
		func(ctx context.Context, _ string) (bool, error) {
			_, err := native.FinalizeManagedDeletion(ctx, *claim)
			return err == nil, err
		}, claim)
	if err != nil {
		return s.reconcileManagedDeletionFailure(operationCtx, native, *claim, err, true)
	}
	return nil
}

func managedHistoryError(previous *managed.DeleteClaim, err error) error {
	if previous != nil {
		return &ManagedDeletionError{Outcome: "admitted_failed", Cause: err}
	}
	return err
}

func (s *Service) reconcileManagedDeletionFailure(ctx context.Context, native managed.DeletionRepository, claim managed.DeleteClaim, cause error, admitted bool) error {
	transitionCtx, cancel := detachedCleanupTransitionContext(ctx)
	defer cancel()
	current, job, err := native.InspectManagedDeletion(transitionCtx, claim.DeleteRequest)
	if err != nil {
		return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: errors.Join(cause, err)}
	}
	if current == nil {
		if admitted {
			return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: cause}
		}
		return cause
	}
	if current.Owner != claim.Owner {
		return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: errors.Join(cause, managed.ErrDeletionOwned)}
	}
	if current.Phase == managed.DeleteCommitted {
		activateErr := s.StartPreparedTaskResourceCleanup(transitionCtx, job.OperationID)
		return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: errors.Join(cause, activateErr)}
	}
	released, releaseErr := native.ReleaseManagedDeletion(transitionCtx, claim)
	if releaseErr != nil || !released {
		return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: errors.Join(cause, releaseErr)}
	}
	return &ManagedDeletionError{Outcome: "admitted_failed", Cause: cause}
}

func (s *Service) preserveTaskEnvironmentForManagedDeletion(ctx context.Context, taskID string, env *models.TaskEnvironment, claim *managed.DeleteClaim) (bool, error) {
	if claim == nil {
		return s.preserveTaskEnvironmentForActiveBorrower(ctx, taskID, env)
	}
	if env == nil || env.ID == "" || s.sessions == nil {
		return false, nil
	}
	finder, ok := s.sessions.(taskEnvironmentSessionBorrowerFinder)
	if !ok {
		return false, nil
	}
	borrower, err := finder.FindActiveTaskSessionTaskIDByTaskEnvironmentExcludingTask(ctx, env.ID, taskID)
	if err != nil || borrower == "" {
		return false, err
	}
	native, ok := s.taskEnvironments.(managed.DeletionRepository)
	if !ok {
		return false, managed.ErrUnavailable
	}
	if err := native.TransferManagedDeletionEnvironment(ctx, *claim, env.ID, env.TaskID, env.OwnershipGeneration, borrower); err != nil {
		return false, err
	}
	env.TaskID = borrower
	return true, nil
}
