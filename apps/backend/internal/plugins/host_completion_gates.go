package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/state"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	maxExactCompletionCriteria = 64
)

type exactTaskCompletionGateCommandAdapter struct{ host *pluginHost }

func (h *pluginHost) TaskCompletionGates() pluginsdk.ExactTaskCompletionGateCommandManager {
	return exactTaskCompletionGateCommandAdapter{host: h}
}

func (a exactTaskCompletionGateCommandAdapter) SetCriteria(ctx context.Context, input pluginsdk.ExactTaskCompletionCriteria) (*pluginsdk.CommandResult, *pluginsdk.TaskCompletionGate, error) {
	return a.host.SetTaskCompletionCriteriaExact(ctx, input)
}

func (a exactTaskCompletionGateCommandAdapter) Verify(ctx context.Context, input pluginsdk.ExactTaskCompletionEvidence) (*pluginsdk.CommandResult, *pluginsdk.TaskCompletionGate, error) {
	return a.host.VerifyTaskCompletionCriterionExact(ctx, input)
}

func (h *pluginHost) SetTaskCompletionCriteriaExact(ctx context.Context, input pluginsdk.ExactTaskCompletionCriteria) (*pluginsdk.CommandResult, *pluginsdk.TaskCompletionGate, error) {
	criteria := make([]taskmodels.TaskCompletionCriterion, 0, len(input.Criteria))
	for _, item := range input.Criteria {
		criteria = append(criteria, taskmodels.TaskCompletionCriterion{
			ID: item.ID, Description: item.Description,
			EvidenceSubject: taskmodels.TaskCompletionEvidenceSubject{Kind: item.EvidenceSubject.Kind, ID: item.EvidenceSubject.ID},
		})
	}
	if !validExactTaskCompletionBase(input.RequestID, input.WorkspaceID, input.TaskID, input.IdempotencyKey,
		input.ExpectedTaskResourceVersion, input.ExpectedRevision, input.ManagementInstanceKey, input.ExpectedClaimGeneration) ||
		len(criteria) > maxExactCompletionCriteria || !validExactCompletionCriteria(criteria) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	digest, err := exactTaskCompletionCriteriaDigest(input, criteria)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	admission, result := h.admitExactTaskCompletionGate(ctx, input.RequestID, input.WorkspaceID, input.TaskID,
		input.IdempotencyKey, input.ExpectedTaskResourceVersion, input.ApprovalRevision, input.ManifestDigest,
		digest, exactTaskCompletionCriteriaMethod)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted && !successfulTaskGateReceipt(admission.record) {
		return commandResultFromRecord(admission.record), nil, nil
	}
	snapshot, alreadyApplied, err := admission.writer.SetTaskCompletionCriteriaExact(ctx, ExactTaskCompletionCriteriaInput{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion, ExpectedRevision: input.ExpectedRevision,
		OperationID: admission.record.Intent.OperationID, PayloadDigest: digest,
		ClaimFence: completionGateClaimFence(h, input.ManagementInstanceKey, input.ExpectedClaimGeneration), Criteria: criteria,
	})
	if err != nil {
		return h.completeTaskCompletionGateError(ctx, admission.store, admission.record, input.TaskID, err), nil, nil
	}
	return h.completeTaskCompletionGate(ctx, admission.store, admission.record, snapshot, alreadyApplied)
}

