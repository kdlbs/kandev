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

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/plugins/state"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	exactTaskDirectiveIssueMethod       = "IssueTaskDirectiveExact"
	exactTaskDirectiveResolveMethod     = "ResolveTaskDirectiveExact"
	exactTaskDirectiveCapability        = "host.v2.write:task_directives"
	maxTaskDirectiveLifetime            = 7 * 24 * time.Hour
	exactDirectiveStateResolved         = "resolved"
	exactDirectiveStatePending          = "pending"
	exactDirectiveStateGenerationFenced = "generation_fenced"
)

func (m exactTaskCommandManager) IssueDirective(ctx context.Context, input pluginsdk.ExactTaskDirectiveIssue) (*pluginsdk.CommandResult, pluginsdk.TaskDirective, error) {
	return m.host.issueTaskDirectiveExact(ctx, input)
}

func (m exactTaskCommandManager) ResolveDirective(ctx context.Context, input pluginsdk.ExactTaskDirectiveResolve) (*pluginsdk.CommandResult, pluginsdk.TaskDirective, error) {
	return m.host.resolveTaskDirectiveExact(ctx, input)
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validManifestDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validDirectiveResourceVersion(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func directiveResourceVersionMatches(observed time.Time, expected string) bool {
	if observed.IsZero() {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, expected)
	return err == nil && observed.Equal(parsed)
}

func validTaskDirectiveIssue(input pluginsdk.ExactTaskDirectiveIssue) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.TaskID) || !isBoundedApprovalIdentifier(input.SessionID) ||
		!isBoundedApprovalIdentifier(input.IdempotencyKey) || input.ApprovalRevision == 0 ||
		!validManifestDigest(input.ManifestDigest) || !validDigest(input.InstructionDigest) ||
		!validDirectiveResourceVersion(input.ExpectedTaskResourceVersion) || !validDirectiveResourceVersion(input.ExpectedSessionResourceVersion) {
		return false
	}
	if !isExactHostV2Capability(input.CapabilityClass) || !isDelegableDirectiveCapability(input.CapabilityClass) {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, input.ExpiresAt)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	return expiresAt.After(now) && expiresAt.Sub(now) <= maxTaskDirectiveLifetime
}

