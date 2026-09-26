package cursorcloud

import (
	"context"
	"errors"
	"fmt"
	"strings"

	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Runtime) cancelOperation(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) error {
	if models.ManagedAgentOperationTerminal(operation.State) || operation.State == models.ManagedAgentSubmissionRejected {
		return nil
	}
	if operation.RemoteRunID == "" {
		recordCancel("pending")
		return provider.ErrOutcomeUnknown
	}
	binding, operation, err := r.ensureCancellationIntent(ctx, binding, operation)
	if err != nil {
		return err
	}
	client, err := r.clientFactory(ctx, binding)
	if err != nil {
		recordCancel("pending")
		return fmt.Errorf("load Cursor Cloud cancellation client: %w", err)
	}
	cancelErr := client.CancelRun(ctx, binding.RemoteAgentID, operation.RemoteRunID)
	run, readErr := client.GetRun(ctx, binding.RemoteAgentID, operation.RemoteRunID)
	return r.finishCancellation(ctx, binding, operation, cancelErr, run, readErr)
}

func (r *Runtime) ensureCancellationIntent(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, error) {
	if operation.State == models.ManagedAgentSubmissionCancelling {
		return binding, operation, nil
	}
	leaseBinding, owner, err := r.ensureCancellationLease(ctx, binding, operation)
	if err != nil {
		return nil, nil, err
	}
	updated, err := r.updateOperation(ctx, leaseBinding, operation, owner, models.ManagedAgentSubmissionCancelling, "", "")
	if err != nil {
		return nil, nil, fmt.Errorf("record Cursor Cloud cancellation intent: %w", err)
	}
	return leaseBinding, updated, nil
}

func (r *Runtime) finishCancellation(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	cancelErr error,
	run provider.Run,
	readErr error,
) error {
	if readErr != nil {
		return r.cancellationReadError(cancelErr, readErr)
	}
	state, terminal := terminalSubmissionState(run.Status)
	if !terminal {
		return r.cancellationStatusError(cancelErr)
	}
	if operation.State != state {
		updated, err := r.settleCancelledRun(ctx, binding, operation, run, state)
		if err != nil {
			recordCancel("pending")
			return err
		}
		operation = updated
	}
	r.publishPendingCompletion(ctx, binding, operation)
	recordCancel("confirmed")
	return nil
}

func (r *Runtime) cancellationReadError(cancelErr, readErr error) error {
	recordCancel("pending")
	return errors.Join(ErrCancellationPending, cancelErr, readErr)
}

func (r *Runtime) cancellationStatusError(cancelErr error) error {
	if cancelErr != nil {
		recordCancel("rejected")
		return errors.Join(ErrCancellationPending, cancelErr)
	}
	recordCancel("pending")
	return ErrCancellationPending
}

func (r *Runtime) settleCancelledRun(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	run provider.Run,
	state models.ManagedAgentSubmissionState,
) (*models.ManagedAgentOperation, error) {
	result := resultSnapshotForRun(binding, run)
	if err := r.persistRunResult(ctx, binding, operation, run); err != nil {
		return nil, err
	}
	leaseBinding, owner, err := r.ensureCancellationLease(ctx, binding, operation)
	if err != nil {
		return nil, err
	}
	pending := true
	updated, err := r.repository.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: leaseBinding.Revision, LeaseOwner: owner,
		State: state, ResultSnapshot: &result, CompletionPending: &pending,
	})
	if err != nil {
		return nil, fmt.Errorf("settle Cursor Cloud cancellation: %w", err)
	}
	return updated, nil
}

func terminalSubmissionState(status string) (models.ManagedAgentSubmissionState, bool) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FINISHED":
		return models.ManagedAgentSubmissionSucceeded, true
	case runStatusError, runStatusExpired:
		return models.ManagedAgentSubmissionFailed, true
	case runStatusCancelled:
		return models.ManagedAgentSubmissionCancelled, true
	default:
		return "", false
	}
}
