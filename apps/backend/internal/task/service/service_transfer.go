package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type taskTransferRepository interface {
	TransferTask(context.Context, models.TaskTransferCommand) (*models.TaskTransferReceipt, error)
}

type taskTransferAttemptRepository interface {
	RecordTaskTransferAttempt(context.Context, models.TaskTransferCommand, string) error
}

type taskTransferReplayRepository interface {
	ReplayTaskTransfer(context.Context, models.TaskTransferCommand) (*models.TaskTransferReceipt, bool, error)
	ResolveTaskTransferReplayActor(context.Context, models.TaskTransferCommand) (models.TaskTransferActor, bool, error)
}

// TransferTask preserves one task identity while atomically rebinding its
// workspace, workflow, and semantically equivalent lane.
func (s *Service) TransferTask(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	repository, ok := s.tasks.(taskTransferRepository)
	if !ok {
		err := fmt.Errorf("task transfer repository is unavailable")
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, "failed"))
	}
	if err := s.authorizeTaskID(ctx, command.TaskID); err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, "denied"))
	}
	if err := s.authorizeWorkspaceID(ctx, command.DestinationWorkspaceID); err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, "denied"))
	}
	if replayRepository, ok := s.tasks.(taskTransferReplayRepository); ok {
		receipt, found, err := replayRepository.ReplayTaskTransfer(ctx, command)
		if err != nil {
			return nil, err
		}
		if found {
			return receipt, nil
		}
	}
	if err := s.authorizeWorkflowID(ctx, command.DestinationWorkflowID); err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, "denied"))
	}
	authorizedOwnerID, err := s.validateTaskTransferBoundary(ctx, command)
	if err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, "denied"))
	}
	command.AuthorizedOwnerID = authorizedOwnerID
	command.OwnerPredicateSet = true
	receipt, err := repository.TransferTask(ctx, command)
	if err != nil {
		return nil, err
	}
	if !receipt.IdempotentReplay {
		s.publishTaskTransferUpdate(ctx, receipt)
		s.pullNextTaskOnVacate(ctx, receipt.SourceStepID, receipt.TaskID)
	}
	return receipt, nil
}

// AuditTaskTransferAttempt records a redacted denied attempt without invoking
// the placement mutation path.
func (s *Service) AuditTaskTransferAttempt(
	ctx context.Context,
	command models.TaskTransferCommand,
	result string,
) error {
	repository, ok := s.tasks.(taskTransferAttemptRepository)
	if !ok {
		return fmt.Errorf("task transfer audit repository is unavailable")
	}
	return repository.RecordTaskTransferAttempt(ctx, command, result)
}

// ResolveTaskTransferReplayActor authorizes only an exact committed replay;
// it cannot create a new operation.
func (s *Service) ResolveTaskTransferReplayActor(
	ctx context.Context,
	command models.TaskTransferCommand,
) (models.TaskTransferActor, bool, error) {
	if err := s.authorizeTaskID(ctx, command.TaskID); err != nil {
		return models.TaskTransferActor{}, false, err
	}
	if err := s.authorizeWorkspaceID(ctx, command.DestinationWorkspaceID); err != nil {
		return models.TaskTransferActor{}, false, err
	}
	repository, ok := s.tasks.(taskTransferReplayRepository)
	if !ok {
		return models.TaskTransferActor{}, false, fmt.Errorf("task transfer replay repository is unavailable")
	}
	return repository.ResolveTaskTransferReplayActor(ctx, command)
}

func (s *Service) validateTaskTransferBoundary(
	ctx context.Context,
	command models.TaskTransferCommand,
) (string, error) {
	source, err := s.workspaces.GetWorkspace(ctx, command.ExpectedSourceWorkspaceID)
	if err != nil {
		return "", err
	}
	destination, err := s.workspaces.GetWorkspace(ctx, command.DestinationWorkspaceID)
	if err != nil {
		return "", err
	}
	if source.OwnerID != destination.OwnerID {
		return "", repoerrors.ErrWorkspaceNotFound
	}
	workflow, err := s.workflows.GetWorkflow(ctx, command.DestinationWorkflowID)
	if err != nil {
		return "", err
	}
	if workflow.WorkspaceID != destination.ID {
		return "", repoerrors.ErrWorkspaceNotFound
	}
	return source.OwnerID, nil
}

func (s *Service) publishTaskTransferUpdate(ctx context.Context, receipt *models.TaskTransferReceipt) {
	task, err := s.tasks.GetTask(ctx, receipt.TaskID)
	if err != nil {
		s.logger.Error("load transferred task for event")
		return
	}
	extra := map[string]interface{}{
		"transfer_operation_id": receipt.OperationID,
		"source_workspace_id":   receipt.SourceWorkspaceID,
		"source_workflow_id":    receipt.SourceWorkflowID,
		"source_step_id":        receipt.SourceStepID,
		"preservation_digest":   receipt.PreservationDigest,
	}
	s.publishTaskEventWithExtra(ctx, events.TaskUpdated, task, nil, extra, receipt.SourceWorkflowID)
}
