package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/plugins/state"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const exactTaskCreateMethod = "CreateTaskExact"

type exactTaskCommandManager struct{ host *pluginHost }

func (h *pluginHost) TaskCommands() pluginsdk.ExactTaskCommandManager {
	return exactTaskCommandManager{host: h}
}

func (m exactTaskCommandManager) CreateTask(ctx context.Context, input pluginsdk.ExactTaskCreate) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return m.host.CreateTaskExact(ctx, input)
}

func (m exactTaskCommandManager) SetLabels(ctx context.Context, input pluginsdk.ExactTaskLabels) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return m.host.SetTaskLabelsExact(ctx, input)
}

func (m exactTaskCommandManager) Assign(ctx context.Context, input pluginsdk.ExactTaskAssignment) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return m.host.AssignTaskExact(ctx, input)
}

func (m exactTaskCommandManager) Move(ctx context.Context, input pluginsdk.ExactTaskMove) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return m.host.MoveTaskExact(ctx, input)
}

func (m exactTaskCommandManager) Archive(ctx context.Context, input pluginsdk.ExactTaskArchive) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return m.host.ArchiveTaskExact(ctx, input)
}

func (m exactTaskCommandManager) AddRelation(ctx context.Context, input pluginsdk.ExactTaskRelation) (*pluginsdk.CommandResult, error) {
	return m.host.exactTaskRelation(ctx, input, exactTaskRelationAddMethod, true)
}

func (m exactTaskCommandManager) RemoveRelation(ctx context.Context, input pluginsdk.ExactTaskRelation) (*pluginsdk.CommandResult, error) {
	return m.host.exactTaskRelation(ctx, input, exactTaskRelationRemoveMethod, false)
}

func (m exactTaskCommandManager) SendMessage(ctx context.Context, input pluginsdk.ExactTaskMessage) (*pluginsdk.CommandResult, error) {
	return m.host.sendTaskMessageExact(ctx, input)
}

type exactTaskCreateAdmission struct {
	writer exactTaskCreateWriter
	store  *state.CommandStore
	record state.CommandRecord
	unlock func()
}

type exactTaskMoveAdmission struct {
	writer   exactTaskMoveWriter
	store    *state.CommandStore
	record   state.CommandRecord
	replayed bool
	unlock   func()
}

type exactTaskArchiveAdmission struct {
	writer exactTaskArchiveWriter
	store  *state.CommandStore
	record state.CommandRecord
	unlock func()
}

type exactTaskRelationAdmission struct {
	writer exactTaskRelationWriter
	store  *state.CommandStore
	record state.CommandRecord
	unlock func()
}

func validExactTaskRelation(input pluginsdk.ExactTaskRelation) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) ||
		!isBoundedApprovalIdentifier(input.WorkspaceID) || !isBoundedApprovalIdentifier(input.TaskID) ||
		!isBoundedApprovalIdentifier(input.RelatedTaskID) || input.TaskID == input.RelatedTaskID ||
		!isBoundedApprovalIdentifier(input.IdempotencyKey) || input.ApprovalRevision == 0 || len(input.ManifestDigest) != 64 ||
		!validManagementClaimFence(input.ManagementInstanceKey, input.ExpectedClaimGeneration) {
		return false
	}
	_, taskErr := time.Parse(time.RFC3339Nano, input.ExpectedTaskResourceVersion)
	_, relatedErr := time.Parse(time.RFC3339Nano, input.ExpectedRelatedResourceVersion)
	return taskErr == nil && relatedErr == nil
}