func validTaskDirectiveResolve(input pluginsdk.ExactTaskDirectiveResolve) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.DirectiveID) || !isBoundedApprovalIdentifier(input.IdempotencyKey) ||
		input.ApprovalRevision == 0 || !validManifestDigest(input.ManifestDigest) ||
		!strings.HasPrefix(input.ExpectedResourceVersion, "sha256:") || !validDigest(input.ExpectedResourceVersion) ||
		!validDigest(input.ResolutionDigest) {
		return false
	}
	switch input.Resolution {
	case exactReceiptStateCompleted, "rejected", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func isDelegableDirectiveCapability(capability string) bool {
	if capability == exactTaskDirectiveCapability || isHumanReservedCapability(capability) {
		return false
	}
	for _, method := range exactHostMethods {
		if strings.HasPrefix(method.capability, "host.v2.write:") && method.capability == capability {
			return true
		}
	}
	return false
}

func exactTaskDirectiveIssueDigest(input pluginsdk.ExactTaskDirectiveIssue) (string, error) {
	canonical := struct {
		WorkspaceID    string `json:"workspace_id"`
		TaskID         string `json:"task_id"`
		SessionID      string `json:"session_id"`
		Capability     string `json:"capability_class"`
		Instruction    string `json:"instruction_digest"`
		TaskVersion    string `json:"task_version"`
		SessionVersion string `json:"session_version"`
		ExpiresAt      string `json:"expires_at"`
	}{input.WorkspaceID, input.TaskID, input.SessionID, input.CapabilityClass, input.InstructionDigest,
		input.ExpectedTaskResourceVersion, input.ExpectedSessionResourceVersion, input.ExpiresAt}
	return directivePayloadDigest(canonical)
}

func exactTaskDirectiveResolveDigest(input pluginsdk.ExactTaskDirectiveResolve) (string, error) {
	canonical := struct {
		WorkspaceID      string `json:"workspace_id"`
		DirectiveID      string `json:"directive_id"`
		ResourceVersion  string `json:"resource_version"`
		Resolution       string `json:"resolution"`
		ResolutionDigest string `json:"resolution_digest"`
	}{input.WorkspaceID, input.DirectiveID, input.ExpectedResourceVersion, input.Resolution, input.ResolutionDigest}
	return directivePayloadDigest(canonical)
}

func directivePayloadDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 65536 {
		return "", fmt.Errorf("invalid task directive payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

//nolint:cyclop // The command admits, persists, and returns one short-lived directive atomically.
func (h *pluginHost) issueTaskDirectiveExact(ctx context.Context, input pluginsdk.ExactTaskDirectiveIssue) (*pluginsdk.CommandResult, pluginsdk.TaskDirective, error) {
	if !validTaskDirectiveIssue(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.TaskDirective{}, nil
	}
	digest, err := exactTaskDirectiveIssueDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.TaskDirective{}, nil
	}
	store, record, unlock, result := h.admitTaskDirective(ctx, input.WorkspaceID, input.RequestID, input.IdempotencyKey,
		exactTaskDirectiveIssueMethod, exactTaskDirectiveCapability, input.ApprovalRevision, input.ManifestDigest,
		digest, input.TaskID, input.ExpectedTaskResourceVersion+"/"+input.ExpectedSessionResourceVersion)
	if result != nil {
		return result, pluginsdk.TaskDirective{}, nil
	}
	defer unlock()
	if record.Receipt.State == exactReceiptStateCompleted {
		directive, err := store.GetTaskDirective(ctx, h.installationID, input.WorkspaceID, record.Receipt.TargetID)
		if err != nil {
			return unavailableTaskDirective("task_directive_receipt_unavailable"), pluginsdk.TaskDirective{}, nil
		}
		return commandResultFromRecord(record), taskDirectiveToSDK(directive), nil
	}
	if directive, readErr := store.GetTaskDirective(ctx, h.installationID, input.WorkspaceID, record.Intent.OperationID); readErr == nil {
		directive, readErr = h.refreshTaskDirective(ctx, directive, input.ApprovalRevision)
		if readErr != nil {
			return unavailableTaskDirective("task_directive_validation_unavailable"), taskDirectiveToSDK(directive), nil
		}
		completed, completeErr := store.Complete(ctx, record.Intent.OperationID, string(pluginsdk.CommandAlreadyApplied), "", directive.ID, directive.ResourceVersion())
		if completeErr != nil {
			return unavailableTaskDirective("command_receipt_unavailable"), taskDirectiveToSDK(directive), nil
		}
		return commandResultFromRecord(completed), taskDirectiveToSDK(directive), nil
	} else if !errors.Is(readErr, state.ErrTaskDirectiveNotFound) {
		return unavailableTaskDirective("task_directive_store_unavailable"), pluginsdk.TaskDirective{}, nil
	}
	if !h.taskDirectiveCapabilityAllowed(input.WorkspaceID, input.ApprovalRevision, digest, input.CapabilityClass) {
		return h.completeDirectiveFailure(ctx, store, record, pluginsdk.CommandDenied, "delegated_capability_unavailable", input.TaskID, ""), pluginsdk.TaskDirective{}, nil
	}
	if err := h.validateTaskDirectiveTarget(ctx, input.WorkspaceID, input.TaskID, input.SessionID,
		input.ExpectedTaskResourceVersion, input.ExpectedSessionResourceVersion); err != nil {
		resultStatus, reason := exactDirectiveTargetError(err)
		return h.completeDirectiveFailure(ctx, store, record, resultStatus, reason, input.TaskID, ""), pluginsdk.TaskDirective{}, nil
	}
	pending, pendingErr := h.pendingDirectiveTransition(ctx, input.TaskID)
	if pendingErr != nil {
		return unavailableTaskDirective("task_transition_data_unavailable"), pluginsdk.TaskDirective{}, nil
	}
	if pending {
		return h.completeDirectiveFailure(ctx, store, record, pluginsdk.CommandConflict, "pending_workflow_transition", input.TaskID, ""), pluginsdk.TaskDirective{}, nil
	}
	operationID := record.Intent.OperationID
	directive, alreadyApplied, err := store.IssueTaskDirective(ctx, state.TaskDirectiveRecord{
		ID: operationID, OperationID: operationID, InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		TaskID: input.TaskID, SessionID: input.SessionID, IdempotencyKey: input.IdempotencyKey, PayloadDigest: digest,
		CapabilityClass: input.CapabilityClass, InstructionDigest: input.InstructionDigest,
		ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion, ExpectedSessionResourceVersion: input.ExpectedSessionResourceVersion,
		ApprovalRevision: input.ApprovalRevision, ExpiresAt: input.ExpiresAt, AuditID: uuid.NewString(),
	})
	if errors.Is(err, state.ErrTaskDirectivePayloadConflict) {
		return h.completeDirectiveFailure(ctx, store, record, pluginsdk.CommandConflict, "idempotency_payload_mismatch", input.TaskID, ""), pluginsdk.TaskDirective{}, nil
	}
	if err != nil {
		return unavailableTaskDirective("task_directive_store_unavailable"), pluginsdk.TaskDirective{}, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	completed, err := store.Complete(ctx, operationID, string(resultStatus), "", directive.ID, directive.ResourceVersion())
	if err != nil {
		return unavailableTaskDirective("command_receipt_unavailable"), taskDirectiveToSDK(directive), nil
	}
	return commandResultFromRecord(completed), taskDirectiveToSDK(directive), nil
}

//nolint:cyclop // The command validates directive state and persists its exact resolution.
func (h *pluginHost) resolveTaskDirectiveExact(ctx context.Context, input pluginsdk.ExactTaskDirectiveResolve) (*pluginsdk.CommandResult, pluginsdk.TaskDirective, error) {
	if !validTaskDirectiveResolve(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.TaskDirective{}, nil
	}
	digest, err := exactTaskDirectiveResolveDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.TaskDirective{}, nil
	}
	store, command, unlock, result := h.admitTaskDirective(ctx, input.WorkspaceID, input.RequestID, input.IdempotencyKey,
		exactTaskDirectiveResolveMethod, exactTaskDirectiveCapability, input.ApprovalRevision, input.ManifestDigest,
		digest, input.DirectiveID, input.ExpectedResourceVersion)
	if result != nil {
		return result, pluginsdk.TaskDirective{}, nil
	}
	defer unlock()
	if command.Receipt.State == exactReceiptStateCompleted {
		directive, err := store.GetTaskDirective(ctx, h.installationID, input.WorkspaceID, input.DirectiveID)
		if err != nil {
			return unavailableTaskDirective("task_directive_receipt_unavailable"), pluginsdk.TaskDirective{}, nil
		}
		return commandResultFromRecord(command), taskDirectiveToSDK(directive), nil
	}
	directive, err := store.GetTaskDirective(ctx, h.installationID, input.WorkspaceID, input.DirectiveID)
	if errors.Is(err, state.ErrTaskDirectiveNotFound) {
		return h.completeDirectiveFailure(ctx, store, command, pluginsdk.CommandNotFound, "task_directive_not_found", input.DirectiveID, ""), pluginsdk.TaskDirective{}, nil
	}
	if err != nil {
		return unavailableTaskDirective("task_directive_store_unavailable"), pluginsdk.TaskDirective{}, nil
	}
	if directive.State == exactDirectiveStateResolved && directive.ResolutionOperationID == command.Intent.OperationID {
		resolved, _, resolveErr := store.ResolveTaskDirective(ctx, h.installationID, input.WorkspaceID, input.DirectiveID,
			input.ExpectedResourceVersion, command.Intent.OperationID, input.Resolution, input.ResolutionDigest)
		if resolveErr != nil {
			return unavailableTaskDirective("task_directive_receipt_unavailable"), taskDirectiveToSDK(directive), nil
		}
		completed, completeErr := store.Complete(ctx, command.Intent.OperationID, string(pluginsdk.CommandAlreadyApplied), "", input.DirectiveID, resolved.ResourceVersion())
		if completeErr != nil {
			return unavailableTaskDirective("command_receipt_unavailable"), taskDirectiveToSDK(resolved), nil
		}
		return commandResultFromRecord(completed), taskDirectiveToSDK(resolved), nil
	}
	directive, err = h.refreshTaskDirective(ctx, directive, input.ApprovalRevision)
	if err != nil {
		return unavailableTaskDirective("task_directive_validation_unavailable"), taskDirectiveToSDK(directive), nil
	}
	if directive.ResourceVersion() != input.ExpectedResourceVersion {
		return h.completeDirectiveFailure(ctx, store, command, pluginsdk.CommandConflict, "resource_version_changed", input.DirectiveID, directive.ResourceVersion()), taskDirectiveToSDK(directive), nil
	}
	resolved, alreadyApplied, err := store.ResolveTaskDirective(ctx, h.installationID, input.WorkspaceID, input.DirectiveID,
		input.ExpectedResourceVersion, command.Intent.OperationID, input.Resolution, input.ResolutionDigest)
	if errors.Is(err, state.ErrTaskDirectiveConflict) {
		current, readErr := store.GetTaskDirective(ctx, h.installationID, input.WorkspaceID, input.DirectiveID)
		if readErr != nil {
			return unavailableTaskDirective("task_directive_store_unavailable"), pluginsdk.TaskDirective{}, nil
		}
		return h.completeDirectiveFailure(ctx, store, command, pluginsdk.CommandConflict, "task_directive_not_pending", input.DirectiveID, current.ResourceVersion()), taskDirectiveToSDK(current), nil
	}
	if err != nil {
		return unavailableTaskDirective("task_directive_store_unavailable"), pluginsdk.TaskDirective{}, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	completed, err := store.Complete(ctx, command.Intent.OperationID, string(resultStatus), "", input.DirectiveID, resolved.ResourceVersion())
	if err != nil {
		return unavailableTaskDirective("command_receipt_unavailable"), taskDirectiveToSDK(resolved), nil
	}
	return commandResultFromRecord(completed), taskDirectiveToSDK(resolved), nil
}

func (h *pluginHost) admitTaskDirective(ctx context.Context, workspaceID, requestID, idempotencyKey, method, capability string,
	approvalRevision uint64, manifestDigest, payloadDigest, targetID, expectedVersion string,
) (*state.CommandStore, state.CommandRecord, func(), *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, state.CommandRecord{}, nil, unavailableTaskDirective("authorization_unavailable")
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil || (h.pluginID != "" && installed.ID != h.pluginID) {
		unlock()
		return nil, state.CommandRecord{}, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	if ManifestCapabilityDigest(installed.Manifest) != manifestDigest {
		unlock()
		return nil, state.CommandRecord{}, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(h.installationID, workspaceID, capability, approvalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", method, "v2"))
	if !decision.Allowed {
		unlock()
		return nil, state.CommandRecord{}, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, state.CommandRecord{}, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "task_directives_unsupported"}
	}
	record, _, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: workspaceID, RequestID: requestID,
		IdempotencyKey: idempotencyKey, Method: method, CapabilityID: capability, PayloadDigest: payloadDigest,
		TargetID: targetID, ExpectedResourceVersion: expectedVersion, ApprovalRevision: approvalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, state.CommandRecord{}, nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, state.CommandRecord{}, nil, unavailableTaskDirective("command_admission_unavailable")
	}
	return store, record, unlock, nil
}

func (h *pluginHost) taskDirectiveCapabilityAllowed(workspaceID string, revision uint64, requestDigest, capability string) bool {
	if !isDelegableDirectiveCapability(capability) {
		return false
	}
	decision := h.service.authorizePluginCapability(h.installationID, workspaceID, capability, revision,
		CanonicalApprovalDigest("task-directive-capability", requestDigest, capability),
		CanonicalApprovalDigest("task-directive-worker-capability", capability, "v2"))
	return decision.Allowed
}

//nolint:cyclop // The target check binds task and optional session versions to one directive.
func (h *pluginHost) validateTaskDirectiveTarget(ctx context.Context, workspaceID, taskID, sessionID, taskVersion, sessionVersion string) error {
	if h.taskData == nil {
		return status.Error(codes.Unavailable, "task data is unavailable")
	}
	task, err := h.taskData.GetTask(ctx, taskID)
	if err != nil {
		return mapExactDirectiveReadError(err)
	}
	if task == nil || task.ID != taskID || task.WorkspaceID != workspaceID {
		return status.Error(codes.NotFound, "task was not found")
	}
	if task.ArchivedAt != nil {
		return status.Error(codes.FailedPrecondition, "archived task cannot receive a directive")
	}
	if !directiveResourceVersionMatches(task.UpdatedAt, taskVersion) {
		return status.Error(codes.Aborted, "task resource version changed")
	}
	sessions, err := h.taskData.ListTaskSessions(ctx, taskID)
	if err != nil {
		return status.Error(codes.Unavailable, "task sessions are unavailable")
	}
	for _, session := range sessions {
		if session == nil || session.ID != sessionID || session.TaskID != taskID {
			continue
		}
		if session.State == taskmodels.TaskSessionStateCompleted || session.State == taskmodels.TaskSessionStateFailed || session.State == taskmodels.TaskSessionStateCancelled {
			return status.Error(codes.FailedPrecondition, "task session is terminal")
		}
		if !directiveResourceVersionMatches(session.UpdatedAt, sessionVersion) {
			return status.Error(codes.Aborted, "task session resource version changed")
		}
		return nil
	}
	return status.Error(codes.NotFound, "task session was not found")
}

func mapExactDirectiveReadError(err error) error {
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return status.Error(codes.NotFound, "task was not found")
	}
	return status.Error(codes.Unavailable, "task state is unavailable")
}

func exactDirectiveTargetError(err error) (pluginsdk.CommandStatus, string) {
	switch status.Code(err) {
	case codes.NotFound:
		return pluginsdk.CommandNotFound, "task_or_session_not_found"
	case codes.Aborted, codes.FailedPrecondition, codes.AlreadyExists:
		return pluginsdk.CommandConflict, "task_or_session_changed"
	case codes.InvalidArgument:
		return pluginsdk.CommandInvalid, "invalid_task_or_session"
	default:
		return pluginsdk.CommandUnavailable, "task_or_session_unavailable"
	}
}

func (h *pluginHost) pendingDirectiveTransition(ctx context.Context, taskID string) (bool, error) {
	if h.pendingTaskTransitions == nil {
		return false, status.Error(codes.Unavailable, "pending task transition service is unavailable")
	}
	source := h.pendingTaskTransitions()
	if source == nil {
		return false, status.Error(codes.Unavailable, "pending task transition service is unavailable")
	}
	transitions, err := source.ListPendingTaskTransitions(ctx)
	if err != nil {
		return false, status.Error(codes.Unavailable, "pending task transitions are unavailable")
	}
	for _, transition := range transitions {
		if transition.TaskID == taskID {
			return true, nil
		}
	}
	return false, nil
}

//nolint:nestif // Refresh revokes stale directives before returning any current target data.
func (h *pluginHost) refreshTaskDirective(ctx context.Context, record state.TaskDirectiveRecord, currentApprovalRevision uint64) (state.TaskDirectiveRecord, error) {
	if record.State != exactDirectiveStatePending {
		return record, nil
	}
	terminal := ""
	if record.ApprovalRevision != currentApprovalRevision || !h.taskDirectiveCapabilityAllowed(record.WorkspaceID, currentApprovalRevision,
		CanonicalApprovalDigest("directive-refresh", record.ID, record.ResourceVersion()), record.CapabilityClass) {
		terminal = "revoked"
	} else if expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt); err != nil {
		return record, status.Error(codes.Internal, "task directive expiry is invalid")
	} else if !time.Now().UTC().Before(expiresAt) {
		terminal = "expired"
	} else if err := h.validateTaskDirectiveTarget(ctx, record.WorkspaceID, record.TaskID, record.SessionID,
		record.ExpectedTaskResourceVersion, record.ExpectedSessionResourceVersion); err != nil {
		if status.Code(err) == codes.Unavailable {
			return record, err
		}
		terminal = exactDirectiveStateGenerationFenced
	} else if pending, err := h.pendingDirectiveTransition(ctx, record.TaskID); err != nil {
		return record, err
	} else if pending {
		terminal = exactDirectiveStateGenerationFenced
	}
	if terminal == "" {
		return record, nil
	}
	store := h.commandStoreForDirective()
	if store == nil {
		return record, status.Error(codes.Unavailable, "task directive store is unavailable")
	}
	return store.MarkTaskDirectiveState(ctx, h.installationID, record.WorkspaceID, record.ID, record.ResourceVersion(), terminal)
}

func (h *pluginHost) commandStoreForDirective() *state.CommandStore {
	if h.commandStore != nil {
		return h.commandStore
	}
	if h.service != nil {
		return h.service.exactCommandStoreDep()
	}
	return nil
}

func taskDirectiveToSDK(record state.TaskDirectiveRecord) pluginsdk.TaskDirective {
	expiresAt := record.ExpiresAt
	return pluginsdk.TaskDirective{ID: record.ID, WorkspaceID: record.WorkspaceID, TaskID: record.TaskID,
		CapabilityClass: record.CapabilityClass, State: record.State, InstructionDigest: record.InstructionDigest,
		ExpiresAt: &expiresAt, ResourceVersion: record.ResourceVersion()}
}

func unavailableTaskDirective(reason string) *pluginsdk.CommandResult {
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: reason}
}

func (h *pluginHost) completeDirectiveFailure(ctx context.Context, store *state.CommandStore, record state.CommandRecord,
	commandStatus pluginsdk.CommandStatus, reason, targetID, resourceVersion string,
) *pluginsdk.CommandResult {
	if commandStatus == pluginsdk.CommandUnavailable {
		return unavailableTaskDirective(reason)
	}
	completed, err := store.Complete(ctx, record.Intent.OperationID, string(commandStatus), reason, targetID, resourceVersion)
	if err != nil {
		return unavailableTaskDirective("command_receipt_unavailable")
	}
	return commandResultFromRecord(completed)
}
