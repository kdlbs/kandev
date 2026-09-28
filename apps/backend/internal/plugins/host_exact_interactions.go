package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactInteractionManager struct{ host *pluginHost }

func (m exactInteractionManager) RespondPermission(ctx context.Context, in pluginsdk.ExactPermissionResponse) (*pluginsdk.CommandResult, *pluginsdk.Interaction, error) {
	response := HumanInteractionResponse{Kind: string(taskmodels.InteractionKindPermission), OptionID: in.OptionID, Cancelled: in.Cancelled}
	if !validExactInteractionCommand(in.RequestID, in.WorkspaceID, in.InteractionID, in.ExpectedResourceVersion, in.HumanResponseReceiptID, in.ApprovalRevision, in.ManifestDigest) {
		return exactInvalidResult(), nil, nil
	}
	digest := exactCommandDigest(exactRespondPermissionMethod, in)
	commandKey := humanResponseIdempotencyKey(in.HumanResponseReceiptID, digest)
	admission, result := m.host.admitExecutionCommand(ctx, exactRespondPermissionMethod, "host.v2.write:interactions", in.RequestID, in.WorkspaceID, in.InteractionID, commandKey, in.ExpectedResourceVersion, in.ApprovalRevision, in.ManifestDigest, digest)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		interaction, err := m.host.resolveInteraction(ctx, in.InteractionID)
		if err != nil {
			return replay, nil, err
		}
		return replay, ptrInteraction(interactionModelToDTO(interaction)), nil
	}
	responder, interaction, result := m.host.exactHumanInteractionTarget(ctx, in.WorkspaceID, in.InteractionID, in.ExpectedResourceVersion, response)
	if result != nil {
		return m.host.completeHumanInteractionResult(ctx, admission, result, in.InteractionID, in.ExpectedResourceVersion), nil, nil
	}
	optionID, rejected, err := resolvePermissionChoice(interaction, pluginsdk.PermissionResponse{InteractionID: in.InteractionID, OptionID: in.OptionID, Cancelled: in.Cancelled})
	if err != nil {
		return m.host.completeHumanInteractionError(ctx, admission, err, in.InteractionID, in.ExpectedResourceVersion), nil, nil
	}
	if _, ok := m.host.service.humanInteractionReceipts.consume(in.HumanResponseReceiptID, in.WorkspaceID, in.InteractionID, in.ExpectedResourceVersion, response.Kind, response); !ok {
		return m.host.completeHumanInteractionResult(ctx, admission, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "human_response_receipt_invalid_or_used"}, in.InteractionID, in.ExpectedResourceVersion), nil, nil
	}
	err = responder.RespondToPermission(ctx, PluginPermissionResponse{TaskID: interaction.TaskID, SessionID: interaction.SessionID, RequestID: interaction.RequestID, PendingID: interaction.ID, OptionID: optionID, Cancelled: in.Cancelled})
	if err != nil {
		return m.host.completeHumanInteractionError(ctx, admission, err, interaction.ID, in.ExpectedResourceVersion), nil, nil
	}
	resolved := taskmodels.InteractionStatusApproved
	if rejected || in.Cancelled {
		resolved = taskmodels.InteractionStatusRejected
	}
	updated, err := m.host.reloadInteraction(ctx, interaction, resolved)
	if err != nil {
		return exactUnavailableResult("interaction_readback_unavailable"), nil, nil
	}
	result, err = completeExecutionCommand(ctx, admission, pluginsdk.CommandApplied, "", updated.ID, digestPublicValue(*updated))
	return result, updated, err
}

