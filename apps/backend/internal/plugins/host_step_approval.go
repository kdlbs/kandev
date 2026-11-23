package plugins

import (
	"context"
	"encoding/json"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	reasonStepNotAwaitingApproval = "step_not_awaiting_approval"
	reasonStepApprovalChanged     = "step_approval_changed"
	stepApprovalReadSessionParam  = "session_id"
)

// GetStepApproval reads whether a session's current workflow step awaits
// the native Approve action, under host.v2.read:sessions.
func (m exactExecutionManager) GetStepApproval(ctx context.Context, in pluginsdk.ExactStepApprovalQuery) (pluginsdk.StepApproval, error) {
	if !isBoundedApprovalIdentifier(in.RequestID) || !isBoundedApprovalIdentifier(in.WorkspaceID) || !isBoundedApprovalIdentifier(in.SessionID) {
		return pluginsdk.StepApproval{}, status.Error(codes.InvalidArgument, "invalid exact step approval query")
	}
	_, unlock, err := m.host.exactReadAdmission(ctx, in.RequestID, in.WorkspaceID, exactGetStepApprovalMethod, "host.v2.read:sessions", map[string]string{stepApprovalReadSessionParam: in.SessionID})
	if err != nil {
		return pluginsdk.StepApproval{}, err
	}
	defer unlock()
	controller := m.host.executionController()
	if controller == nil {
		return pluginsdk.StepApproval{}, status.Error(codes.Unavailable, "execution control is unavailable")
	}
	return controller.GetStepApproval(ctx, m.host.installationID, in)
}

// ApproveStep spends a step_approval receipt minted for a linked Kandev user
// and approves the awaiting step as that user. The command is admitted under
// host.v2.write:execution; a replay returns ALREADY_APPLIED without a second
// approval.
func (m exactExecutionManager) ApproveStep(ctx context.Context, in pluginsdk.ExactStepApprovalCommand) (*pluginsdk.CommandResult, pluginsdk.StepApprovalResult, error) {
	if !validStepApprovalCommand(in) {
		return exactInvalidResult(), pluginsdk.StepApprovalResult{}, nil
	}
	digest := exactCommandDigest(exactApproveStepMethod, in)
	admission, result := m.host.admitExecutionCommand(ctx, exactApproveStepMethod, "host.v2.write:execution", in.RequestID, in.WorkspaceID, in.SessionID, in.IdempotencyKey, in.ExpectedResourceVersion, in.ApprovalRevision, in.ManifestDigest, digest)
	if result != nil {
		return result, pluginsdk.StepApprovalResult{}, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		return replay, replayedStepApproval(admission, replay), nil
	}
	controller := m.host.executionController()
	if controller == nil || m.host.service.humanInteractionReceipts == nil {
		return exactUnavailableResult("execution_control_unavailable"), pluginsdk.StepApprovalResult{}, nil
	}
	response := HumanInteractionResponse{Kind: pluginsdk.HumanResponseKindStepApproval}
	actingCtx, receipt, result := m.host.consumeHumanResponse(ctx, in.HumanResponseReceiptID, in.WorkspaceID, in.SessionID, in.ExpectedResourceVersion, response, in.ApprovalRevision, in.ManifestDigest)
	if result != nil {
		return m.host.completeHumanInteractionResult(ctx, admission, result, in.SessionID, in.ExpectedResourceVersion), pluginsdk.StepApprovalResult{}, nil
	}
	approved, err := controller.ApproveStep(actingCtx, m.host.installationID, in)
	if err != nil {
		m.host.releaseLinkedResponseClaim(ctx, in.HumanResponseReceiptID, receipt)
		return m.host.completeHumanInteractionResult(ctx, admission, stepApprovalErrorResult(err), in.SessionID, in.ExpectedResourceVersion), pluginsdk.StepApprovalResult{}, nil
	}
	m.host.markLinkedResponseConsumed(ctx, in.HumanResponseReceiptID, receipt)
	data, err := json.Marshal(approved)
	if err != nil {
		return exactUnavailableResult("command_receipt_unavailable"), approved, nil
	}
	completed, err := admission.store.CompleteWithResultData(ctx, admission.record.Intent.OperationID,
		string(pluginsdk.CommandApplied), "", in.SessionID, in.ExpectedResourceVersion, string(data))
	if err != nil {
		return exactUnavailableResult("command_receipt_unavailable"), approved, nil
	}
	return commandResultFromRecord(completed), approved, nil
}

func validStepApprovalCommand(in pluginsdk.ExactStepApprovalCommand) bool {
	return isBoundedApprovalIdentifier(in.RequestID) && isBoundedApprovalIdentifier(in.WorkspaceID) &&
		isBoundedApprovalIdentifier(in.SessionID) && in.ExpectedResourceVersion != "" &&
		isBoundedApprovalIdentifier(in.IdempotencyKey) && isBoundedApprovalIdentifier(in.HumanResponseReceiptID) &&
		in.ApprovalRevision > 0 && len(in.ManifestDigest) == 64
}

func replayedStepApproval(admission *exactExecutionAdmission, result *pluginsdk.CommandResult) pluginsdk.StepApprovalResult {
	var approved pluginsdk.StepApprovalResult
	if result == nil || result.Status != pluginsdk.CommandAlreadyApplied || admission.record.Receipt.ResultData == "" {
		return approved
	}
	_ = json.Unmarshal([]byte(admission.record.Receipt.ResultData), &approved)
	return approved
}

func stepApprovalErrorResult(err error) *pluginsdk.CommandResult {
	switch status.Code(err) {
	case codes.FailedPrecondition:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: reasonStepNotAwaitingApproval}
	case codes.Aborted:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: reasonStepApprovalChanged}
	default:
		return exactExecutionErrorResult(err)
	}
}

// stepApprovalReceiptVersion verifies a session's step awaits approval at
// the observed version before a step_approval receipt is minted.
func (h *pluginHost) stepApprovalReceiptVersion(ctx context.Context, workspaceID, sessionID, expectedVersion string) (string, *pluginsdk.CommandResult) {
	controller := h.executionController()
	if controller == nil {
		return "", exactUnavailableResult("execution_control_unavailable")
	}
	approval, err := controller.GetStepApproval(ctx, h.installationID, pluginsdk.ExactStepApprovalQuery{
		RequestID: "step-approval-receipt", WorkspaceID: workspaceID, SessionID: sessionID,
	})
	if err != nil {
		return "", exactExecutionErrorResult(err)
	}
	if !approval.Awaiting {
		return "", &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: reasonStepNotAwaitingApproval}
	}
	if approval.ResourceVersion != expectedVersion {
		return "", &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: reasonStepApprovalChanged}
	}
	return approval.ResourceVersion, nil
}
