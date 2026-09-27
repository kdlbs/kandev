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
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactExecutionAdmission struct {
	store  *state.CommandStore
	record state.CommandRecord
	replay bool
	unlock func()
}

type exactExecutionManager struct{ host *pluginHost }

func (h *pluginHost) ExecutionCommands() pluginsdk.ExactExecutionCommandManager {
	return exactExecutionManager{host: h}
}

func (h *pluginHost) ExactInteractionCommands() pluginsdk.ExactInteractionCommandManager {
	return exactInteractionManager{host: h}
}

func (m exactExecutionManager) EnsureTaskRun(ctx context.Context, in pluginsdk.ExactTaskRunCommand) (*pluginsdk.CommandResult, pluginsdk.ExactTaskRunResult, error) {
	if !validTaskRunCommand(in) || !validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) {
		return exactInvalidResult(), pluginsdk.ExactTaskRunResult{}, nil
	}
	digest := exactCommandDigest(exactEnsureTaskRunMethod, in)
	admission, result := m.host.admitExecutionCommand(ctx, exactEnsureTaskRunMethod, "host.v2.write:execution", in.RequestID, in.WorkspaceID, in.TaskID, in.IdempotencyKey, in.ExpectedTaskResourceVersion, in.ApprovalRevision, in.ManifestDigest, digest)
	if result != nil {
		return result, pluginsdk.ExactTaskRunResult{}, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		return replay, replayedTaskRunResult(admission.record.Receipt, replay), nil
	}
	controller := m.host.executionController()
	if controller == nil {
		return exactUnavailableResult("execution_control_unavailable"), pluginsdk.ExactTaskRunResult{}, nil
	}
	run, err := controller.EnsureTaskRun(ctx, m.host.installationID, in)
	if err != nil {
		return finishExactExecutionError(ctx, admission, err, in.TaskID, in.ExpectedTaskResourceVersion)
	}
	result, err = completeExecutionRunCommand(ctx, admission, run)
	run.Result = result
	return result, run, err
}

func (m exactExecutionManager) StopTaskRun(ctx context.Context, in pluginsdk.ExactTaskExecutionCommand) (*pluginsdk.CommandResult, bool, error) {
	if !validTaskExecutionCommand(in, true) || !validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) {
		return exactInvalidResult(), false, nil
	}
	digest := exactCommandDigest(exactStopTaskRunMethod, in)
	admission, result := m.host.admitExecutionCommand(ctx, exactStopTaskRunMethod, "host.v2.write:execution", in.RequestID, in.WorkspaceID, in.SessionID, in.IdempotencyKey, in.ExpectedSessionResourceVersion, in.ApprovalRevision, in.ManifestDigest, digest)
	if result != nil {
		return result, false, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		return replay, replay.Status == pluginsdk.CommandAlreadyApplied, nil
	}
	controller := m.host.executionController()
	if controller == nil {
		return exactUnavailableResult("execution_control_unavailable"), false, nil
	}
	stopped, err := controller.StopTaskRun(ctx, m.host.installationID, in)
	if err != nil {
		result, _, finishErr := finishExactExecutionError(ctx, admission, err, in.SessionID, in.ExpectedSessionResourceVersion)
		return result, false, finishErr
	}
	statusValue := pluginsdk.CommandNoChange
	if stopped {
		statusValue = pluginsdk.CommandApplied
	}
	result, err = completeExecutionCommand(ctx, admission, statusValue, "", in.SessionID, in.ExpectedSessionResourceVersion)
	return result, stopped, err
}

func (m exactExecutionManager) RecoverSession(ctx context.Context, in pluginsdk.ExactSessionRecoveryCommand) (*pluginsdk.CommandResult, pluginsdk.ExactTaskRunResult, error) {
	if !validTaskExecutionCommand(in.ExactTaskExecutionCommand, true) || !validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) || in.Action != "resume" {
		return exactInvalidResult(), pluginsdk.ExactTaskRunResult{}, nil
	}
	digest := exactCommandDigest(exactRecoverSessionMethod, in)
	base := in.ExactTaskExecutionCommand
	admission, result := m.host.admitExecutionCommand(ctx, exactRecoverSessionMethod, "host.v2.write:execution", base.RequestID, base.WorkspaceID, base.SessionID, base.IdempotencyKey, base.ExpectedSessionResourceVersion, base.ApprovalRevision, base.ManifestDigest, digest)
	if result != nil {
		return result, pluginsdk.ExactTaskRunResult{}, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		return replay, replayedTaskRunResult(admission.record.Receipt, replay), nil
	}
	controller := m.host.executionController()
	if controller == nil {
		return exactUnavailableResult("execution_control_unavailable"), pluginsdk.ExactTaskRunResult{}, nil
	}
	run, err := controller.RecoverSession(ctx, m.host.installationID, in)
	if err != nil {
		return finishExactExecutionError(ctx, admission, err, base.SessionID, base.ExpectedSessionResourceVersion)
	}
	result, err = completeExecutionRunCommand(ctx, admission, run)
	run.Result = result
	return result, run, err
}

