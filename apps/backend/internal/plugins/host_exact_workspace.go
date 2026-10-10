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
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	exactWorkspaceReasonResourceVersionChanged = "resource_version_changed"
	exactWorkspaceAdminCapability              = "host.v2.write:workflows"
	exactWorkflowMoveToStepAction              = "move_to_step"
)

type WorkspaceAdminWriter interface {
	ApplyWorkspaceAdministrationExact(context.Context, ExactWorkspaceAdminInput) (ExactWorkspaceAdminResult, error)
}

type ExactWorkspaceAdminInput struct {
	Command       pluginsdk.WorkspaceAdminCommand
	OperationKind pluginsdk.WorkspaceAdminOperationKind
	OperationID   string
	PayloadDigest string
	TargetID      string
}

type ExactWorkspaceAdminResult struct {
	TargetID        string
	ResourceVersion string
	AlreadyApplied  bool
}

type exactWorkspaceAdministrationManager struct{ host *pluginHost }

func (h *pluginHost) WorkspaceAdministration() pluginsdk.ExactWorkspaceAdministrationManager {
	return exactWorkspaceAdministrationManager{host: h}
}

func (m exactWorkspaceAdministrationManager) Apply(ctx context.Context, command pluginsdk.WorkspaceAdminCommand) (*pluginsdk.CommandResult, error) {
	return m.host.applyWorkspaceAdministrationExact(ctx, command)
}

func (h *pluginHost) applyWorkspaceAdministrationExact(ctx context.Context, command pluginsdk.WorkspaceAdminCommand) (*pluginsdk.CommandResult, error) {
	kind, capabilityID, targetID, expectedVersion, ok := exactWorkspaceAdminCommandDetails(command)
	if !ok || !validExactWorkspaceAdminCommand(command, kind) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	digest, err := exactWorkspaceAdminDigest(command)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	admission, result := h.admitExactWorkspaceAdmin(ctx, command, kind, capabilityID, targetID, expectedVersion, digest)
	if result != nil {
		return result, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted {
		return replayedExactWorkspaceAdminCommand(admission.record), nil
	}
	changed, err := admission.writer.ApplyWorkspaceAdministrationExact(ctx, ExactWorkspaceAdminInput{
		Command: command, OperationKind: kind, OperationID: admission.record.Intent.OperationID,
		PayloadDigest: digest, TargetID: admission.record.Intent.TargetID,
	})
	if err != nil {
		status := exactWorkspaceAdminErrorStatus(err)
		reason := string(status)
		if errors.Is(err, repoerrors.ErrTaskVersionConflict) {
			reason = exactWorkspaceReasonResourceVersionChanged
		} else if strings.Contains(strings.ToLower(err.Error()), "synchronized") {
			reason = "synchronized_workflow_read_only"
		}
		completed, completeErr := admission.store.Complete(ctx, admission.record.Intent.OperationID, string(status), reason, admission.record.Intent.TargetID, "")
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
		}
		return commandResultFromRecord(completed), nil
	}
	status := pluginsdk.CommandApplied
	if changed.AlreadyApplied {
		status = pluginsdk.CommandAlreadyApplied
	}
	if changed.TargetID == "" {
		changed.TargetID = admission.record.Intent.TargetID
	}
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(status), "", changed.TargetID, changed.ResourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
	}
	return commandResultFromRecord(completed), nil
}

func replayedExactWorkspaceAdminCommand(record state.CommandRecord) *pluginsdk.CommandResult {
	result := commandResultFromRecord(record)
	if result.Status == pluginsdk.CommandApplied {
		result.Status = pluginsdk.CommandAlreadyApplied
		if result.Receipt != nil {
			result.Receipt.Status = pluginsdk.CommandAlreadyApplied
		}
	}
	return result
}

type exactWorkspaceAdminAdmission struct {
	writer WorkspaceAdminWriter
	store  *state.CommandStore
	record state.CommandRecord
	unlock func()
}

func (h *pluginHost) admitExactWorkspaceAdmin(
	ctx context.Context,
	command pluginsdk.WorkspaceAdminCommand,
	kind pluginsdk.WorkspaceAdminOperationKind,
	capabilityID, targetID, expectedVersion, digest string,
) (*exactWorkspaceAdminAdmission, *pluginsdk.CommandResult) {
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
	if command.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, command.WorkspaceID, capabilityID, command.ApprovalRevision,
		CanonicalApprovalDigest("capability-context", command.WorkspaceID, string(kind)),
		CanonicalApprovalDigest("host-method", string(kind)),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	if h.workspaceAdminWriter == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "workspace_administration_unavailable"}
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
		InstallationID: h.installationID, WorkspaceID: command.WorkspaceID, RequestID: command.RequestID,
		IdempotencyKey: command.IdempotencyKey, Method: string(kind), CapabilityID: capabilityID,
		PayloadDigest: digest, TargetID: targetID, ExpectedResourceVersion: expectedVersion,
		ApprovalRevision: command.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactWorkspaceAdminAdmission{writer: h.workspaceAdminWriter, store: store, record: record, unlock: unlock}, nil
}