func (m exactInteractionManager) AnswerClarification(ctx context.Context, in pluginsdk.ExactClarificationResponse) (*pluginsdk.CommandResult, *pluginsdk.Interaction, error) {
	response := HumanInteractionResponse{Kind: string(taskmodels.InteractionKindClarification), Answers: in.Answers}
	if !validExactInteractionCommand(in.RequestID, in.WorkspaceID, in.InteractionID, in.ExpectedResourceVersion, in.HumanResponseReceiptID, in.ApprovalRevision, in.ManifestDigest) || len(in.Answers) == 0 {
		return exactInvalidResult(), nil, nil
	}
	digest := exactCommandDigest(exactAnswerClarificationMethod, in)
	commandKey := humanResponseIdempotencyKey(in.HumanResponseReceiptID, digest)
	admission, result := m.host.admitExecutionCommand(ctx, exactAnswerClarificationMethod, "host.v2.write:interactions", in.RequestID, in.WorkspaceID, in.InteractionID, commandKey, in.ExpectedResourceVersion, in.ApprovalRevision, in.ManifestDigest, digest)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		interaction, err := m.host.resolveInteraction(ctx, in.InteractionID)
		if err != nil {
			return replay, nil, err
		}
		return replay, ptrInteraction(interactionModelToDTO(interaction)), nil
	}
	responder, interaction, result := m.host.exactHumanInteractionTarget(ctx, in.WorkspaceID, in.InteractionID, in.ExpectedResourceVersion, response)
	if result != nil {
		return m.host.completeHumanInteractionResult(ctx, admission, result, in.InteractionID, in.ExpectedResourceVersion), nil, nil
	}
	answers := make([]PluginClarificationAnswer, len(in.Answers))
	for index, answer := range in.Answers {
		if answer.QuestionID == "" {
			return m.host.completeHumanInteractionResult(ctx, admission, exactInvalidResult(), in.InteractionID, in.ExpectedResourceVersion), nil, nil
		}
		answers[index] = PluginClarificationAnswer{QuestionID: answer.QuestionID, SelectedOptions: answer.SelectedOptions, CustomText: answer.CustomText}
	}
	if _, ok := m.host.service.humanInteractionReceipts.consume(in.HumanResponseReceiptID, in.WorkspaceID, in.InteractionID, in.ExpectedResourceVersion, response.Kind, response); !ok {
		return m.host.completeHumanInteractionResult(ctx, admission, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "human_response_receipt_invalid_or_used"}, in.InteractionID, in.ExpectedResourceVersion), nil, nil
	}
	if err := responder.AnswerClarification(ctx, interaction.ID, answers); err != nil {
		return m.host.completeHumanInteractionError(ctx, admission, err, interaction.ID, in.ExpectedResourceVersion), nil, nil
	}
	updated, err := m.host.reloadInteraction(ctx, interaction, taskmodels.InteractionStatusAnswered)
	if err != nil {
		return exactUnavailableResult("interaction_readback_unavailable"), nil, nil
	}
	result, err = completeExecutionCommand(ctx, admission, pluginsdk.CommandApplied, "", updated.ID, digestPublicValue(*updated))
	return result, updated, err
}

func (h *pluginHost) exactHumanInteractionTarget(ctx context.Context, workspaceID, interactionID, expectedVersion string, response HumanInteractionResponse) (interactionResponder, *taskmodels.Interaction, *pluginsdk.CommandResult) {
	responder, err := h.interactionWriteTarget()
	if err != nil {
		return nil, nil, exactInteractionErrorResult(err)
	}
	if responder == nil || h.service == nil || h.service.humanInteractionReceipts == nil {
		return nil, nil, exactUnavailableResult("human_interaction_service_unavailable")
	}
	interaction, err := h.answerableInteraction(ctx, interactionID, taskmodels.InteractionKind(response.Kind))
	if err != nil {
		return nil, nil, exactInteractionErrorResult(err)
	}
	if h.taskData == nil {
		return nil, nil, exactUnavailableResult("task_data_unavailable")
	}
	task, err := h.taskData.GetTask(ctx, interaction.TaskID)
	if err != nil || task == nil {
		return nil, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandNotFound, Reason: "interaction_not_found"}
	}
	if task.WorkspaceID != workspaceID {
		return nil, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandNotFound, Reason: "interaction_not_found"}
	}
	version := digestPublicValue(interactionModelToDTO(interaction))
	if version != expectedVersion {
		return nil, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "interaction_revision_changed"}
	}
	return responder, interaction, nil
}

func (h *pluginHost) completeHumanInteractionResult(ctx context.Context, admission *exactExecutionAdmission, result *pluginsdk.CommandResult, target, version string) *pluginsdk.CommandResult {
	completed, err := completeExecutionCommand(ctx, admission, result.Status, result.Reason, target, version)
	if err != nil {
		return exactUnavailableResult("command_receipt_unavailable")
	}
	return completed
}

func (h *pluginHost) completeHumanInteractionError(ctx context.Context, admission *exactExecutionAdmission, err error, target, version string) *pluginsdk.CommandResult {
	result := exactExecutionErrorResult(err)
	if result.Status == pluginsdk.CommandUnavailable {
		return result
	}
	return h.completeHumanInteractionResult(ctx, admission, result, target, version)
}

func validExactInteractionCommand(requestID, workspaceID, interactionID, version, receiptID string, approvalRevision uint64, manifestDigest string) bool {
	return isBoundedApprovalIdentifier(requestID) && isBoundedApprovalIdentifier(workspaceID) &&
		isBoundedApprovalIdentifier(interactionID) && version != "" && isBoundedApprovalIdentifier(receiptID) &&
		approvalRevision > 0 && len(manifestDigest) == 64
}

func humanResponseIdempotencyKey(receiptID, payloadDigest string) string {
	digest := sha256.Sum256([]byte(receiptID + "\x00" + payloadDigest))
	return "human-response-" + hex.EncodeToString(digest[:])
}

func exactInteractionErrorResult(err error) *pluginsdk.CommandResult {
	if status.Code(err) == codes.FailedPrecondition {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "interaction_already_resolved"}
	}
	return exactExecutionErrorResult(err)
}

func ptrInteraction(value pluginsdk.Interaction) *pluginsdk.Interaction { return &value }