func (h *pluginHost) VerifyTaskCompletionCriterionExact(ctx context.Context, input pluginsdk.ExactTaskCompletionEvidence) (*pluginsdk.CommandResult, *pluginsdk.TaskCompletionGate, error) {
	evidence := taskmodels.TaskCompletionEvidence{
		Subject: taskmodels.TaskCompletionEvidenceSubject{
			Kind: input.Evidence.Subject.Kind, ID: input.Evidence.Subject.ID, Revision: input.Evidence.Subject.Revision,
		},
		Summary: input.Evidence.Summary, Reference: input.Evidence.Reference,
	}
	if !validExactTaskCompletionBase(input.RequestID, input.WorkspaceID, input.TaskID, input.IdempotencyKey,
		input.ExpectedTaskResourceVersion, input.ExpectedRevision, input.ManagementInstanceKey, input.ExpectedClaimGeneration) ||
		!isBoundedApprovalIdentifier(input.CriterionID) || !validTaskCompletionEvidence(evidence) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	digest, err := exactTaskCompletionEvidenceDigest(input, evidence)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	admission, result := h.admitExactTaskCompletionGate(ctx, input.RequestID, input.WorkspaceID, input.TaskID,
		input.IdempotencyKey, input.ExpectedTaskResourceVersion, input.ApprovalRevision, input.ManifestDigest,
		digest, exactTaskCompletionEvidenceMethod)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted && !successfulTaskGateReceipt(admission.record) {
		return commandResultFromRecord(admission.record), nil, nil
	}
	snapshot, alreadyApplied, err := admission.writer.VerifyTaskCompletionCriterionExact(ctx, ExactTaskCompletionEvidenceInput{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion, ExpectedRevision: input.ExpectedRevision,
		OperationID: admission.record.Intent.OperationID, PayloadDigest: digest,
		ClaimFence:  completionGateClaimFence(h, input.ManagementInstanceKey, input.ExpectedClaimGeneration),
		CriterionID: input.CriterionID, Evidence: evidence,
	})
	if err != nil {
		return h.completeTaskCompletionGateError(ctx, admission.store, admission.record, input.TaskID, err), nil, nil
	}
	return h.completeTaskCompletionGate(ctx, admission.store, admission.record, snapshot, alreadyApplied)
}

type exactTaskCompletionGateAdmission struct {
	writer exactTaskCompletionGateWriter
	store  *state.CommandStore
	record state.CommandRecord
	unlock func()
}

func (h *pluginHost) admitExactTaskCompletionGate(
	ctx context.Context,
	requestID, workspaceID, taskID, idempotencyKey, expectedTaskVersion string,
	approvalRevision uint64, requestedManifestDigest, payloadDigest, method string,
) (*exactTaskCompletionGateAdmission, *pluginsdk.CommandResult) {
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
	if requestedManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, workspaceID, "host.v2.write:tasks", approvalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", method, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	writer, ok := h.taskWriter.(exactTaskCompletionGateWriter)
	if !ok {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "task_completion_gates_unsupported"}
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
		InstallationID: h.installationID, WorkspaceID: workspaceID,
		RequestID: requestID, IdempotencyKey: idempotencyKey,
		Method: method, CapabilityID: "host.v2.write:tasks", PayloadDigest: payloadDigest,
		TargetID: taskID, ExpectedResourceVersion: expectedTaskVersion,
		ApprovalRevision: approvalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactTaskCompletionGateAdmission{writer: writer, store: store, record: record, unlock: unlock}, nil
}

func (h *pluginHost) completeTaskCompletionGate(
	ctx context.Context,
	store *state.CommandStore,
	record state.CommandRecord,
	snapshot *taskmodels.TaskCompletionGateSnapshot,
	alreadyApplied bool,
) (*pluginsdk.CommandResult, *pluginsdk.TaskCompletionGate, error) {
	if snapshot == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "completion_gate_result_unavailable"}, nil, nil
	}
	gate := taskCompletionGateToSDK(snapshot)
	if record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(record), gate, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	completed, err := store.Complete(ctx, record.Intent.OperationID, string(resultStatus), "", snapshot.TaskID,
		fmt.Sprintf("completion:%d", snapshot.Revision))
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, gate, nil
	}
	return commandResultFromRecord(completed), gate, nil
}