func exactTaskRelationDigest(input pluginsdk.ExactTaskRelation, method string) (string, error) {
	canonical := struct {
		Method                  string `json:"method"`
		WorkspaceID             string `json:"workspace_id"`
		TaskID                  string `json:"task_id"`
		RelatedTaskID           string `json:"related_task_id"`
		TaskVersion             string `json:"task_version"`
		RelatedVersion          string `json:"related_version"`
		ManagementInstanceKey   string `json:"management_instance_key"`
		ExpectedClaimGeneration int64  `json:"expected_claim_generation"`
	}{method, input.WorkspaceID, input.TaskID, input.RelatedTaskID,
		input.ExpectedTaskResourceVersion, input.ExpectedRelatedResourceVersion,
		input.ManagementInstanceKey, input.ExpectedClaimGeneration}
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 65536 {
		return "", fmt.Errorf("invalid exact task relation payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (h *pluginHost) exactTaskRelation(
	ctx context.Context,
	input pluginsdk.ExactTaskRelation,
	method string,
	add bool,
) (*pluginsdk.CommandResult, error) {
	if !validExactTaskRelation(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	digest, err := exactTaskRelationDigest(input, method)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	admission, result := h.admitExactTaskRelation(ctx, input, digest, method)
	if result != nil {
		return result, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(admission.record), nil
	}
	command := ExactTaskRelationInput{
		WorkspaceID: input.WorkspaceID, TaskID: input.TaskID, RelatedTaskID: input.RelatedTaskID,
		ExpectedTaskResourceVersion:    input.ExpectedTaskResourceVersion,
		ExpectedRelatedResourceVersion: input.ExpectedRelatedResourceVersion,
		OperationID:                    admission.record.Intent.OperationID, PayloadDigest: digest,
		ClaimFence: taskmodels.TaskManagementClaimFence{
			InstallationID: h.installationID, InstanceKey: input.ManagementInstanceKey,
			Generation: input.ExpectedClaimGeneration,
		},
	}
	var alreadyApplied bool
	if add {
		alreadyApplied, err = admission.writer.AddTaskRelationExact(ctx, command)
	} else {
		alreadyApplied, err = admission.writer.RemoveTaskRelationExact(ctx, command)
	}
	if err != nil {
		resultStatus := exactTaskUpdateErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "task_relation_unavailable"}, nil
		}
		completed, completeErr := admission.store.Complete(ctx, admission.record.Intent.OperationID,
			string(resultStatus), string(resultStatus), input.TaskID, "")
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
		}
		return commandResultFromRecord(completed), nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), "", input.TaskID, "")
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
	}
	return commandResultFromRecord(completed), nil
}

func (h *pluginHost) admitExactTaskRelation(
	ctx context.Context,
	input pluginsdk.ExactTaskRelation,
	payloadDigest, method string,
) (*exactTaskRelationAdmission, *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if input.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, input.WorkspaceID, "host.v2.write:tasks", input.ApprovalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", method, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	writer, ok := h.taskWriter.(exactTaskRelationWriter)
	if !ok {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "exact_task_relations_unsupported"}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
	}
	record, _, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: method, CapabilityID: "host.v2.write:tasks", PayloadDigest: payloadDigest,
		TargetID:                input.TaskID,
		ExpectedResourceVersion: input.ExpectedTaskResourceVersion + "/" + input.ExpectedRelatedResourceVersion,
		ApprovalRevision:        input.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactTaskRelationAdmission{writer: writer, store: store, record: record, unlock: unlock}, nil
}

func (h *pluginHost) ArchiveTaskExact(ctx context.Context, input pluginsdk.ExactTaskArchive) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	if !validExactTaskArchive(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	digest, err := exactTaskArchiveDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	admission, result := h.admitExactTaskArchive(ctx, input, digest)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(admission.record), h.exactTaskUpdateReplayValue(ctx, input.TaskID), nil
	}
	archived, alreadyApplied, err := admission.writer.ArchiveTaskExact(ctx, ExactTaskArchiveInput{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ExpectedResourceVersion: input.ExpectedResourceVersion,
		OperationID:             admission.record.Intent.OperationID, PayloadDigest: digest,
		ClaimFence: taskmodels.TaskManagementClaimFence{
			InstallationID: h.installationID, InstanceKey: input.ManagementInstanceKey,
			Generation: input.ExpectedClaimGeneration,
		},
	})
	if err != nil {
		resultStatus := exactTaskUpdateErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "task_archive_unavailable"}, nil, nil
		}
		completed, completeErr := admission.store.Complete(ctx, admission.record.Intent.OperationID,
			string(resultStatus), string(resultStatus), input.TaskID, "")
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil, nil
		}
		return commandResultFromRecord(completed), nil, nil
	}
	if archived == nil || archived.ID == "" {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "task_archive_result_unavailable"}, nil, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	dto := taskModelToDTO(archived)
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), "", archived.ID, dto.ResourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, &dto, nil
	}
	return commandResultFromRecord(completed), &dto, nil
}

func validExactTaskArchive(input pluginsdk.ExactTaskArchive) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.TaskID) || !isBoundedApprovalIdentifier(input.IdempotencyKey) ||
		input.ApprovalRevision == 0 || len(input.ManifestDigest) != 64 ||
		!validManagementClaimFence(input.ManagementInstanceKey, input.ExpectedClaimGeneration) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, input.ExpectedResourceVersion)
	return err == nil
}

