package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	exactTaskChangeRequestReportMethod = "ReportTaskChangeRequestExact"
	exactTaskChangeRequestRemoveMethod = "RemoveTaskChangeRequestExact"
	exactTaskChangeRequestCapability   = "host.v2.write:task_change_requests"
)

type exactTaskChangeRequestCommandManager struct{ host *pluginHost }

func (h *pluginHost) TaskChangeRequests() pluginsdk.ExactTaskChangeRequestCommandManager {
	return exactTaskChangeRequestCommandManager{host: h}
}

func (m exactTaskChangeRequestCommandManager) Report(ctx context.Context, input pluginsdk.ExactTaskChangeRequestReport) (*pluginsdk.CommandResult, pluginsdk.TaskChangeRequest, error) {
	return m.host.reportTaskChangeRequestExact(ctx, input)
}

func (m exactTaskChangeRequestCommandManager) Remove(ctx context.Context, input pluginsdk.ExactTaskChangeRequestRemove) (*pluginsdk.CommandResult, error) {
	return m.host.removeTaskChangeRequestExact(ctx, input)
}

// validChangeRequestEnvelope checks the command envelope shared by report and
// remove: bounded identifiers plus the approval revision and manifest digest.
func validChangeRequestEnvelope(requestID, workspaceID, idempotencyKey string, approvalRevision uint64, manifestDigest string) bool {
	if !isBoundedApprovalIdentifier(requestID) || !isBoundedApprovalIdentifier(workspaceID) ||
		!isBoundedApprovalIdentifier(idempotencyKey) {
		return false
	}
	return approvalRevision != 0 && validManifestDigest(manifestDigest)
}

// validChangeRequestIdentity checks the provider, repository, and number that
// name the change request on its code host.
func validChangeRequestIdentity(providerID, repositoryID string, number int64) bool {
	provider := strings.TrimSpace(providerID)
	if provider == "" || len(provider) > 64 || number <= 0 {
		return false
	}
	repository := strings.TrimSpace(repositoryID)
	return repository != "" && len(repository) <= 512
}

func isKnownChangeRequestState(value string) bool {
	switch value {
	case state.ChangeRequestStateOpen, state.ChangeRequestStateMerged, state.ChangeRequestStateClosed:
		return true
	default:
		return false
	}
}

func validOptionalChangeRequestTimestamp(value string) bool {
	return value == "" || validChangeRequestTimestamp(value)
}

func validTaskChangeRequestReport(input pluginsdk.ExactTaskChangeRequestReport) bool {
	if !isBoundedApprovalIdentifier(input.TaskID) ||
		!validChangeRequestEnvelope(input.RequestID, input.WorkspaceID, input.IdempotencyKey, input.ApprovalRevision, input.ManifestDigest) ||
		!validChangeRequestIdentity(input.ProviderID, input.RepositoryID, input.Number) {
		return false
	}
	if !isKnownChangeRequestState(input.State) {
		return false
	}
	if len(input.URL) > 2048 || len(input.Title) > 512 || len(input.HeadBranch) > 256 || len(input.BaseBranch) > 256 {
		return false
	}
	if !validOptionalChangeRequestTimestamp(input.CreatedAt) ||
		!validOptionalChangeRequestTimestamp(input.MergedAt) ||
		!validOptionalChangeRequestTimestamp(input.ClosedAt) {
		return false
	}
	return validChangeRequestStateTimestamps(input.State, input.MergedAt, input.ClosedAt)
}

func validChangeRequestStateTimestamps(stateValue, mergedAt, closedAt string) bool {
	switch stateValue {
	case state.ChangeRequestStateOpen:
		return mergedAt == "" && closedAt == ""
	case state.ChangeRequestStateMerged:
		return mergedAt != "" && closedAt == ""
	case state.ChangeRequestStateClosed:
		return closedAt != "" && mergedAt == ""
	default:
		return false
	}
}

func validTaskChangeRequestRemove(input pluginsdk.ExactTaskChangeRequestRemove) bool {
	if !validChangeRequestEnvelope(input.RequestID, input.WorkspaceID, input.IdempotencyKey, input.ApprovalRevision, input.ManifestDigest) {
		return false
	}
	return validChangeRequestIdentity(input.ProviderID, input.RepositoryID, input.Number)
}