func (h *pluginHost) completeTaskCompletionGateError(ctx context.Context, store *state.CommandStore, record state.CommandRecord, taskID string, err error) *pluginsdk.CommandResult {
	commandStatus, reason := pluginsdk.CommandUnavailable, "task_completion_gate_unavailable"
	switch {
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		commandStatus, reason = pluginsdk.CommandNotFound, "task_not_found"
	case errors.Is(err, repoerrors.ErrTaskVersionConflict):
		commandStatus, reason = pluginsdk.CommandConflict, "task_version_changed"
	case errors.Is(err, repoerrors.ErrTaskManagementClaimConflict), errors.Is(err, repoerrors.ErrTaskOperationConflict):
		commandStatus, reason = pluginsdk.CommandConflict, "management_claim_or_operation_changed"
	case errors.Is(err, repoerrors.ErrTaskCompletionCriteriaConflict):
		commandStatus, reason = pluginsdk.CommandConflict, "completion_criteria_changed"
	case errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged):
		commandStatus, reason = pluginsdk.CommandConflict, "completion_evidence_changed"
	case errors.Is(err, repoerrors.ErrTaskCompletionHumanConfirmationRequired):
		commandStatus, reason = pluginsdk.CommandConflict, "human_confirmation_required"
	default:
		return &pluginsdk.CommandResult{Status: commandStatus, Reason: reason}
	}
	completed, completeErr := store.Complete(ctx, record.Intent.OperationID, string(commandStatus), reason, taskID, "")
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}
	}
	return commandResultFromRecord(completed)
}

func successfulTaskGateReceipt(record state.CommandRecord) bool {
	return record.Receipt.ResultStatus == string(pluginsdk.CommandApplied) ||
		record.Receipt.ResultStatus == string(pluginsdk.CommandAlreadyApplied)
}

func completionGateClaimFence(host *pluginHost, instanceKey string, generation int64) taskmodels.TaskManagementClaimFence {
	return taskmodels.TaskManagementClaimFence{InstallationID: host.installationID, InstanceKey: instanceKey, Generation: generation}
}

func validExactTaskCompletionBase(requestID, workspaceID, taskID, idempotencyKey, taskVersion string, revision int64, instanceKey string, generation int64) bool {
	if !isBoundedApprovalIdentifier(requestID) || !isBoundedApprovalIdentifier(workspaceID) ||
		!isBoundedApprovalIdentifier(taskID) || !isBoundedApprovalIdentifier(idempotencyKey) ||
		revision < 0 || !validManagementClaimFence(instanceKey, generation) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, taskVersion)
	return err == nil
}

func validExactCompletionCriteria(criteria []taskmodels.TaskCompletionCriterion) bool {
	seen := make(map[string]struct{}, len(criteria))
	for _, item := range criteria {
		if strings.TrimSpace(item.ID) != item.ID || item.ID == "" || len(item.ID) > 128 ||
			strings.TrimSpace(item.Description) == "" || len(item.Description) > 2000 ||
			!validCompletionEvidenceSubject(item.EvidenceSubject, false) {
			return false
		}
		if _, exists := seen[item.ID]; exists {
			return false
		}
		seen[item.ID] = struct{}{}
	}
	return true
}

func validTaskCompletionEvidence(evidence taskmodels.TaskCompletionEvidence) bool {
	return validCompletionEvidenceSubject(evidence.Subject, true) && len(evidence.Summary) <= 2000 && len(evidence.Reference) <= 2048
}

func validCompletionEvidenceSubject(subject taskmodels.TaskCompletionEvidenceSubject, requireRevision bool) bool {
	if strings.TrimSpace(subject.ID) != subject.ID || subject.ID == "" || len(subject.ID) > 256 || len(subject.Revision) > 256 {
		return false
	}
	if requireRevision != (subject.Revision != "") {
		return false
	}
	switch subject.Kind {
	case taskmodels.TaskCompletionEvidenceTaskRevision, taskmodels.TaskCompletionEvidenceExecution,
		taskmodels.TaskCompletionEvidenceArtifact, taskmodels.TaskCompletionEvidenceGitHubPRHead:
		return true
	default:
		return false
	}
}

