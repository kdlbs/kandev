package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const managedAutomationCapabilityRead = "host.v2.read:automations"
const managedAutomationCapabilityWrite = "host.v2.write:automations"

type pluginHostManagedConversationScheduleManager struct{ host *pluginHost }

type managedAutomationScheduleAdmission struct {
	service ManagedConversationScheduleService
	store   *state.CommandStore
	record  state.CommandRecord
	unlock  func()
}

func (h *pluginHost) ManagedConversationSchedules() pluginsdk.ManagedConversationScheduleManager {
	return &pluginHostManagedConversationScheduleManager{host: h}
}

func (m *pluginHostManagedConversationScheduleManager) List(ctx context.Context, query pluginsdk.ManagedConversationScheduleQuery) ([]pluginsdk.ManagedConversationSchedule, error) {
	if !validManagedScheduleWorkspace(query.WorkspaceID) {
		return nil, managedScheduleInvalid("workspace_id is required")
	}
	digest, err := managedConversationDigest(query)
	if err != nil {
		return nil, managedScheduleInvalid("invalid schedule query")
	}
	call, result := m.host.beginManagedAutomationSchedule(ctx, query.WorkspaceID, managedAutomationCapabilityRead,
		"ListManagedConversationSchedulesExact", query.ApprovalRevision, query.ManifestDigest, digest, false)
	if result != nil {
		return nil, managedScheduleGRPCError(result)
	}
	defer call.unlock()
	return call.service.ListManagedConversationSchedules(ctx, m.host.installationID, query.WorkspaceID)
}

func (m *pluginHostManagedConversationScheduleManager) Create(ctx context.Context, input pluginsdk.ManagedConversationScheduleCreate) (*pluginsdk.CommandResult, pluginsdk.ManagedConversationSchedule, error) {
	if !validManagedScheduleWrite(input.RequestID, input.IdempotencyKey, input.Schedule) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	digest, err := managedConversationDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	workspaceID := input.Schedule.WorkspaceID
	targetID := managedScheduleStableID(m.host.installationID, workspaceID, input.RequestID)
	call, result := m.host.admitManagedAutomationSchedule(ctx, workspaceID, "CreateManagedConversationScheduleExact",
		input.RequestID, input.IdempotencyKey, input.ApprovalRevision, input.ManifestDigest, digest, targetID, "0")
	if result != nil {
		return result, pluginsdk.ManagedConversationSchedule{}, nil
	}
	defer call.unlock()
	input.Schedule.ID = call.record.Intent.TargetID
	if call.record.Receipt.State == exactReceiptStateCompleted {
		schedule, found, readErr := findManagedSchedule(ctx, call.service, m.host.installationID, workspaceID, input.Schedule.ID)
		if readErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "schedule_readback_unavailable"}, pluginsdk.ManagedConversationSchedule{}, nil
		}
		if !found {
			return commandResultFromRecord(call.record), pluginsdk.ManagedConversationSchedule{}, nil
		}
		return replayedExactManagedSchedule(call.record), schedule, nil
	}
	ctx = withManagedScheduleApprovalLock(ctx, m.host.installationID)
	schedule, alreadyApplied, err := call.service.CreateManagedConversationSchedule(ctx, m.host.installationID, m.host.pluginID,
		input.Schedule, call.record.Intent.OperationID, digest)
	if err != nil {
		return m.finishManagedScheduleError(ctx, call, err)
	}
	status := pluginsdk.CommandApplied
	if alreadyApplied {
		status = pluginsdk.CommandAlreadyApplied
	}
	completed, completeErr := call.store.Complete(ctx, call.record.Intent.OperationID, string(status), "",
		call.record.Intent.TargetID, strconv.FormatUint(schedule.ResourceRevision, 10))
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, schedule, nil
	}
	return commandResultFromRecord(completed), schedule, nil
}