func exactWorkspaceAdminCommandDetails(command pluginsdk.WorkspaceAdminCommand) (pluginsdk.WorkspaceAdminOperationKind, string, string, string, bool) {
	if workspaceAdminOperationCount(command) != 1 {
		return "", "", "", "", false
	}
	switch {
	case command.UpdateDefaults != nil:
		return pluginsdk.WorkspaceAdminUpdateDefaults, "host.v2.write:workspaces", command.WorkspaceID, command.UpdateDefaults.ExpectedResourceVersion, true
	case command.CreateWorkflow != nil:
		return pluginsdk.WorkspaceAdminCreateWorkflow, exactWorkspaceAdminCapability, workspaceAdminStableID("workflow", command), command.CreateWorkflow.ExpectedWorkspaceResourceVersion, true
	case command.UpdateWorkflow != nil:
		return pluginsdk.WorkspaceAdminUpdateWorkflow, exactWorkspaceAdminCapability, command.UpdateWorkflow.WorkflowID, command.UpdateWorkflow.ExpectedResourceVersion, true
	case command.ReorderWorkflows != nil:
		return pluginsdk.WorkspaceAdminReorderWorkflows, exactWorkspaceAdminCapability, command.WorkspaceID, command.ReorderWorkflows.ExpectedWorkspaceResourceVersion, true
	case command.CreateStep != nil:
		return pluginsdk.WorkspaceAdminCreateWorkflowStep, exactWorkspaceAdminCapability, workspaceAdminStableID("workflow-step", command), command.CreateStep.ExpectedWorkflowResourceVersion, true
	case command.UpdateStep != nil:
		return pluginsdk.WorkspaceAdminUpdateWorkflowStep, exactWorkspaceAdminCapability, command.UpdateStep.StepID, command.UpdateStep.ExpectedWorkflowResourceVersion + "/" + command.UpdateStep.ExpectedStepResourceVersion, true
	case command.ReorderSteps != nil:
		return pluginsdk.WorkspaceAdminReorderWorkflowStep, exactWorkspaceAdminCapability, command.ReorderSteps.WorkflowID, command.ReorderSteps.ExpectedWorkflowResourceVersion, true
	case command.RegisterRepo != nil:
		return pluginsdk.WorkspaceAdminRegisterRepository, "host.v2.write:repositories", workspaceAdminStableID("repository", command), command.RegisterRepo.ExpectedWorkspaceResourceVersion, true
	case command.UpdateRepo != nil:
		return pluginsdk.WorkspaceAdminUpdateRepository, "host.v2.write:repositories", command.UpdateRepo.RepositoryID, command.UpdateRepo.ExpectedResourceVersion, true
	default:
		return "", "", "", "", false
	}
}

func workspaceAdminOperationCount(command pluginsdk.WorkspaceAdminCommand) int {
	count := 0
	for _, present := range []bool{
		command.UpdateDefaults != nil, command.CreateWorkflow != nil, command.UpdateWorkflow != nil,
		command.ReorderWorkflows != nil, command.CreateStep != nil, command.UpdateStep != nil,
		command.ReorderSteps != nil, command.RegisterRepo != nil, command.UpdateRepo != nil,
	} {
		if present {
			count++
		}
	}
	return count
}

//nolint:cyclop // The operation switch validates the selected command against its full payload contract.
func validExactWorkspaceAdminCommand(command pluginsdk.WorkspaceAdminCommand, kind pluginsdk.WorkspaceAdminOperationKind) bool {
	if !isBoundedApprovalIdentifier(command.RequestID) || !isBoundedApprovalIdentifier(command.WorkspaceID) ||
		!isBoundedApprovalIdentifier(command.IdempotencyKey) || command.ApprovalRevision == 0 || len(command.ManifestDigest) != 64 {
		return false
	}
	switch kind {
	case pluginsdk.WorkspaceAdminUpdateDefaults:
		return validAdminDefaultsCommand(command)
	case pluginsdk.WorkspaceAdminCreateWorkflow:
		return validAdminCreateWorkflowCommand(command)
	case pluginsdk.WorkspaceAdminUpdateWorkflow:
		return validAdminUpdateWorkflowCommand(command)
	case pluginsdk.WorkspaceAdminReorderWorkflows:
		return validAdminReorderWorkflowsCommand(command)
	case pluginsdk.WorkspaceAdminCreateWorkflowStep:
		return validAdminCreateStepCommand(command)
	case pluginsdk.WorkspaceAdminUpdateWorkflowStep:
		return validAdminUpdateStepCommand(command)
	case pluginsdk.WorkspaceAdminReorderWorkflowStep:
		return validAdminReorderStepsCommand(command)
	case pluginsdk.WorkspaceAdminRegisterRepository:
		return validAdminRegisterRepositoryCommand(command)
	case pluginsdk.WorkspaceAdminUpdateRepository:
		return validAdminUpdateRepositoryCommand(command)
	default:
		return false
	}
}

func validAdminDefaultsCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.UpdateDefaults
	return value != nil && validAdminResourceVersion(value.ExpectedResourceVersion) &&
		(value.Name != nil || value.Description != nil || value.DefaultExecutorID != nil || value.DefaultEnvironmentID != nil || value.DefaultAgentProfileID != nil || value.DefaultConfigAgentProfile != nil)
}

func validAdminCreateWorkflowCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.CreateWorkflow
	return value != nil && validAdminResourceVersion(value.ExpectedWorkspaceResourceVersion) &&
		validAdminText(value.Name, 256, true) && len(value.Description) <= 4096 && len(value.Prompt) <= 65536
}

func validAdminUpdateWorkflowCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.UpdateWorkflow
	return value != nil && isBoundedApprovalIdentifier(value.WorkflowID) && validAdminResourceVersion(value.ExpectedResourceVersion) &&
		(value.Name != nil || value.Description != nil || value.Prompt != nil || value.AgentProfileID != nil) &&
		(value.Name == nil || validAdminText(*value.Name, 256, true)) && (value.Description == nil || len(*value.Description) <= 4096) &&
		(value.Prompt == nil || len(*value.Prompt) <= 65536)
}

func validAdminReorderWorkflowsCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.ReorderWorkflows
	return value != nil && validAdminResourceVersion(value.ExpectedWorkspaceResourceVersion) &&
		validAdminOrder(value.WorkflowIDs, value.WorkflowResourceVersions, validAdminResourceVersion)
}

func validAdminCreateStepCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.CreateStep
	return value != nil && isBoundedApprovalIdentifier(value.WorkflowID) && validAdminResourceVersion(value.ExpectedWorkflowResourceVersion) &&
		(value.Configuration.ID == "" || isBoundedApprovalIdentifier(value.Configuration.ID)) &&
		validAdminStepConfiguration(value.Configuration, value.WorkflowID)
}

func validAdminUpdateStepCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.UpdateStep
	return value != nil && isBoundedApprovalIdentifier(value.WorkflowID) && isBoundedApprovalIdentifier(value.StepID) &&
		validAdminResourceVersion(value.ExpectedWorkflowResourceVersion) && validAdminResourceVersion(value.ExpectedStepResourceVersion) &&
		value.Configuration.ID == value.StepID && validAdminStepConfiguration(value.Configuration, value.WorkflowID)
}

func validAdminReorderStepsCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.ReorderSteps
	return value != nil && isBoundedApprovalIdentifier(value.WorkflowID) && validAdminResourceVersion(value.ExpectedWorkflowResourceVersion) &&
		validAdminOrder(value.StepIDs, value.StepResourceVersions, validAdminResourceVersion)
}

func validAdminRegisterRepositoryCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.RegisterRepo
	return value != nil && validAdminResourceVersion(value.ExpectedWorkspaceResourceVersion) && validAdminText(value.Name, 256, true) &&
		len(value.LocalPath) <= 4096 && len(value.RemoteURL) <= 2048 && len(value.ProviderHost) <= 512 &&
		len(value.ProviderRepoID) <= 512 && len(value.ProviderScope) <= 512 && len(value.DefaultBranch) <= 256 &&
		len(value.WorktreeBranchPrefix) <= 128 && len(value.WorktreeBranchTemplate) <= 512
}

//nolint:cyclop // Each optional repository field has an explicit bound before admission.
func validAdminUpdateRepositoryCommand(command pluginsdk.WorkspaceAdminCommand) bool {
	value := command.UpdateRepo
	return value != nil && isBoundedApprovalIdentifier(value.RepositoryID) && validAdminResourceVersion(value.ExpectedResourceVersion) &&
		(value.Name != nil || value.DefaultBranch != nil || value.WorktreeBranchPrefix != nil || value.WorktreeBranchTemplate != nil || value.PullBeforeWorktree != nil) &&
		(value.Name == nil || validAdminText(*value.Name, 256, true)) && (value.DefaultBranch == nil || len(*value.DefaultBranch) <= 256) &&
		(value.WorktreeBranchPrefix == nil || len(*value.WorktreeBranchPrefix) <= 128) &&
		(value.WorktreeBranchTemplate == nil || len(*value.WorktreeBranchTemplate) <= 512)
}