func (m exactExecutionManager) CancelPendingTaskTransition(ctx context.Context, in pluginsdk.ExactPendingTransitionCommand) (*pluginsdk.CommandResult, error) {
	if !validPendingTransitionCommand(in) || !validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) {
		return exactInvalidResult(), nil
	}
	digest := exactCommandDigest(exactCancelTaskTransitionMethod, in)
	admission, result := m.host.admitExecutionCommand(ctx, exactCancelTaskTransitionMethod, "host.v2.write:execution", in.RequestID, in.WorkspaceID, in.TransitionID, in.IdempotencyKey, in.ExpectedResourceVersion, in.ApprovalRevision, in.ManifestDigest, digest)
	if result != nil {
		return result, nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		return replay, nil
	}
	controller := m.host.executionController()
	if controller == nil {
		return exactUnavailableResult("execution_control_unavailable"), nil
	}
	removed, err := controller.CancelPendingTaskTransition(ctx, m.host.installationID, in)
	if err != nil {
		result, _, finishErr := finishExactExecutionError(ctx, admission, err, in.TransitionID, in.ExpectedResourceVersion)
		return result, finishErr
	}
	statusValue := pluginsdk.CommandNoChange
	if removed {
		statusValue = pluginsdk.CommandApplied
	}
	result, err = completeExecutionCommand(ctx, admission, statusValue, "", in.TransitionID, in.ExpectedResourceVersion)
	return result, err
}

func (m exactExecutionManager) GetSessionModeContext(ctx context.Context, in pluginsdk.ExactTaskExecutionCommand) (pluginsdk.SessionModeContext, error) {
	if !validTaskExecutionCommand(in, false) || !validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) {
		return pluginsdk.SessionModeContext{}, status.Error(codes.InvalidArgument, "invalid exact session mode query")
	}
	_, unlock, err := m.host.exactReadAdmission(ctx, in.RequestID, in.WorkspaceID, exactGetSessionModeMethod, "host.v2.read:sessions", map[string]string{"task_id": in.TaskID, "session_id": in.SessionID})
	if err != nil {
		return pluginsdk.SessionModeContext{}, err
	}
	defer unlock()
	controller := m.host.executionController()
	if controller == nil {
		return pluginsdk.SessionModeContext{}, status.Error(codes.Unimplemented, "session mode provider is unavailable")
	}
	value, err := controller.GetSessionModeContext(ctx, m.host.installationID, in)
	if err != nil {
		return pluginsdk.SessionModeContext{}, err
	}
	value.AvailableModes = safeSessionModes(value.AvailableModes)
	return value, nil
}

func (m exactExecutionManager) SetSessionMode(ctx context.Context, in pluginsdk.ExactSessionModeCommand) (*pluginsdk.CommandResult, string, error) {
	base := in.ExactTaskExecutionCommand
	if !validTaskExecutionCommand(base, true) || !validManagementClaimFence(base.ManagementInstanceKey, base.ExpectedClaimGeneration) || !isSafeSessionModeID(in.ModeID) {
		return exactInvalidResult(), "", nil
	}
	digest := exactCommandDigest(exactSetSessionModeMethod, in)
	admission, result := m.host.admitExecutionCommand(ctx, exactSetSessionModeMethod, "host.v2.write:execution", base.RequestID, base.WorkspaceID, base.SessionID, base.IdempotencyKey, base.ExpectedSessionResourceVersion, base.ApprovalRevision, base.ManifestDigest, digest)
	if result != nil {
		return result, "", nil
	}
	defer admission.unlock()
	if replay, done := replayedExactCommand(admission); done {
		if replay.Receipt == nil {
			return replay, "", nil
		}
		return replay, replay.Receipt.ResourceVersion, nil
	}
	controller := m.host.executionController()
	if controller == nil {
		return exactUnavailableResult("execution_control_unavailable"), "", nil
	}
	current, err := controller.SetSessionMode(ctx, m.host.installationID, in)
	if err != nil {
		result, _, finishErr := finishExactExecutionError(ctx, admission, err, base.SessionID, base.ExpectedSessionResourceVersion)
		return result, "", finishErr
	}
	result, err = completeExecutionCommand(ctx, admission, pluginsdk.CommandApplied, "", base.SessionID, current)
	return result, current, err
}