func exactTaskArchiveDigest(input pluginsdk.ExactTaskArchive) (string, error) {
	canonical := struct {
		WorkspaceID             string `json:"workspace_id"`
		TaskID                  string `json:"task_id"`
		ExpectedResourceVersion string `json:"expected_resource_version"`
		ManagementInstanceKey   string `json:"management_instance_key"`
		ExpectedClaimGeneration int64  `json:"expected_claim_generation"`
	}{
		WorkspaceID: input.WorkspaceID, TaskID: input.TaskID, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 65536 {
		return "", fmt.Errorf("invalid exact task archive payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (h *pluginHost) admitExactTaskArchive(
	ctx context.Context,
	input pluginsdk.ExactTaskArchive,
	payloadDigest string,
) (*exactTaskArchiveAdmission, *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if input.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, input.WorkspaceID, "host.v2.write:tasks", input.ApprovalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", exactTaskArchiveMethod, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	writer, ok := h.taskWriter.(exactTaskArchiveWriter)
	if !ok {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "exact_task_archive_unsupported"}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
	}
	record, _, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: exactTaskArchiveMethod, CapabilityID: "host.v2.write:tasks", PayloadDigest: payloadDigest,
		TargetID: input.TaskID, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactTaskArchiveAdmission{writer: writer, store: store, record: record, unlock: unlock}, nil
}

func (h *pluginHost) MoveTaskExact(ctx context.Context, input pluginsdk.ExactTaskMove) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	if !validExactTaskMove(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	digest, err := exactTaskMoveDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	admission, result := h.admitExactTaskMove(ctx, input, digest)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(admission.record), h.exactTaskUpdateReplayValue(ctx, input.TaskID), nil
	}
	updated, alreadyApplied, err := admission.writer.MoveTaskExact(ctx, ExactTaskMoveInput{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ExpectedResourceVersion: input.ExpectedResourceVersion,
		OperationID:             admission.record.Intent.OperationID, PayloadDigest: digest,
		ClaimFence: taskmodels.TaskManagementClaimFence{
			InstallationID: h.installationID, InstanceKey: input.ManagementInstanceKey,
			Generation: input.ExpectedClaimGeneration,
		},
		Move: TaskMoveInput{
			TaskID: input.TaskID, WorkflowStepID: input.WorkflowStepID,
			WorkflowID: optionalTaskWorkflowID(input.WorkflowID), Position: input.Position,
			Source: h.pluginSource(),
		},
	})
	if err != nil {
		resultStatus := exactTaskMoveErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "task_move_unavailable"}, nil, nil
		}
		completed, completeErr := admission.store.Complete(ctx, admission.record.Intent.OperationID,
			string(resultStatus), string(resultStatus), input.TaskID, "")
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil, nil
		}
		return commandResultFromRecord(completed), nil, nil
	}
	if updated == nil || updated.ID == "" {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "task_move_result_unavailable"}, nil, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	dto := taskModelToDTO(updated)
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), "", updated.ID, dto.ResourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, &dto, nil
	}
	return commandResultFromRecord(completed), &dto, nil
}

func optionalTaskWorkflowID(workflowID string) *string {
	if workflowID == "" {
		return nil
	}
	return &workflowID
}

func validExactTaskMove(input pluginsdk.ExactTaskMove) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.TaskID) || !isBoundedApprovalIdentifier(input.IdempotencyKey) ||
		!isBoundedApprovalIdentifier(input.WorkflowStepID) || input.ApprovalRevision == 0 || len(input.ManifestDigest) != 64 ||
		!validManagementClaimFence(input.ManagementInstanceKey, input.ExpectedClaimGeneration) {
		return false
	}
	if input.WorkflowID != "" && !isBoundedApprovalIdentifier(input.WorkflowID) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, input.ExpectedResourceVersion)
	return err == nil
}