func (m *pluginHostManagedConversationScheduleManager) Update(ctx context.Context, input pluginsdk.ManagedConversationScheduleUpdate) (*pluginsdk.CommandResult, pluginsdk.ManagedConversationSchedule, error) {
	if !validManagedScheduleWrite(input.RequestID, input.IdempotencyKey, input.Schedule) ||
		!isBoundedApprovalIdentifier(input.AutomationID) || input.ExpectedResourceRevision == 0 ||
		input.Schedule.WorkspaceID == "" {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	digest, err := managedConversationDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	call, result := m.host.admitManagedAutomationSchedule(ctx, input.Schedule.WorkspaceID, "UpdateManagedConversationScheduleExact",
		input.RequestID, input.IdempotencyKey, input.ApprovalRevision, input.ManifestDigest, digest,
		input.AutomationID, strconv.FormatUint(input.ExpectedResourceRevision, 10))
	if result != nil {
		return result, pluginsdk.ManagedConversationSchedule{}, nil
	}
	defer call.unlock()
	if call.record.Receipt.State == exactReceiptStateCompleted {
		schedule, found, readErr := findManagedSchedule(ctx, call.service, m.host.installationID, input.Schedule.WorkspaceID, input.AutomationID)
		if readErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "schedule_readback_unavailable"}, pluginsdk.ManagedConversationSchedule{}, nil
		}
		if !found {
			return commandResultFromRecord(call.record), pluginsdk.ManagedConversationSchedule{}, nil
		}
		return replayedExactManagedSchedule(call.record), schedule, nil
	}
	ctx = withManagedScheduleApprovalLock(ctx, m.host.installationID)
	schedule, alreadyApplied, err := call.service.UpdateManagedConversationSchedule(ctx, m.host.installationID, m.host.pluginID,
		input.Schedule.WorkspaceID, input.AutomationID, input.ExpectedResourceRevision, input.Schedule,
		call.record.Intent.OperationID, digest)
	if err != nil {
		return m.finishManagedScheduleError(ctx, call, err)
	}
	status := pluginsdk.CommandApplied
	if alreadyApplied {
		status = pluginsdk.CommandAlreadyApplied
	}
	completed, completeErr := call.store.Complete(ctx, call.record.Intent.OperationID, string(status), "",
		input.AutomationID, strconv.FormatUint(schedule.ResourceRevision, 10))
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, schedule, nil
	}
	return commandResultFromRecord(completed), schedule, nil
}

func (m *pluginHostManagedConversationScheduleManager) SetEnabled(ctx context.Context, input pluginsdk.ManagedConversationScheduleEnabled) (*pluginsdk.CommandResult, pluginsdk.ManagedConversationSchedule, error) {
	if !validManagedScheduleMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.AutomationID, input.ExpectedResourceRevision) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	digest, err := managedConversationDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	call, result := m.host.admitManagedAutomationSchedule(ctx, input.WorkspaceID, "SetManagedConversationScheduleEnabledExact",
		input.RequestID, input.IdempotencyKey, input.ApprovalRevision, input.ManifestDigest, digest,
		input.AutomationID, strconv.FormatUint(input.ExpectedResourceRevision, 10))
	if result != nil {
		return result, pluginsdk.ManagedConversationSchedule{}, nil
	}
	defer call.unlock()
	if call.record.Receipt.State == exactReceiptStateCompleted {
		schedule, found, readErr := findManagedSchedule(ctx, call.service, m.host.installationID, input.WorkspaceID, input.AutomationID)
		if readErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "schedule_readback_unavailable"}, pluginsdk.ManagedConversationSchedule{}, nil
		}
		if !found {
			return commandResultFromRecord(call.record), pluginsdk.ManagedConversationSchedule{}, nil
		}
		return replayedExactManagedSchedule(call.record), schedule, nil
	}
	schedule, alreadyApplied, err := call.service.SetManagedConversationScheduleEnabled(ctx, m.host.installationID,
		input.WorkspaceID, input.AutomationID, input.ExpectedResourceRevision, input.Enabled,
		call.record.Intent.OperationID, digest)
	if err != nil {
		return m.finishManagedScheduleError(ctx, call, err)
	}
	status := pluginsdk.CommandApplied
	if alreadyApplied {
		status = pluginsdk.CommandAlreadyApplied
	}
	completed, completeErr := call.store.Complete(ctx, call.record.Intent.OperationID, string(status), "",
		input.AutomationID, strconv.FormatUint(schedule.ResourceRevision, 10))
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, schedule, nil
	}
	return commandResultFromRecord(completed), schedule, nil
}