func (h *pluginHost) admitExecutionCommand(ctx context.Context, method, capability, requestID, workspaceID, targetID, idempotencyKey, expectedVersion string, revision uint64, manifestDigest, payloadDigest string) (*exactExecutionAdmission, *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, exactUnavailableResult("authorization_unavailable")
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	digest := ManifestCapabilityDigest(installed.Manifest)
	if manifestDigest != digest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(h.installationID, workspaceID, capability, revision, payloadDigest, CanonicalApprovalDigest("host-method", method, "v2"))
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	commandStore := h.commandStore
	if commandStore == nil {
		commandStore = h.service.exactCommandStoreDep()
	}
	if commandStore == nil {
		unlock()
		return nil, exactUnavailableResult("command_store_unavailable")
	}
	record, replay, err := commandStore.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: workspaceID, RequestID: requestID,
		IdempotencyKey: idempotencyKey, Method: method, CapabilityID: capability,
		PayloadDigest: payloadDigest, TargetID: targetID, ExpectedResourceVersion: expectedVersion,
		ApprovalRevision: revision, ManifestDigest: digest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, exactUnavailableResult("command_admission_unavailable")
	}
	return &exactExecutionAdmission{store: commandStore, record: record, replay: replay, unlock: unlock}, nil
}

func replayedExactCommand(admission *exactExecutionAdmission) (*pluginsdk.CommandResult, bool) {
	if !admission.replay {
		return nil, false
	}
	if admission.record.Receipt.State != exactReceiptStateCompleted {
		return exactUnavailableResult("operation_outcome_uncertain"), true
	}
	result := commandResultFromRecord(admission.record)
	if result.Status == pluginsdk.CommandApplied {
		result.Status = pluginsdk.CommandAlreadyApplied
		if result.Receipt != nil {
			result.Receipt.Status = pluginsdk.CommandAlreadyApplied
		}
	}
	return result, true
}

func replayedTaskRunResult(receipt state.CommandReceipt, result *pluginsdk.CommandResult) pluginsdk.ExactTaskRunResult {
	if result == nil || receipt.State != exactReceiptStateCompleted || result.Status != pluginsdk.CommandAlreadyApplied {
		return pluginsdk.ExactTaskRunResult{}
	}
	var replay struct {
		SessionID    string `json:"session_id"`
		SessionState string `json:"session_state"`
		ExecutionID  string `json:"execution_id"`
	}
	if receipt.ResultData != "" && json.Unmarshal([]byte(receipt.ResultData), &replay) == nil {
		return pluginsdk.ExactTaskRunResult{
			Result: result, SessionID: replay.SessionID,
			SessionState: replay.SessionState, ExecutionID: replay.ExecutionID,
		}
	}
	return pluginsdk.ExactTaskRunResult{
		Result: result, SessionID: receipt.TargetID,
		SessionState: receipt.ResourceVersion,
	}
}

func completeExecutionCommand(ctx context.Context, admission *exactExecutionAdmission, statusValue pluginsdk.CommandStatus, reason, target, version string) (*pluginsdk.CommandResult, error) {
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID, string(statusValue), reason, target, version)
	if err != nil {
		return exactUnavailableResult("command_receipt_unavailable"), nil
	}
	return commandResultFromRecord(completed), nil
}

func completeExecutionRunCommand(ctx context.Context, admission *exactExecutionAdmission, run pluginsdk.ExactTaskRunResult) (*pluginsdk.CommandResult, error) {
	data, err := json.Marshal(struct {
		SessionID    string `json:"session_id"`
		SessionState string `json:"session_state"`
		ExecutionID  string `json:"execution_id"`
	}{SessionID: run.SessionID, SessionState: run.SessionState, ExecutionID: run.ExecutionID})
	if err != nil {
		return exactUnavailableResult("command_receipt_unavailable"), nil
	}
	completed, err := admission.store.CompleteWithResultData(ctx, admission.record.Intent.OperationID,
		string(pluginsdk.CommandApplied), "", run.SessionID, run.SessionState, string(data))
	if err != nil {
		return exactUnavailableResult("command_receipt_unavailable"), nil
	}
	return commandResultFromRecord(completed), nil
}

