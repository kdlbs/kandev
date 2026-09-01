package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type TaskTransferService interface {
	TransferTask(context.Context, models.TaskTransferCommand) (*models.TaskTransferReceipt, error)
	ResolveTaskTransferReplayActor(context.Context, models.TaskTransferCommand) (models.TaskTransferActor, bool, error)
}

type TaskTransferCoordinatorAuthorizer interface {
	AttestTaskTransferCoordinator(context.Context, mcpscope.Principal) (models.TaskTransferActor, bool)
}

type taskTransferCoordinatorReplayAuthorizer interface {
	AttestTaskTransferCoordinatorReplay(
		context.Context,
		mcpscope.Principal,
		models.TaskTransferCommand,
		models.TaskTransferActor,
	) (models.TaskTransferActor, bool)
}

type taskTransferAttemptAuditor interface {
	AuditTaskTransferAttempt(context.Context, models.TaskTransferCommand, string) error
}

type taskTransferRequest struct {
	AuditOnly                 bool   `json:"_audit_only"`
	TaskID                    string `json:"task_id"`
	ExpectedSourceWorkspaceID string `json:"expected_source_workspace_id"`
	ExpectedSourceWorkflowID  string `json:"expected_source_workflow_id"`
	ExpectedSourceStepID      string `json:"expected_source_workflow_step_id"`
	ExpectedTaskUpdatedAt     string `json:"expected_task_updated_at"`
	DestinationWorkspaceID    string `json:"destination_workspace_id"`
	DestinationWorkflowID     string `json:"destination_workflow_id"`
	DestinationStepID         string `json:"destination_workflow_step_id"`
	DestinationStepName       string `json:"destination_workflow_step_name"`
	IdempotencyKey            string `json:"idempotency_key"`
	PreservationPolicy        string `json:"preservation_policy"`
}

const rejectedTaskTransferAuditTimeout = 2 * time.Second

func (h *Handlers) handleTransferTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var request taskTransferRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		h.auditRejectedTaskTransfer(ctx, request, "failed")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid task transfer payload", nil)
	}
	if request.AuditOnly {
		h.auditRejectedTaskTransfer(ctx, request, "failed")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid task transfer request", nil)
	}
	if h.taskTransferSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task transfer unavailable", nil)
	}
	command, err := h.taskTransferCommand(ctx, request)
	if err != nil {
		result := "denied"
		if errors.Is(err, repoerrors.ErrTaskTransferConflict) {
			result = "conflict"
		}
		h.auditRejectedTaskTransfer(ctx, request, result)
		if errors.Is(err, repoerrors.ErrTaskTransferConflict) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "task transfer conflict", nil)
		}
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task transfer unavailable", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid task transfer request", nil)
	}
	receipt, err := h.taskTransferSvc.TransferTask(ctx, command)
	switch {
	case errors.Is(err, repoerrors.ErrTaskTransferConflict):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "task transfer conflict", nil)
	case errors.Is(err, repoerrors.ErrTaskNotFound), errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task transfer unavailable", nil)
	case err != nil:
		h.logger.Error("task transfer failed", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task transfer failed", nil)
	default:
		return ws.NewResponse(msg.ID, msg.Action, receipt)
	}
}

func (h *Handlers) auditRejectedTaskTransfer(ctx context.Context, request taskTransferRequest, result string) {
	auditor, ok := h.taskTransferSvc.(taskTransferAttemptAuditor)
	if !ok {
		return
	}
	updatedAt, _ := time.Parse(time.RFC3339Nano, request.ExpectedTaskUpdatedAt)
	principal, _ := mcpscope.PrincipalFromContext(ctx)
	command := models.TaskTransferCommand{
		TaskID: request.TaskID, ExpectedSourceWorkspaceID: request.ExpectedSourceWorkspaceID,
		ExpectedSourceWorkflowID: request.ExpectedSourceWorkflowID, ExpectedSourceStepID: request.ExpectedSourceStepID,
		ExpectedTaskUpdatedAt: updatedAt, DestinationWorkspaceID: request.DestinationWorkspaceID,
		DestinationWorkflowID: request.DestinationWorkflowID, DestinationStepID: request.DestinationStepID,
		DestinationStepName: request.DestinationStepName, IdempotencyKey: request.IdempotencyKey,
		PreservationPolicy: request.PreservationPolicy, Actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorRejected, ID: principal.CallerTaskID, SessionID: principal.CallerSessionID,
		},
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rejectedTaskTransferAuditTimeout)
	defer cancel()
	if err := auditor.AuditTaskTransferAttempt(auditCtx, command, result); err != nil {
		h.logger.Error("audit rejected task transfer", zap.Error(err))
	}
}