func validAdminResourceVersion(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validAdminText(value string, maxLength int, requireNonEmpty bool) bool {
	trimmed := strings.TrimSpace(value)
	return len(value) <= maxLength && trimmed == value && (!requireNonEmpty || trimmed != "")
}

func validAdminOrder(ids, versions []string, validVersion func(string) bool) bool {
	if len(ids) == 0 || len(ids) > 200 || len(ids) != len(versions) {
		return false
	}
	seen := make(map[string]struct{}, len(ids))
	for i, id := range ids {
		if !isBoundedApprovalIdentifier(id) || !validVersion(versions[i]) {
			return false
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validAdminStepConfiguration(value pluginsdk.WorkflowStepConfiguration, workflowID string) bool {
	if value.WorkflowID != workflowID || !validAdminText(value.Name, 256, true) || len(value.Prompt) > 65536 ||
		value.Position < 0 || value.WIPLimit < 0 || value.AutoArchiveAfterHours < 0 ||
		len(value.OnEnterActionTypes) > 16 || len(value.OnTurnStartActions) > 16 || len(value.OnTurnCompleteActions) > 16 {
		return false
	}
	return validAdminOnEnterActions(value.OnEnterActionTypes) &&
		validAdminTransitions(value.OnTurnStartActions, false) && validAdminTransitions(value.OnTurnCompleteActions, true)
}

func validAdminOnEnterActions(actions []string) bool {
	allowed := map[string]bool{"enable_plan_mode": true, "auto_start_agent": true, "reset_agent_context": true, "clear_decisions": true}
	for _, action := range actions {
		if !allowed[action] {
			return false
		}
	}
	return true
}

func validAdminTransitions(actions []pluginsdk.WorkflowTransitionAction, allowDisablePlanMode bool) bool {
	for _, action := range actions {
		if !validAdminTransitionAction(action, allowDisablePlanMode) {
			return false
		}
	}
	return true
}

func validAdminTransitionAction(action pluginsdk.WorkflowTransitionAction, allowDisablePlanMode bool) bool {
	validType := action.Type == "move_to_next" || action.Type == "move_to_previous" || action.Type == exactWorkflowMoveToStepAction
	if allowDisablePlanMode && action.Type == "disable_plan_mode" {
		validType = true
	}
	if !validType {
		return false
	}
	if action.Type == exactWorkflowMoveToStepAction {
		return isBoundedApprovalIdentifier(action.TargetStepID)
	}
	return action.TargetStepID == ""
}

func exactWorkspaceAdminDigest(command pluginsdk.WorkspaceAdminCommand) (string, error) {
	encoded, err := json.Marshal(command)
	if err != nil || len(encoded) > 256*1024 {
		return "", fmt.Errorf("invalid workspace administration payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func workspaceAdminStableID(kind string, command pluginsdk.WorkspaceAdminCommand) string {
	name := fmt.Sprintf("kandev-plugin-workspace-admin:%s:%s:%s:%s", command.WorkspaceID, command.IdempotencyKey, kind, command.ManifestDigest)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(name)).String()
}

func exactWorkspaceAdminErrorStatus(err error) pluginsdk.CommandStatus {
	if errors.Is(err, repoerrors.ErrTaskVersionConflict) || strings.Contains(strings.ToLower(err.Error()), "conflict") {
		return pluginsdk.CommandConflict
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return pluginsdk.CommandUnavailable
	}
	return pluginsdk.CommandInvalid
}

func isExactWorkspaceAdminMethod(method string) bool {
	switch method {
	case string(pluginsdk.WorkspaceAdminUpdateDefaults), string(pluginsdk.WorkspaceAdminCreateWorkflow), string(pluginsdk.WorkspaceAdminUpdateWorkflow),
		string(pluginsdk.WorkspaceAdminReorderWorkflows), string(pluginsdk.WorkspaceAdminCreateWorkflowStep), string(pluginsdk.WorkspaceAdminUpdateWorkflowStep),
		string(pluginsdk.WorkspaceAdminReorderWorkflowStep), string(pluginsdk.WorkspaceAdminRegisterRepository), string(pluginsdk.WorkspaceAdminUpdateRepository):
		return true
	default:
		return false
	}
}