func finishExactExecutionError(ctx context.Context, admission *exactExecutionAdmission, err error, target, version string) (*pluginsdk.CommandResult, pluginsdk.ExactTaskRunResult, error) {
	result := exactExecutionErrorResult(err)
	if result.Status == pluginsdk.CommandUnavailable {
		return result, pluginsdk.ExactTaskRunResult{}, nil
	}
	completed, completeErr := completeExecutionCommand(ctx, admission, result.Status, result.Reason, target, version)
	return completed, pluginsdk.ExactTaskRunResult{}, completeErr
}

func exactExecutionErrorResult(err error) *pluginsdk.CommandResult {
	switch status.Code(err) {
	case codes.NotFound:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandNotFound, Reason: "target_not_found"}
	case codes.FailedPrecondition, codes.Aborted:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "stale_resource_or_execution"}
	case codes.PermissionDenied:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "native_policy_denied"}
	case codes.InvalidArgument:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}
	case codes.Unimplemented:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "operation_unsupported"}
	default:
		return exactUnavailableResult("execution_control_unavailable")
	}
}

func exactInvalidResult() *pluginsdk.CommandResult {
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}
}
func exactUnavailableResult(reason string) *pluginsdk.CommandResult {
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: reason}
}

func validTaskRunCommand(in pluginsdk.ExactTaskRunCommand) bool {
	return isBoundedApprovalIdentifier(in.RequestID) && isBoundedApprovalIdentifier(in.WorkspaceID) &&
		isBoundedApprovalIdentifier(in.TaskID) && isBoundedApprovalIdentifier(in.IdempotencyKey) &&
		isRFC3339ResourceVersion(in.ExpectedTaskResourceVersion) && validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) &&
		in.ApprovalRevision > 0 && len(in.ManifestDigest) == 64
}

func validTaskExecutionCommand(in pluginsdk.ExactTaskExecutionCommand, requireWrite bool) bool {
	if !isBoundedApprovalIdentifier(in.RequestID) || !isBoundedApprovalIdentifier(in.WorkspaceID) ||
		!isBoundedApprovalIdentifier(in.TaskID) || !isBoundedApprovalIdentifier(in.SessionID) ||
		!validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) ||
		in.ApprovalRevision == 0 || len(in.ManifestDigest) != 64 {
		return false
	}
	if requireWrite && (!isBoundedApprovalIdentifier(in.IdempotencyKey) || !isBoundedApprovalIdentifier(in.ExpectedExecutionID) || !isRFC3339ResourceVersion(in.ExpectedSessionResourceVersion)) {
		return false
	}
	return true
}

func validPendingTransitionCommand(in pluginsdk.ExactPendingTransitionCommand) bool {
	return isBoundedApprovalIdentifier(in.RequestID) && isBoundedApprovalIdentifier(in.WorkspaceID) &&
		isBoundedApprovalIdentifier(in.TaskID) && isBoundedApprovalIdentifier(in.TransitionID) &&
		isBoundedApprovalIdentifier(in.IdempotencyKey) && in.ExpectedResourceVersion != "" &&
		validManagementClaimFence(in.ManagementInstanceKey, in.ExpectedClaimGeneration) &&
		in.ApprovalRevision > 0 && len(in.ManifestDigest) == 64
}

func isRFC3339ResourceVersion(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func exactCommandDigest(method string, value any) string {
	encoded, _ := json.Marshal(struct {
		Method  string `json:"method"`
		Payload any    `json:"payload"`
	}{method, value})
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func safeSessionModes(in []pluginsdk.SessionModeOption) []pluginsdk.SessionModeOption {
	out := make([]pluginsdk.SessionModeOption, 0, len(in))
	for _, mode := range in {
		if isSafeSessionMode(mode) {
			out = append(out, mode)
		}
	}
	return out
}

func isSafeSessionModeID(modeID string) bool {
	return modeID != "" && isSafeSessionModeValue(modeID)
}

func isSafeSessionMode(mode pluginsdk.SessionModeOption) bool {
	return isSafeSessionModeID(mode.ID) && isSafeSessionModeValue(mode.Name) && isSafeSessionModeValue(mode.Description)
}

func isSafeSessionModeValue(value string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", " ", "", ".", "", "/", "").Replace(strings.ToLower(value))
	for _, forbidden := range []string{
		"bypass", "dangerouslyskip", "dontask", "yolo", "credential", "autoapprove",
		"skippermission", "nopermission", "noapproval", "acceptall", "fullauto", "unrestricted", "allowall",
	} {
		if strings.Contains(normalized, forbidden) {
			return false
		}
	}
	return true
}
