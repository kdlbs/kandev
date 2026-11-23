package backendapp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// exactStepApprovalService is the task-service slice behind plugin step
// approval: the awaiting-approval read and the native Approve path.
type exactStepApprovalService interface {
	GetStepApprovalStatus(context.Context, string) (*taskservice.StepApprovalStatus, error)
	ApproveAwaitingStep(context.Context, string, func(*taskservice.StepApprovalStatus) error) (*taskservice.StepApprovalStatus, *taskservice.ApproveSessionResult, error)
}

func (a pluginsExactExecutionController) GetStepApproval(ctx context.Context, _ string, in pluginsdk.ExactStepApprovalQuery) (pluginsdk.StepApproval, error) {
	if a.steps == nil {
		return pluginsdk.StepApproval{}, status.Error(codes.Unavailable, "step approval is unavailable")
	}
	approval, err := a.steps.GetStepApprovalStatus(ctx, in.SessionID)
	if err != nil {
		return pluginsdk.StepApproval{}, stepApprovalStatusError(err)
	}
	if approval.Task.WorkspaceID != in.WorkspaceID {
		return pluginsdk.StepApproval{}, status.Error(codes.NotFound, "session not found")
	}
	return stepApprovalDTO(approval), nil
}

// ApproveStep approves the awaiting step through task/service.ApproveSession,
// the native Approve path, only while the observed version still holds.
func (a pluginsExactExecutionController) ApproveStep(ctx context.Context, _ string, in pluginsdk.ExactStepApprovalCommand) (pluginsdk.StepApprovalResult, error) {
	if a.steps == nil {
		return pluginsdk.StepApprovalResult{}, status.Error(codes.Unavailable, "step approval is unavailable")
	}
	approval, result, err := a.steps.ApproveAwaitingStep(ctx, in.SessionID, func(observed *taskservice.StepApprovalStatus) error {
		if observed.Task.WorkspaceID != in.WorkspaceID {
			return status.Error(codes.NotFound, "session not found")
		}
		if stepApprovalResourceVersion(observed) != in.ExpectedResourceVersion {
			return status.Error(codes.Aborted, "step approval changed")
		}
		return nil
	})
	if err != nil {
		return pluginsdk.StepApprovalResult{}, stepApprovalStatusError(err)
	}
	approved := pluginsdk.StepApprovalResult{}
	if approval.Step != nil {
		approved.FromStepID = approval.Step.ID
	}
	if result != nil && result.Task != nil && result.Task.WorkflowStepID != approved.FromStepID {
		approved.ToStepID = result.Task.WorkflowStepID
	}
	return approved, nil
}

func stepApprovalStatusError(err error) error {
	switch {
	case status.Code(err) != codes.Unknown:
		return err
	case errors.Is(err, taskservice.ErrOfficeApprovalStage):
		return status.Error(codes.Unimplemented, "office approval stages are decided by their participants")
	case errors.Is(err, taskservice.ErrStepNotAwaitingApproval):
		return status.Error(codes.FailedPrecondition, "step is not awaiting approval")
	case errors.Is(err, taskservice.ErrStepApprovalChanged):
		return status.Error(codes.Aborted, "step approval changed")
	case errors.Is(err, repoerrors.ErrTaskNotFound), errors.Is(err, sql.ErrNoRows), errors.Is(err, taskservice.ErrForbidden):
		return status.Error(codes.NotFound, "session not found")
	default:
		return status.Error(codes.Unavailable, "step approval is unavailable")
	}
}

func stepApprovalDTO(approval *taskservice.StepApprovalStatus) pluginsdk.StepApproval {
	out := pluginsdk.StepApproval{
		SessionID: approval.Session.ID, TaskID: approval.Task.ID, Awaiting: approval.Awaiting,
		ResourceVersion: stepApprovalResourceVersion(approval), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if approval.Step != nil {
		out.FromStepID, out.FromStepName = approval.Step.ID, approval.Step.Name
	}
	if approval.NextStep != nil {
		out.ToStepID, out.ToStepName = approval.NextStep.ID, approval.NextStep.Name
	}
	return out
}

// stepApprovalResourceVersion binds a receipt and an ApproveStep command to
// the observed session revision, review state, step, and approval target.
func stepApprovalResourceVersion(approval *taskservice.StepApprovalStatus) string {
	canonical := map[string]any{
		sessionIDPayloadKey: approval.Session.ID, "session_version": approval.Session.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"review_status": string(approval.Session.ReviewStatus), "step_id": approval.Task.WorkflowStepID,
		"awaiting": approval.Awaiting, "next_step_id": nextStepID(approval),
	}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func nextStepID(approval *taskservice.StepApprovalStatus) string {
	if approval.NextStep == nil {
		return ""
	}
	return approval.NextStep.ID
}