func (h *Handlers) taskTransferCommand(
	ctx context.Context,
	request taskTransferRequest,
) (models.TaskTransferCommand, error) {
	expectedUpdatedAt, err := time.Parse(time.RFC3339Nano, request.ExpectedTaskUpdatedAt)
	if err != nil {
		return models.TaskTransferCommand{}, err
	}
	command := models.TaskTransferCommand{
		TaskID: request.TaskID, ExpectedSourceWorkspaceID: request.ExpectedSourceWorkspaceID,
		ExpectedSourceWorkflowID: request.ExpectedSourceWorkflowID, ExpectedSourceStepID: request.ExpectedSourceStepID,
		ExpectedTaskUpdatedAt: expectedUpdatedAt, DestinationWorkspaceID: request.DestinationWorkspaceID,
		DestinationWorkflowID: request.DestinationWorkflowID, DestinationStepID: request.DestinationStepID,
		DestinationStepName: request.DestinationStepName, IdempotencyKey: request.IdempotencyKey,
		PreservationPolicy: request.PreservationPolicy,
	}
	if err := validateTaskTransferRequest(command); err != nil {
		return models.TaskTransferCommand{}, err
	}
	actor, err := h.taskTransferActor(ctx, command)
	command.Actor = actor
	return command, err
}

func (h *Handlers) taskTransferActor(ctx context.Context, command models.TaskTransferCommand) (models.TaskTransferActor, error) {
	principal, inSession := mcpscope.PrincipalFromContext(ctx)
	if !inSession {
		identity, _ := authn.IdentityFromContext(ctx)
		actorID := identity.UserID
		if actorID == "" {
			actorID = "local-human"
		}
		return models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: actorID}, nil
	}
	if principal.Surface != mcpprofile.SurfaceOfficeTask {
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	actor, replay, replayErr := h.taskTransferSvc.ResolveTaskTransferReplayActor(ctx, command)
	if replayErr != nil {
		if errors.Is(replayErr, repoerrors.ErrTaskTransferConflict) {
			return models.TaskTransferActor{}, replayErr
		}
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	if replay {
		return h.taskTransferReplayActor(ctx, principal, command, actor)
	}
	if principal.WorkspaceID != command.ExpectedSourceWorkspaceID || h.transferAuthorizer == nil {
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	actor, ok := h.transferAuthorizer.AttestTaskTransferCoordinator(ctx, principal)
	if !ok || actor.Kind != models.TaskTransferActorCoordinator || actor.ID == "" {
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	return actor, nil
}

func (h *Handlers) taskTransferReplayActor(
	ctx context.Context,
	principal mcpscope.Principal,
	command models.TaskTransferCommand,
	persisted models.TaskTransferActor,
) (models.TaskTransferActor, error) {
	if persisted.Kind != models.TaskTransferActorCoordinator || persisted.SessionID != principal.CallerSessionID ||
		h.transferAuthorizer == nil {
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	current, ok := h.transferAuthorizer.AttestTaskTransferCoordinator(ctx, principal)
	if ok && current.Kind == persisted.Kind && current.ID == persisted.ID && current.SessionID == persisted.SessionID {
		return current, nil
	}
	replayAuthorizer, ok := h.transferAuthorizer.(taskTransferCoordinatorReplayAuthorizer)
	if !ok {
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	current, ok = replayAuthorizer.AttestTaskTransferCoordinatorReplay(ctx, principal, command, persisted)
	if !ok || current.Kind != persisted.Kind || current.ID != persisted.ID || current.SessionID != persisted.SessionID {
		return models.TaskTransferActor{}, repoerrors.ErrTaskNotFound
	}
	return current, nil
}

func validateTaskTransferRequest(command models.TaskTransferCommand) error {
	missing := command.TaskID == "" || command.ExpectedSourceWorkspaceID == "" ||
		command.ExpectedSourceWorkflowID == "" || command.ExpectedSourceStepID == "" ||
		command.DestinationWorkspaceID == "" || command.DestinationWorkflowID == "" ||
		(command.DestinationStepID == "" && command.DestinationStepName == "") || command.IdempotencyKey == ""
	if missing || command.PreservationPolicy != models.TaskTransferPreservationPolicyV1 {
		return repoerrors.ErrTaskTransferConflict
	}
	return nil
}