func validChangeRequestTimestamp(value string) bool {
	if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func exactTaskChangeRequestReportDigest(input pluginsdk.ExactTaskChangeRequestReport) (string, error) {
	canonical := struct {
		WorkspaceID  string `json:"workspace_id"`
		TaskID       string `json:"task_id"`
		ProviderID   string `json:"provider_id"`
		ProviderHost string `json:"provider_host"`
		RepositoryID string `json:"repository_id"`
		Number       int64  `json:"number"`
		URL          string `json:"url"`
		Title        string `json:"title"`
		State        string `json:"state"`
		HeadBranch   string `json:"head_branch"`
		BaseBranch   string `json:"base_branch"`
		CreatedAt    string `json:"created_at"`
		MergedAt     string `json:"merged_at"`
		ClosedAt     string `json:"closed_at"`
	}{input.WorkspaceID, input.TaskID, strings.TrimSpace(input.ProviderID), strings.TrimSpace(input.ProviderHost),
		strings.TrimSpace(input.RepositoryID), input.Number, input.URL, input.Title, input.State,
		input.HeadBranch, input.BaseBranch, input.CreatedAt, input.MergedAt, input.ClosedAt}
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 65536 {
		return "", errors.New("invalid task change request payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func exactTaskChangeRequestRemoveDigest(input pluginsdk.ExactTaskChangeRequestRemove) (string, error) {
	canonical := struct {
		WorkspaceID  string `json:"workspace_id"`
		ProviderID   string `json:"provider_id"`
		RepositoryID string `json:"repository_id"`
		Number       int64  `json:"number"`
	}{input.WorkspaceID, strings.TrimSpace(input.ProviderID), strings.TrimSpace(input.RepositoryID), input.Number}
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 65536 {
		return "", errors.New("invalid task change request payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

//nolint:cyclop // The command admits, fences, and persists one plugin-reported change request.
func (h *pluginHost) reportTaskChangeRequestExact(ctx context.Context, input pluginsdk.ExactTaskChangeRequestReport) (*pluginsdk.CommandResult, pluginsdk.TaskChangeRequest, error) {
	if !validTaskChangeRequestReport(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.TaskChangeRequest{}, nil
	}
	digest, err := exactTaskChangeRequestReportDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.TaskChangeRequest{}, nil
	}
	store, record, existingReceipt, unlock, result := h.admitTaskChangeRequest(ctx, input.WorkspaceID, input.RequestID, input.IdempotencyKey,
		exactTaskChangeRequestReportMethod, digest, input.TaskID, input.ApprovalRevision, input.ManifestDigest)
	if result != nil {
		return result, pluginsdk.TaskChangeRequest{}, nil
	}
	defer unlock()
	if existingReceipt && record.Receipt.State != exactReceiptStateCompleted {
		_, _ = store.Complete(ctx, record.Intent.OperationID, string(pluginsdk.CommandUncertain), "change_request_outcome_uncertain", record.Intent.TargetID, "")
		return uncertainTaskChangeRequest("change_request_outcome_uncertain"), pluginsdk.TaskChangeRequest{}, nil
	}
	ledger := h.changeRequestLedger()
	if ledger == nil {
		return unavailableTaskChangeRequest("task_change_requests_unsupported"), pluginsdk.TaskChangeRequest{}, nil
	}
	if !h.ownsRepositoryProvider(strings.TrimSpace(input.ProviderID)) {
		if record.Receipt.State == exactReceiptStateCompleted {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "repository_provider_not_owned"}, pluginsdk.TaskChangeRequest{}, nil
		}
		return h.completeChangeRequestFailure(ctx, store, record, pluginsdk.CommandDenied, "repository_provider_not_owned", input.TaskID, ""), pluginsdk.TaskChangeRequest{}, nil
	}
	if record.Receipt.State == exactReceiptStateCompleted {
		if record.Receipt.ResultStatus != string(pluginsdk.CommandApplied) {
			return commandResultFromRecord(record), pluginsdk.TaskChangeRequest{}, nil
		}
		candidate, err := h.findReportedChangeRequest(ctx, ledger, input)
		if err != nil {
			return unavailableTaskChangeRequest("task_change_request_receipt_unavailable"), pluginsdk.TaskChangeRequest{}, nil
		}
		if candidate != nil {
			return commandResultFromRecord(record), taskChangeRequestToSDK(*candidate), nil
		}
		// The row may have been removed after the applied receipt. The durable
		// receipt still makes the replay idempotent even when no row remains.
		return commandResultFromRecord(record), pluginsdk.TaskChangeRequest{}, nil
	}
	if err := h.validateChangeRequestTarget(ctx, input.WorkspaceID, input.TaskID); err != nil {
		resultStatus, reason := exactChangeRequestTargetError(err)
		return h.completeChangeRequestFailure(ctx, store, record, resultStatus, reason, input.TaskID, ""), pluginsdk.TaskChangeRequest{}, nil
	}
	reported, completed, err := ledger.ReportChangeRequestWithReceipt(ctx, state.ChangeRequestRecord{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID, TaskID: input.TaskID,
		ProviderID: strings.TrimSpace(input.ProviderID), ProviderHost: strings.TrimSpace(input.ProviderHost),
		RepositoryID: strings.TrimSpace(input.RepositoryID), Number: input.Number,
		URL: input.URL, Title: input.Title, State: input.State,
		HeadBranch: input.HeadBranch, BaseBranch: input.BaseBranch,
		CreatedAt: input.CreatedAt, MergedAt: input.MergedAt, ClosedAt: input.ClosedAt,
	}, store, record.Intent.OperationID, string(pluginsdk.CommandApplied))
	if errors.Is(err, state.ErrChangeRequestTaskConflict) {
		return h.completeChangeRequestFailure(ctx, store, record, pluginsdk.CommandConflict, "task_changed", input.TaskID, ""), pluginsdk.TaskChangeRequest{}, nil
	}
	if err != nil {
		return uncertainTaskChangeRequest("task_change_request_transaction_failed"), pluginsdk.TaskChangeRequest{}, nil
	}
	return commandResultFromRecord(completed), taskChangeRequestToSDK(reported), nil
}

func (h *pluginHost) findReportedChangeRequest(ctx context.Context, ledger *state.ChangeRequestStore, input pluginsdk.ExactTaskChangeRequestReport) (*state.ChangeRequestRecord, error) {
	reported, err := ledger.ListChangeRequestsByTask(ctx, h.installationID, input.WorkspaceID, input.TaskID)
	if err != nil {
		return nil, err
	}
	for index := range reported {
		candidate := &reported[index]
		if candidate.ProviderID == strings.TrimSpace(input.ProviderID) &&
			candidate.RepositoryID == strings.TrimSpace(input.RepositoryID) && candidate.Number == input.Number {
			return candidate, nil
		}
	}
	return nil, nil
}

//nolint:cyclop // The command admits, fences, and removes one plugin-reported change request.
func (h *pluginHost) removeTaskChangeRequestExact(ctx context.Context, input pluginsdk.ExactTaskChangeRequestRemove) (*pluginsdk.CommandResult, error) {
	if !validTaskChangeRequestRemove(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	digest, err := exactTaskChangeRequestRemoveDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	store, record, existingReceipt, unlock, result := h.admitTaskChangeRequest(ctx, input.WorkspaceID, input.RequestID, input.IdempotencyKey,
		exactTaskChangeRequestRemoveMethod, digest, changeRequestAdmissionVersion(""), input.ApprovalRevision, input.ManifestDigest)
	if result != nil {
		return result, nil
	}
	defer unlock()
	if existingReceipt && record.Receipt.State != exactReceiptStateCompleted {
		_, _ = store.Complete(ctx, record.Intent.OperationID, string(pluginsdk.CommandUncertain), "change_request_outcome_uncertain", record.Intent.TargetID, "")
		return uncertainTaskChangeRequest("change_request_outcome_uncertain"), nil
	}
	ledger := h.changeRequestLedger()
	if ledger == nil {
		return unavailableTaskChangeRequest("task_change_requests_unsupported"), nil
	}
	if !h.ownsRepositoryProvider(strings.TrimSpace(input.ProviderID)) {
		if record.Receipt.State == exactReceiptStateCompleted {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "repository_provider_not_owned"}, nil
		}
		return h.completeChangeRequestFailure(ctx, store, record, pluginsdk.CommandDenied, "repository_provider_not_owned", "", ""), nil
	}
	if record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(record), nil
	}
	completed, err := ledger.RemoveChangeRequestWithReceipt(ctx, h.installationID, input.WorkspaceID,
		strings.TrimSpace(input.ProviderID), strings.TrimSpace(input.RepositoryID), input.Number,
		store, record.Intent.OperationID, string(pluginsdk.CommandApplied))
	if err != nil {
		return uncertainTaskChangeRequest("task_change_request_transaction_failed"), nil
	}
	return commandResultFromRecord(completed), nil
}

func (h *pluginHost) admitTaskChangeRequest(ctx context.Context, workspaceID, requestID, idempotencyKey, method, payloadDigest, targetID string,
	approvalRevision uint64, manifestDigest string,
) (*state.CommandStore, state.CommandRecord, bool, func(), *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, state.CommandRecord{}, false, nil, unavailableTaskChangeRequest("authorization_unavailable")
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil || (h.pluginID != "" && installed.ID != h.pluginID) {
		unlock()
		return nil, state.CommandRecord{}, false, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	if installed.Status != StatusActive {
		unlock()
		return nil, state.CommandRecord{}, false, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "plugin_not_active"}
	}
	if ManifestCapabilityDigest(installed.Manifest) != manifestDigest {
		unlock()
		return nil, state.CommandRecord{}, false, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(h.installationID, workspaceID, exactTaskChangeRequestCapability, approvalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", method, "v2"))
	if !decision.Allowed {
		unlock()
		return nil, state.CommandRecord{}, false, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, state.CommandRecord{}, false, nil, unavailableTaskChangeRequest(exactReasonCommandStoreUnavailable)
	}
	changeRequests := h.changeRequestLedger()
	if changeRequests == nil {
		unlock()
		return nil, state.CommandRecord{}, false, nil, unavailableTaskChangeRequest("task_change_requests_unsupported")
	}
	record, existingReceipt, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: workspaceID,
		RequestID: requestID, IdempotencyKey: idempotencyKey,
		Method: method, CapabilityID: exactTaskChangeRequestCapability,
		PayloadDigest: payloadDigest, TargetID: targetID,
		ExpectedResourceVersion: changeRequestAdmissionVersion(targetID), ApprovalRevision: approvalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, state.CommandRecord{}, false, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, state.CommandRecord{}, false, nil, unavailableTaskChangeRequest("command_admission_unavailable")
	}
	return store, record, existingReceipt, unlock, nil
}

// changeRequestAdmissionVersion carries the intent's target through the
// command ledger's required version field. Report names its task; remove
// names no single task row, so it carries a stable sentinel instead.
func changeRequestAdmissionVersion(targetID string) string {
	if targetID == "" {
		return "change-request-remove"
	}
	return targetID
}

func (h *pluginHost) changeRequestLedger() *state.ChangeRequestStore {
	if h.changeRequestsDep != nil {
		if ledger := h.changeRequestsDep(); ledger != nil {
			return ledger
		}
	}
	if h.service != nil {
		return h.service.changeRequestStoreDep()
	}
	return nil
}

func (h *pluginHost) validateChangeRequestTarget(ctx context.Context, workspaceID, taskID string) error {
	if h.taskData == nil {
		return status.Error(codes.Unavailable, "task data is unavailable")
	}
	task, err := h.taskData.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return status.Error(codes.NotFound, "task was not found")
		}
		return status.Error(codes.Unavailable, "task state is unavailable")
	}
	if task == nil || task.ID != taskID || task.WorkspaceID != workspaceID {
		return status.Error(codes.NotFound, "task was not found")
	}
	if task.ArchivedAt != nil {
		return status.Error(codes.FailedPrecondition, "archived task cannot receive a change request")
	}
	return nil
}

func exactChangeRequestTargetError(err error) (pluginsdk.CommandStatus, string) {
	switch status.Code(err) {
	case codes.NotFound:
		return pluginsdk.CommandNotFound, "task_not_found"
	case codes.Aborted, codes.FailedPrecondition, codes.AlreadyExists:
		return pluginsdk.CommandConflict, "task_changed"
	case codes.InvalidArgument:
		return pluginsdk.CommandInvalid, "invalid_task"
	default:
		return pluginsdk.CommandUnavailable, "task_unavailable"
	}
}

func taskChangeRequestToSDK(record state.ChangeRequestRecord) pluginsdk.TaskChangeRequest {
	out := pluginsdk.TaskChangeRequest{ID: record.ID, WorkspaceID: record.WorkspaceID, TaskID: record.TaskID,
		ProviderID: record.ProviderID, ProviderHost: record.ProviderHost, RepositoryID: record.RepositoryID,
		Number: record.Number, URL: record.URL, Title: record.Title, State: record.State,
		HeadBranch: record.HeadBranch, BaseBranch: record.BaseBranch, CreatedAt: record.CreatedAt,
		ResourceVersion: state.ChangeRequestResourceVersion(record)}
	if record.MergedAt != "" {
		mergedAt := record.MergedAt
		out.MergedAt = &mergedAt
	}
	if record.ClosedAt != "" {
		closedAt := record.ClosedAt
		out.ClosedAt = &closedAt
	}
	return out
}

func unavailableTaskChangeRequest(reason string) *pluginsdk.CommandResult {
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: reason}
}

func uncertainTaskChangeRequest(reason string) *pluginsdk.CommandResult {
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandUncertain, Reason: reason}
}

func (h *pluginHost) completeChangeRequestFailure(ctx context.Context, store *state.CommandStore, record state.CommandRecord,
	commandStatus pluginsdk.CommandStatus, reason, targetID, resourceVersion string,
) *pluginsdk.CommandResult {
	if commandStatus == pluginsdk.CommandUnavailable {
		return unavailableTaskChangeRequest(reason)
	}
	completed, err := store.Complete(ctx, record.Intent.OperationID, string(commandStatus), reason, targetID, resourceVersion)
	if err != nil {
		return uncertainTaskChangeRequest("command_receipt_unavailable")
	}
	return commandResultFromRecord(completed)
}