func exactTaskCompletionCriteriaDigest(input pluginsdk.ExactTaskCompletionCriteria, criteria []taskmodels.TaskCompletionCriterion) (string, error) {
	canonical := struct {
		WorkspaceID     string                               `json:"workspace_id"`
		TaskID          string                               `json:"task_id"`
		TaskVersion     string                               `json:"task_version"`
		Revision        int64                                `json:"revision"`
		InstanceKey     string                               `json:"instance_key"`
		ClaimGeneration int64                                `json:"claim_generation"`
		Criteria        []taskmodels.TaskCompletionCriterion `json:"criteria"`
	}{input.WorkspaceID, input.TaskID, input.ExpectedTaskResourceVersion, input.ExpectedRevision,
		input.ManagementInstanceKey, input.ExpectedClaimGeneration, criteria}
	return digestCompletionGatePayload(canonical)
}

func exactTaskCompletionEvidenceDigest(input pluginsdk.ExactTaskCompletionEvidence, evidence taskmodels.TaskCompletionEvidence) (string, error) {
	canonical := struct {
		WorkspaceID     string                            `json:"workspace_id"`
		TaskID          string                            `json:"task_id"`
		TaskVersion     string                            `json:"task_version"`
		Revision        int64                             `json:"revision"`
		CriterionID     string                            `json:"criterion_id"`
		InstanceKey     string                            `json:"instance_key"`
		ClaimGeneration int64                             `json:"claim_generation"`
		Evidence        taskmodels.TaskCompletionEvidence `json:"evidence"`
	}{input.WorkspaceID, input.TaskID, input.ExpectedTaskResourceVersion, input.ExpectedRevision,
		input.CriterionID, input.ManagementInstanceKey, input.ExpectedClaimGeneration, evidence}
	return digestCompletionGatePayload(canonical)
}

func digestCompletionGatePayload(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 256*1024 {
		return "", fmt.Errorf("invalid completion gate payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func taskCompletionGateToSDK(value *taskmodels.TaskCompletionGateSnapshot) *pluginsdk.TaskCompletionGate {
	if value == nil {
		return nil
	}
	gate := &pluginsdk.TaskCompletionGate{
		TaskID: value.TaskID, WorkspaceID: value.WorkspaceID, Revision: value.Revision, Blocked: value.Blocked,
		Criteria: make([]pluginsdk.TaskCompletionCriterion, 0, len(value.Criteria)),
		Blockers: make([]pluginsdk.TaskCompletionBlocker, 0, len(value.Blockers)),
	}
	for _, item := range value.Criteria {
		criterion := pluginsdk.TaskCompletionCriterion{
			ID: item.ID, Description: item.Description,
			EvidenceSubject:   pluginsdk.TaskCompletionEvidenceSubject{Kind: item.EvidenceSubject.Kind, ID: item.EvidenceSubject.ID, Revision: item.EvidenceSubject.Revision},
			CriterionRevision: item.CriterionRevision, VerifiedRevision: item.VerifiedRevision,
			VerifierKind: item.VerifierKind, VerifierID: item.VerifierID,
		}
		if item.Evidence != nil {
			criterion.Evidence = &pluginsdk.TaskCompletionEvidence{
				Subject: pluginsdk.TaskCompletionEvidenceSubject{Kind: item.Evidence.Subject.Kind, ID: item.Evidence.Subject.ID, Revision: item.Evidence.Subject.Revision},
				Summary: item.Evidence.Summary, Reference: item.Evidence.Reference,
			}
		}
		if item.VerifiedAt != nil {
			criterion.VerifiedAt = item.VerifiedAt.UTC().Format(time.RFC3339Nano)
		}
		gate.Criteria = append(gate.Criteria, criterion)
	}
	for _, item := range value.Blockers {
		gate.Blockers = append(gate.Blockers, pluginsdk.TaskCompletionBlocker{CriterionID: item.CriterionID, Reason: item.Reason})
	}
	return gate
}