func exactTaskMoveDigest(input pluginsdk.ExactTaskMove) (string, error) {
	canonical := struct {
		WorkspaceID             string `json:"workspace_id"`
		TaskID                  string `json:"task_id"`
		ExpectedResourceVersion string `json:"expected_resource_version"`
		WorkflowID              string `json:"workflow_id"`
		WorkflowStepID          string `json:"workflow_step_id"`
		Position                int32  `json:"position"`
		ManagementInstanceKey   string `json:"management_instance_key"`
		ExpectedClaimGeneration int64  `json:"expected_claim_generation"`
	}{
		WorkspaceID: input.WorkspaceID, TaskID: input.TaskID,
		ExpectedResourceVersion: input.ExpectedResourceVersion,
		WorkflowID:              input.WorkflowID, WorkflowStepID: input.WorkflowStepID, Position: input.Position,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 65536 {
		return "", fmt.Errorf("invalid exact task move payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (h *pluginHost) admitExactTaskMove(
	ctx context.Context,
	input pluginsdk.ExactTaskMove,
	payloadDigest string,
) (*exactTaskMoveAdmission, *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if input.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, input.WorkspaceID, "host.v2.write:tasks", input.ApprovalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", exactTaskMoveMethod, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	writer, ok := h.taskWriter.(exactTaskMoveWriter)
	if !ok {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "exact_task_move_unsupported"}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
	}
	record, replayed, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: exactTaskMoveMethod, CapabilityID: "host.v2.write:tasks", PayloadDigest: payloadDigest,
		TargetID: input.TaskID, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactTaskMoveAdmission{writer: writer, store: store, record: record, replayed: replayed, unlock: unlock}, nil
}

func exactTaskMoveErrorStatus(err error) pluginsdk.CommandStatus {
	switch status.Code(err) {
	case codes.NotFound:
		return pluginsdk.CommandNotFound
	case codes.InvalidArgument:
		return pluginsdk.CommandInvalid
	case codes.Aborted, codes.AlreadyExists, codes.FailedPrecondition:
		return pluginsdk.CommandConflict
	default:
		return exactTaskUpdateErrorStatus(err)
	}
}

func (h *pluginHost) CreateTaskExact(ctx context.Context, input pluginsdk.ExactTaskCreate) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	if !validExactTaskCreate(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	digest, err := exactTaskCreateDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	admission, result := h.admitExactTaskCreate(ctx, input, digest)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(admission.record), h.exactTaskReplayValue(ctx, admission.record.Receipt.TargetID), nil
	}
	prepared, launch, err := (taskReader{host: h}).prepareCreate(ctx, input.Task)
	if err != nil {
		resultStatus := exactTaskUpdateErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "task_create_validation_unavailable"}, nil, nil
		}
		completed, completeErr := admission.store.Complete(ctx, admission.record.Intent.OperationID,
			string(resultStatus), string(resultStatus), input.ExternalID, "")
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil, nil
		}
		return commandResultFromRecord(completed), nil, nil
	}

	created, alreadyApplied, err := admission.writer.CreateTaskExact(ctx, ExactTaskCreateInput{
		Task: prepared, ExternalID: input.ExternalID,
		OperationID: admission.record.Intent.OperationID, PayloadDigest: digest,
	})
	if err != nil {
		return &pluginsdk.CommandResult{Status: exactTaskUpdateErrorStatus(err), Reason: "task_create_unavailable"}, nil, nil
	}
	if created == nil || created.ID == "" {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "task_create_result_unavailable"}, nil, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	if input.Task.StartAgent {
		h.startTaskBestEffort(ctx, created.ID, launch)
	}
	dto := taskModelToDTO(created)
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), "", created.ID, dto.ResourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, &dto, nil
	}
	return commandResultFromRecord(completed), &dto, nil
}

func (h *pluginHost) admitExactTaskCreate(
	ctx context.Context,
	input pluginsdk.ExactTaskCreate,
	payloadDigest string,
) (*exactTaskCreateAdmission, *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if input.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, input.WorkspaceID, "host.v2.write:tasks", input.ApprovalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", exactTaskCreateMethod, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	writer, ok := h.taskWriter.(exactTaskCreateWriter)
	if !ok {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "exact_task_create_unsupported"}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
	}
	record, _, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: exactTaskCreateMethod, CapabilityID: "host.v2.write:tasks",
		PayloadDigest: payloadDigest, TargetID: input.ExternalID,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactTaskCreateAdmission{writer: writer, store: store, record: record, unlock: unlock}, nil
}

func validExactTaskCreate(input pluginsdk.ExactTaskCreate) bool {
	return isBoundedApprovalIdentifier(input.RequestID) &&
		isBoundedApprovalIdentifier(input.WorkspaceID) &&
		isBoundedApprovalIdentifier(input.IdempotencyKey) &&
		isBoundedApprovalIdentifier(input.ExternalID) &&
		input.ApprovalRevision > 0 && len(input.ManifestDigest) == 64 &&
		input.Task.WorkspaceID == input.WorkspaceID && input.Task.Title != ""
}

func exactTaskCreateDigest(input pluginsdk.ExactTaskCreate) (string, error) {
	canonical := map[string]any{
		"workspace_id": input.WorkspaceID,
		"external_id":  input.ExternalID,
		"task":         input.Task,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode exact task create: %w", err)
	}
	if len(encoded) > 65536 {
		return "", errors.New("exact task create payload is too large")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (h *pluginHost) exactTaskReplayValue(ctx context.Context, taskID string) *pluginsdk.Task {
	if taskID == "" || h.taskData == nil {
		return nil
	}
	model, err := h.taskData.GetTask(ctx, taskID)
	if err != nil || model == nil || model.WorkspaceID == "" {
		return nil
	}
	dto := taskModelToDTO(model)
	return &dto
}