func (m *pluginHostManagedConversationScheduleManager) Delete(ctx context.Context, input pluginsdk.ManagedConversationScheduleDelete) (*pluginsdk.CommandResult, error) {
	if !validManagedScheduleMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.AutomationID, input.ExpectedResourceRevision) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	digest, err := managedConversationDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	call, result := m.host.admitManagedAutomationSchedule(ctx, input.WorkspaceID, "DeleteManagedConversationScheduleExact",
		input.RequestID, input.IdempotencyKey, input.ApprovalRevision, input.ManifestDigest, digest,
		input.AutomationID, strconv.FormatUint(input.ExpectedResourceRevision, 10))
	if result != nil {
		return result, nil
	}
	defer call.unlock()
	if call.record.Receipt.State == exactReceiptStateCompleted {
		return replayedExactManagedSchedule(call.record), nil
	}
	_, err = call.service.DeleteManagedConversationSchedule(ctx, m.host.installationID, input.WorkspaceID,
		input.AutomationID, input.ExpectedResourceRevision, call.record.Intent.OperationID, digest)
	if err != nil {
		result, _, finishErr := m.finishManagedScheduleError(ctx, call, err)
		return result, finishErr
	}
	completed, completeErr := call.store.Complete(ctx, call.record.Intent.OperationID, string(pluginsdk.CommandApplied), "",
		input.AutomationID, "")
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
	}
	return commandResultFromRecord(completed), nil
}

func (h *pluginHost) beginManagedAutomationSchedule(
	ctx context.Context, workspaceID, capabilityID, method string, approvalRevision uint64,
	manifestDigest, requestDigest string, needsCommandStore bool,
) (*managedAutomationScheduleAdmission, *pluginsdk.CommandResult) {
	if h == nil || h.service == nil || h.installationID == "" || h.managedAutomationSchedules == nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_automation_schedule_service_unavailable"}
	}
	service := h.managedAutomationSchedules()
	if service == nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_automation_schedule_service_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil || installed.Status != StatusActive {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	currentDigest := ManifestCapabilityDigest(installed.Manifest)
	if manifestDigest != currentDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(h.installationID, workspaceID, capabilityID, approvalRevision,
		requestDigest, CanonicalApprovalDigest("host-method", method, "v2"))
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	var commandStore *state.CommandStore
	if needsCommandStore {
		commandStore = h.commandStore
		if commandStore == nil {
			commandStore = h.service.exactCommandStoreDep()
		}
		if commandStore == nil {
			unlock()
			return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
		}
	}
	return &managedAutomationScheduleAdmission{service: service, store: commandStore, unlock: unlock}, nil
}

func (h *pluginHost) admitManagedAutomationSchedule(
	ctx context.Context, workspaceID, method, requestID, idempotencyKey string,
	approvalRevision uint64, manifestDigest, digest, targetID, expectedRevision string,
) (*managedAutomationScheduleAdmission, *pluginsdk.CommandResult) {
	call, result := h.beginManagedAutomationSchedule(ctx, workspaceID, managedAutomationCapabilityWrite, method,
		approvalRevision, manifestDigest, digest, true)
	if result != nil {
		return nil, result
	}
	command, _, err := call.store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: workspaceID, RequestID: requestID,
		IdempotencyKey: idempotencyKey, Method: method, CapabilityID: managedAutomationCapabilityWrite,
		PayloadDigest: digest, TargetID: targetID, ExpectedResourceVersion: expectedRevision,
		ApprovalRevision: approvalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		call.unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		call.unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	call.record = command
	return call, nil
}

func (m *pluginHostManagedConversationScheduleManager) finishManagedScheduleError(
	ctx context.Context, call *managedAutomationScheduleAdmission, err error,
) (*pluginsdk.CommandResult, pluginsdk.ManagedConversationSchedule, error) {
	commandStatus := pluginsdk.CommandUnavailable
	reason := "schedule_operation_unavailable"
	switch {
	case errors.Is(err, ErrManagedScheduleNotFound):
		commandStatus, reason = pluginsdk.CommandNotFound, "schedule_not_found"
	case errors.Is(err, ErrManagedScheduleRevisionConflict):
		commandStatus, reason = pluginsdk.CommandConflict, "resource_version_changed"
	case errors.Is(err, ErrManagedScheduleInvalid):
		commandStatus, reason = pluginsdk.CommandInvalid, "invalid_schedule"
	default:
		switch status.Code(err) {
		case codes.NotFound:
			commandStatus, reason = pluginsdk.CommandNotFound, "schedule_or_destination_not_found"
		case codes.Aborted, codes.FailedPrecondition, codes.AlreadyExists:
			commandStatus, reason = pluginsdk.CommandConflict, "resource_version_changed"
		case codes.InvalidArgument:
			commandStatus, reason = pluginsdk.CommandInvalid, "invalid_schedule"
		case codes.PermissionDenied, codes.Unauthenticated:
			commandStatus, reason = pluginsdk.CommandDenied, "destination_not_authorized"
		case codes.ResourceExhausted:
			commandStatus, reason = pluginsdk.CommandRateLimited, "schedule_rate_limited"
		}
	}
	completed, completeErr := call.store.Complete(ctx, call.record.Intent.OperationID, string(commandStatus), reason,
		call.record.Intent.TargetID, "")
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, pluginsdk.ManagedConversationSchedule{}, nil
	}
	return commandResultFromRecord(completed), pluginsdk.ManagedConversationSchedule{}, nil
}

func replayedExactManagedSchedule(record state.CommandRecord) *pluginsdk.CommandResult {
	result := commandResultFromRecord(record)
	if result.Status == pluginsdk.CommandApplied {
		result.Status = pluginsdk.CommandAlreadyApplied
		if result.Receipt != nil {
			result.Receipt.Status = pluginsdk.CommandAlreadyApplied
		}
	}
	return result
}

func findManagedSchedule(ctx context.Context, service ManagedConversationScheduleService, installationID, workspaceID, scheduleID string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	schedules, err := service.ListManagedConversationSchedules(ctx, installationID, workspaceID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	for _, schedule := range schedules {
		if schedule.ID == scheduleID {
			return schedule, true, nil
		}
	}
	return pluginsdk.ManagedConversationSchedule{}, false, nil
}

func validManagedScheduleWorkspace(workspaceID string) bool {
	return isBoundedApprovalIdentifier(workspaceID)
}

func validManagedInstanceKey(instanceKey string) bool {
	return instanceKey != "" && len(instanceKey) <= 128 && strings.TrimSpace(instanceKey) == instanceKey &&
		!strings.ContainsRune(instanceKey, '\x00')
}

//nolint:cyclop // Validate the complete durable schedule identity and payload before write admission.
func validManagedScheduleWrite(requestID, idempotencyKey string, schedule pluginsdk.ManagedConversationSchedule) bool {
	if !isBoundedApprovalIdentifier(requestID) || !isBoundedApprovalIdentifier(idempotencyKey) ||
		!validManagedScheduleWorkspace(schedule.WorkspaceID) || len(schedule.Name) == 0 || len(schedule.Name) > 256 ||
		len(schedule.Description) > 4096 || len(schedule.Prompt) > managedInputBytesMax ||
		!validManagedInstanceKey(schedule.InstanceKey) || !isBoundedApprovalIdentifier(schedule.PluginID) ||
		schedule.DestinationRevision == 0 || len(schedule.Triggers) == 0 || len(schedule.Triggers) > 32 {
		return false
	}
	for _, trigger := range schedule.Triggers {
		if !isBoundedApprovalIdentifier(trigger.Type) || len(trigger.Config) == 0 || len(trigger.Config) > managedInputBytesMax || !json.Valid(trigger.Config) {
			return false
		}
	}
	return true
}

func validManagedScheduleMutation(requestID, idempotencyKey, workspaceID, automationID string, revision uint64) bool {
	return isBoundedApprovalIdentifier(requestID) && isBoundedApprovalIdentifier(idempotencyKey) &&
		validManagedScheduleWorkspace(workspaceID) && isBoundedApprovalIdentifier(automationID) && revision > 0
}

func managedScheduleStableID(installationID, workspaceID, requestID string) string {
	return CanonicalApprovalDigest("managed-conversation-schedule", installationID, workspaceID, requestID)
}

func managedScheduleInvalid(message string) error {
	return fmt.Errorf("managed schedule: %s", strings.TrimSpace(message))
}

func managedScheduleGRPCError(result *pluginsdk.CommandResult) error {
	return status.Error(managedCommandGRPCCode(result.Status), result.Reason)
}
