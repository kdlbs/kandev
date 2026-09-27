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
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactMethod struct {
	method      string
	capability  string
	description string
}

const (
	exactReceiptStateCompleted                       = "completed"
	exactReasonCommandStoreUnavailable               = "command_store_unavailable"
	exactReasonTaskTransitionDataUnavailable         = "task_transition_data_unavailable"
	exactReasonManagedConversationServiceUnavailable = "managed_conversation_service_unavailable"
)

const exactTaskUpdateMethod = "UpdateTaskExact"
const exactTaskLabelsMethod = "SetTaskLabelsExact"
const exactTaskAssignmentMethod = "AssignTaskExact"
const exactTaskMoveMethod = "MoveTaskExact"
const exactTaskArchiveMethod = "ArchiveTaskExact"
const exactTaskCompletionCriteriaMethod = "SetTaskCompletionCriteriaExact"
const exactTaskCompletionEvidenceMethod = "VerifyTaskCompletionCriterionExact"
const exactTaskManagementClaimAcquireMethod = "AcquireTaskManagementClaimExact"
const exactTaskManagementClaimReleaseMethod = "ReleaseTaskManagementClaimExact"
const exactTaskManagementClaimTransferMethod = "TransferTaskManagementClaimExact"
const exactWorkspaceDefaultsMethod = "UpdateWorkspaceDefaultsExact"
const exactWorkflowCreateMethod = "CreateWorkflowExact"
const exactWorkflowUpdateMethod = "UpdateWorkflowExact"
const exactWorkflowReorderMethod = "ReorderWorkflowsExact"
const exactWorkflowStepCreateMethod = "CreateWorkflowStepExact"
const exactWorkflowStepUpdateMethod = "UpdateWorkflowStepExact"
const exactWorkflowStepReorderMethod = "ReorderWorkflowStepsExact"
const exactRepositoryRegisterMethod = "RegisterRepositoryExact"
const exactRepositoryUpdateMethod = "UpdateRepositoryExact"
const exactTaskRelationAddMethod = "AddTaskRelationExact"
const exactTaskRelationRemoveMethod = "RemoveTaskRelationExact"
const exactTaskMessageMethod = "SendTaskMessageExact"
const exactEnsureTaskRunMethod = "EnsureTaskRunExact"
const exactStopTaskRunMethod = "StopTaskRunExact"
const exactRecoverSessionMethod = "RecoverSessionExact"
const exactCancelTaskTransitionMethod = "CancelPendingTaskTransitionExact"
const exactGetSessionModeMethod = "GetSessionModeContextExact"
const exactSetSessionModeMethod = "SetSessionModeExact"
const exactRespondPermissionMethod = "RespondPermissionExact"
const exactAnswerClarificationMethod = "AnswerClarificationExact"
const exactSourceIssueCommentMethod = "CommentSourceIssueExact"
const exactSourceIssueTransitionMethod = "TransitionSourceIssueExact"

var exactHostMethods = []exactMethod{
	{method: "GetCapabilityContext", description: "Read supported exact Host methods and their workspace grants"},
	{method: exactTaskUpdateMethod, capability: "host.v2.write:tasks", description: "Update one task at its current version"},
	{method: exactTaskCreateMethod, capability: "host.v2.write:tasks", description: "Create one task with a stable workspace source identity"},
	{method: exactTaskLabelsMethod, capability: "host.v2.write:tasks", description: "Replace task labels at their current version"},
	{method: exactTaskAssignmentMethod, capability: "host.v2.write:tasks", description: "Set or clear a task's human assignee at its current version"},
	{method: exactTaskMoveMethod, capability: "host.v2.write:tasks", description: "Move a task through workflow admission at its current version"},
	{method: exactTaskArchiveMethod, capability: "host.v2.write:tasks", description: "Archive a task through its native lifecycle at its current version"},
	{method: exactTaskManagementClaimAcquireMethod, capability: "host.v2.write:tasks", description: "Acquire management ownership for one task"},
	{method: exactTaskManagementClaimReleaseMethod, capability: "host.v2.write:tasks", description: "Release management ownership for one task"},
	{method: exactTaskManagementClaimTransferMethod, capability: "host.v2.write:tasks", description: "Transfer management ownership for one task"},
	{method: exactTaskCompletionCriteriaMethod, capability: "host.v2.write:tasks", description: "Set task-owned completion criteria at their current revisions"},
	{method: exactTaskCompletionEvidenceMethod, capability: "host.v2.write:tasks", description: "Verify one typed completion criterion at its current revisions"},
	{method: exactWorkspaceDefaultsMethod, capability: "host.v2.write:workspaces", description: "Update workspace defaults at the observed resource version"},
	{method: exactWorkflowCreateMethod, capability: "host.v2.write:workflows", description: "Create one workflow in the approved workspace"},
	{method: exactWorkflowUpdateMethod, capability: "host.v2.write:workflows", description: "Update one manually managed workflow at its observed version"},
	{method: exactWorkflowReorderMethod, capability: "host.v2.write:workflows", description: "Reorder the complete workspace workflow list at observed versions"},
	{method: exactWorkflowStepCreateMethod, capability: "host.v2.write:workflows", description: "Create one validated workflow step"},
	{method: exactWorkflowStepUpdateMethod, capability: "host.v2.write:workflows", description: "Update one workflow step at observed versions"},
	{method: exactWorkflowStepReorderMethod, capability: "host.v2.write:workflows", description: "Reorder workflow steps at observed versions"},
	{method: exactRepositoryRegisterMethod, capability: "host.v2.write:repositories", description: "Register a validated repository in the approved workspace"},
	{method: exactRepositoryUpdateMethod, capability: "host.v2.write:repositories", description: "Update safe repository settings at the observed version"},
	{method: exactTaskRelationAddMethod, capability: "host.v2.write:tasks", description: "Add a same-workspace dependency relation with cycle checks"},
	{method: exactTaskRelationRemoveMethod, capability: "host.v2.write:tasks", description: "Remove a same-workspace dependency relation"},
	{method: exactTaskMessageMethod, capability: "host.v2.write:messages", description: "Queue one version-fenced task message"},
	{method: exactEnsureTaskRunMethod, capability: "host.v2.write:execution", description: "Ensure one task has a live run through native launch admission"},
	{method: exactStopTaskRunMethod, capability: "host.v2.write:execution", description: "Stop only the observed session execution"},
	{method: exactRecoverSessionMethod, capability: "host.v2.write:execution", description: "Recover one observed interrupted session through native launch"},
	{method: exactCancelTaskTransitionMethod, capability: "host.v2.write:execution", description: "Cancel one observed pending workflow transition"},
	{method: exactGetSessionModeMethod, capability: "host.v2.read:sessions", description: "Read provider-advertised modes for one session"},
	{method: exactSetSessionModeMethod, capability: "host.v2.write:execution", description: "Set one provider-advertised safe mode on the observed execution"},
	{method: exactRespondPermissionMethod, capability: "host.v2.write:interactions", description: "Relay a human-authorized permission response to one pending request"},
	{method: exactAnswerClarificationMethod, capability: "host.v2.write:interactions", description: "Relay a human-authorized answer to one pending clarification"},
	{method: exactTaskDirectiveIssueMethod, capability: exactTaskDirectiveCapability, description: "Record one short-lived task directive"},
	{method: exactTaskDirectiveResolveMethod, capability: exactTaskDirectiveCapability, description: "Resolve one pending task directive"},
	{method: exactSourceIssueCommentMethod, capability: "host.v2.write:source_issues", description: "Comment on the task's existing linked Jira or Linear issue"},
	{method: exactSourceIssueTransitionMethod, capability: "host.v2.write:source_issues", description: "Transition the task's existing linked Jira or Linear issue"},
	{method: "EnsureManagedAgentConversationExact", capability: "host.v2.write:managed_agent_conversations", description: "Ensure or update one installation-owned conversation"},
	{method: "GetManagedAgentConversationStatusExact", capability: "host.v2.read:managed_agent_conversations", description: "Read one installation-owned conversation"},
	{method: "ListManagedAgentConversationsExact", capability: "host.v2.read:managed_agent_conversations", description: "List installation-owned conversations in a workspace"},
	{method: "SetManagedAgentConversationPausedExact", capability: "host.v2.write:managed_agent_conversations", description: "Pause or resume new conversation turns"},
	{method: "DeleteManagedAgentConversationExact", capability: "host.v2.write:managed_agent_conversations", description: "Delete one installation-owned conversation"},
	{method: "EnqueueManagedAgentInputExact", capability: "host.v2.write:managed_agent_conversations", description: "Append one durable input to a managed conversation"},
	{method: "GetManagedAgentInputExact", capability: "host.v2.read:managed_agent_conversations", description: "Read one managed conversation input receipt"},
	{method: "ListManagedAgentInputsExact", capability: "host.v2.read:managed_agent_conversations", description: "List managed conversation input receipts"},
	{method: "CancelManagedAgentInputExact", capability: "host.v2.write:managed_agent_conversations", description: "Cancel one exact managed conversation input"},
	{method: "DispatchManagedAgentConversationExact", capability: "host.v2.write:managed_agent_conversations", description: "Immediately dispatch without queueing when busy"},
	{method: "ListManagedConversationSchedulesExact", capability: "host.v2.read:automations", description: "List this installation's managed-conversation schedules"},
	{method: "CreateManagedConversationScheduleExact", capability: "host.v2.write:automations", description: "Create an installation-owned managed-conversation schedule"},
	{method: "UpdateManagedConversationScheduleExact", capability: "host.v2.write:automations", description: "Update an owned schedule at its observed revision"},
	{method: "SetManagedConversationScheduleEnabledExact", capability: "host.v2.write:automations", description: "Pause or enable an owned schedule at its observed revision"},
	{method: "DeleteManagedConversationScheduleExact", capability: "host.v2.write:automations", description: "Delete an owned schedule without deleting its destination"},
}

// GetCapabilityContext returns the exact method registry and the approval
// state for this installed plugin in workspaceID.
func (s *Service) GetCapabilityContext(installationID, workspaceID string) (*pluginsdk.CapabilityContext, error) {
	if !isBoundedApprovalIdentifier(workspaceID) {
		return nil, status.Error(codes.InvalidArgument, "workspace_id is required")
	}
	s.approvalEffectMu.Lock()
	defer s.approvalEffectMu.Unlock()

	installed := s.installedRecordByInstallationID(installationID)
	if installed == nil {
		return nil, status.Error(codes.PermissionDenied, "plugin installation is unavailable")
	}
	if s.approvals == nil {
		return nil, status.Error(codes.Unavailable, "capability approval store is unavailable")
	}
	approval, found, err := s.approvalCurrent(installationID, workspaceID)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "capability approval state is unavailable")
	}

	context := &pluginsdk.CapabilityContext{
		InstallationID: installed.InstallationID,
		WorkspaceID:    workspaceID,
		ManifestDigest: ManifestCapabilityDigest(installed.Manifest),
		Operations:     make([]pluginsdk.CapabilityOperation, 0, len(exactHostMethods)),
		Limits:         map[string]uint64{"page_size": 200, "command_payload_bytes": 65536},
	}
	if found {
		context.ApprovalRevision = approval.Revision
		context.ApprovalState = string(approval.State)
	} else {
		context.ApprovalState = string(ApprovalDenyMissingApproval)
	}
	methods := append(append([]exactMethod(nil), exactHostMethods...), exactReadMethods...)
	for _, method := range methods {
		operation := pluginsdk.CapabilityOperation{
			Method: method.method, CapabilityID: method.capability,
			Description: method.description, Supported: true,
		}
		if isExactWorkspaceAdminMethod(method.method) && s.workspaceAdminWriter == nil {
			operation.Supported = false
			operation.UnavailableReason = "workspace_administration_unavailable"
			context.Operations = append(context.Operations, operation)
			continue
		}
		if method.capability == "" {
			operation.Authorized = true
		} else {
			decision := s.authorizePluginCapability(
				installationID,
				workspaceID,
				method.capability,
				context.ApprovalRevision,
				CanonicalApprovalDigest("capability-context", workspaceID, method.method),
				CanonicalApprovalDigest("host-method", method.method),
			)
			operation.Authorized = decision.Allowed
			if !decision.Allowed {
				operation.UnavailableReason = string(decision.Reason)
			}
		}
		context.Operations = append(context.Operations, operation)
	}
	return context, nil
}

func (h *pluginHost) GetCapabilityContext(_ context.Context, workspaceID string) (*pluginsdk.CapabilityContext, error) {
	if h.service == nil || h.installationID == "" {
		return nil, status.Error(codes.Unavailable, "exact Host authorization is unavailable")
	}
	capability, err := h.service.GetCapabilityContext(h.installationID, workspaceID)
	if err != nil {
		return nil, err
	}
	commandStore := h.commandStore
	if commandStore == nil {
		commandStore = h.service.exactCommandStoreDep()
	}
	_, taskWriterAvailable := h.taskWriter.(exactTaskWriter)
	executionControllerAvailable := h.executionController() != nil
	interactionResponderAvailable := h.interactionResponder() != nil
	managedConversations := h.managedConversations
	var managedService ManagedAgentConversationService
	var managedInputService bool
	if managedConversations != nil {
		managedService = managedConversations()
		_, managedInputService = managedService.(ManagedAgentInputService)
	}
	managedAutomationSchedules := h.managedAutomationSchedules
	var managedAutomationSchedulesAvailable bool
	if managedAutomationSchedules != nil {
		managedAutomationSchedulesAvailable = managedAutomationSchedules() != nil
	}
	for index := range capability.Operations {
		operation := &capability.Operations[index]
		reason := h.capabilityOperationUnavailableReason(operation, taskWriterAvailable,
			commandStore != nil, managedService != nil, managedInputService,
			executionControllerAvailable, interactionResponderAvailable, managedAutomationSchedulesAvailable)
		if reason != "" {
			operation.Supported = false
			operation.Authorized = false
			operation.UnavailableReason = reason
		}
	}
	return capability, nil
}

func (h *pluginHost) capabilityOperationUnavailableReason(
	operation *pluginsdk.CapabilityOperation,
	taskWriterAvailable, commandStoreAvailable, managedServiceAvailable, managedInputServiceAvailable,
	executionControllerAvailable, interactionResponderAvailable, managedAutomationSchedulesAvailable bool,
) string {
	reason := exactOperationUnavailableReason(operation, taskWriterAvailable, commandStoreAvailable, managedServiceAvailable, managedInputServiceAvailable)
	if specialized := h.specializedOperationUnavailableReason(operation, commandStoreAvailable,
		executionControllerAvailable, interactionResponderAvailable, managedAutomationSchedulesAvailable); specialized != "" {
		reason = specialized
	}
	if reason == "" {
		reason = exactReadUnavailableReason(operation.Method, h)
	}
	return reason
}

func (h *pluginHost) specializedOperationUnavailableReason(
	operation *pluginsdk.CapabilityOperation,
	commandStoreAvailable, executionControllerAvailable, interactionResponderAvailable, managedAutomationSchedulesAvailable bool,
) string {
	if reason := managedScheduleUnavailableReason(operation, commandStoreAvailable, managedAutomationSchedulesAvailable); reason != "" {
		return reason
	}
	if reason := exactExecutionUnavailableReason(operation.Method, commandStoreAvailable, executionControllerAvailable); reason != "" {
		return reason
	}
	if reason := exactInteractionUnavailableReason(operation.Method, h, commandStoreAvailable, interactionResponderAvailable); reason != "" {
		return reason
	}
	if reason := exactTaskWriterUnavailableReason(operation.Method, h, commandStoreAvailable); reason != "" {
		return reason
	}
	if reason := exactExternalMutationUnavailableReason(operation.Method, h, commandStoreAvailable); reason != "" {
		return reason
	}
	return ""
}

func managedScheduleUnavailableReason(operation *pluginsdk.CapabilityOperation, commandStoreAvailable, scheduleServiceAvailable bool) string {
	if !isManagedAutomationScheduleMethod(operation.Method) {
		return ""
	}
	if !scheduleServiceAvailable {
		return "managed_automation_schedule_service_unavailable"
	}
	if strings.HasPrefix(operation.CapabilityID, "host.v2.write:") && !commandStoreAvailable {
		return exactReasonCommandStoreUnavailable
	}
	return ""
}

func exactExecutionUnavailableReason(method string, commandStoreAvailable, controllerAvailable bool) string {
	if !isExactExecutionMethod(method) {
		return ""
	}
	if !controllerAvailable {
		return "execution_control_unavailable"
	}
	if exactExecutionRequiresCommandStore(method) && !commandStoreAvailable {
		return exactReasonCommandStoreUnavailable
	}
	return ""
}

func exactInteractionUnavailableReason(method string, host *pluginHost, commandStoreAvailable, responderAvailable bool) string {
	if method != exactRespondPermissionMethod && method != exactAnswerClarificationMethod {
		return ""
	}
	if !responderAvailable || host.interactionData == nil || host.service.humanInteractionReceipts == nil || !commandStoreAvailable {
		return "human_interaction_service_unavailable"
	}
	return ""
}

func exactTaskWriterUnavailableReason(method string, host *pluginHost, commandStoreAvailable bool) string {
	reason := taskWriterCapabilityUnavailableReason(method, host.taskWriter)
	if reason != "" && commandStoreAvailable {
		return reason
	}
	if exactTaskWriterNeedsCommandStore(method) && !commandStoreAvailable {
		return exactReasonCommandStoreUnavailable
	}
	return reason
}

func exactExternalMutationUnavailableReason(method string, host *pluginHost, commandStoreAvailable bool) string {
	switch method {
	case exactSourceIssueCommentMethod, exactSourceIssueTransitionMethod:
		if host.sourceIssueController == nil {
			return "source_issue_controller_unavailable"
		}
		if !commandStoreAvailable {
			return exactReasonCommandStoreUnavailable
		}
	case exactTaskMessageMethod:
		messenger, _ := host.writeDependencies()
		if _, ok := messenger.(exactTaskMessageMessenger); !ok && commandStoreAvailable {
			return "exact_task_messages_unavailable"
		}
		if !commandStoreAvailable {
			return exactReasonCommandStoreUnavailable
		}
	case exactTaskDirectiveIssueMethod, exactTaskDirectiveResolveMethod:
		switch {
		case !commandStoreAvailable:
			return exactReasonCommandStoreUnavailable
		case host.taskData == nil:
			return "task_data_unavailable"
		case host.pendingTaskTransitions == nil || host.pendingTaskTransitions() == nil:
			return exactReasonTaskTransitionDataUnavailable
		}
	}
	return ""
}

func taskWriterCapabilityUnavailableReason(method string, writer any) string {
	switch method {
	case exactTaskCreateMethod:
		if _, ok := writer.(exactTaskCreateWriter); !ok {
			return "exact_task_create_unavailable"
		}
	case exactTaskLabelsMethod, exactTaskAssignmentMethod:
		if _, ok := writer.(exactTaskWriter); !ok {
			return "exact_task_update_unavailable"
		}
	case exactTaskMoveMethod:
		if _, ok := writer.(exactTaskMoveWriter); !ok {
			return "exact_task_move_unavailable"
		}
	case exactTaskArchiveMethod:
		if _, ok := writer.(exactTaskArchiveWriter); !ok {
			return "exact_task_archive_unavailable"
		}
	case exactTaskManagementClaimAcquireMethod, exactTaskManagementClaimReleaseMethod, exactTaskManagementClaimTransferMethod:
		if _, ok := writer.(exactTaskManagementClaimWriter); !ok {
			return "task_management_claims_unavailable"
		}
	case exactTaskCompletionCriteriaMethod, exactTaskCompletionEvidenceMethod:
		if _, ok := writer.(exactTaskCompletionGateWriter); !ok {
			return "task_completion_gates_unavailable"
		}
	case exactTaskRelationAddMethod, exactTaskRelationRemoveMethod:
		if _, ok := writer.(exactTaskRelationWriter); !ok {
			return "exact_task_relations_unavailable"
		}
	}
	return ""
}

func exactTaskWriterNeedsCommandStore(method string) bool {
	switch method {
	case exactTaskCreateMethod, exactTaskLabelsMethod, exactTaskAssignmentMethod, exactTaskMoveMethod,
		exactTaskArchiveMethod, exactTaskManagementClaimAcquireMethod, exactTaskManagementClaimReleaseMethod,
		exactTaskManagementClaimTransferMethod, exactTaskCompletionCriteriaMethod, exactTaskCompletionEvidenceMethod,
		exactTaskRelationAddMethod, exactTaskRelationRemoveMethod:
		return true
	default:
		return false
	}
}

func isManagedAutomationScheduleMethod(method string) bool {
	switch method {
	case "ListManagedConversationSchedulesExact", "CreateManagedConversationScheduleExact",
		"UpdateManagedConversationScheduleExact", "SetManagedConversationScheduleEnabledExact",
		"DeleteManagedConversationScheduleExact":
		return true
	default:
		return false
	}
}

func isExactExecutionMethod(method string) bool {
	switch method {
	case exactEnsureTaskRunMethod, exactStopTaskRunMethod, exactRecoverSessionMethod,
		exactCancelTaskTransitionMethod, exactGetSessionModeMethod, exactSetSessionModeMethod:
		return true
	default:
		return false
	}
}

func exactExecutionRequiresCommandStore(method string) bool {
	return method != exactGetSessionModeMethod
}

func exactReadUnavailableReason(method string, host *pluginHost) string {
	if reason := exactReadSpecificUnavailableReason(method, host); reason != "" {
		return reason
	}
	if _, requiresTaskData := exactReadTaskDataMethods[method]; requiresTaskData && host.taskData == nil {
		return "task_data_unavailable"
	}
	return ""
}

//nolint:cyclop // The method matrix lists each read dependency and its fail-closed reason.
func exactReadSpecificUnavailableReason(method string, host *pluginHost) string {
	switch method {
	case exactReadSourceIssueCapabilities:
		if host.sourceIssueController == nil {
			return "source_issue_controller_unavailable"
		}
		if host.taskData == nil {
			return "workspace_data_unavailable"
		}
	case exactReadListWorkflows, exactReadListWorkflowSteps:
		if host.workflows == nil || host.workflowSteps == nil {
			return "workflow_data_unavailable"
		}
	case exactReadListMessages:
		if host.messageData == nil {
			return "message_data_unavailable"
		}
	case exactReadListInteractions, exactReadGetInteraction:
		if host.interactionData == nil {
			return "interaction_data_unavailable"
		}
	case exactReadListTaskInbox:
		if host.interactionData == nil && host.pendingTaskTransitionSource() == nil {
			return "task_inbox_data_unavailable"
		}
	case exactReadGetTaskDirective, exactReadListTaskDirectives:
		if host.commandStoreForDirective() == nil {
			return "task_directive_data_unavailable"
		}
		if host.pendingTaskTransitions == nil || host.pendingTaskTransitions() == nil {
			return exactReasonTaskTransitionDataUnavailable
		}
	case exactReadListTaskTransitions:
		if host.pendingTaskTransitionSource() == nil {
			return exactReasonTaskTransitionDataUnavailable
		}
	case exactReadTaskUsage:
		if _, ok := host.taskData.(exactTaskObservationSource); !ok {
			return "task_usage_data_unavailable"
		}
	}
	return ""
}

var exactReadTaskDataMethods = map[string]struct{}{
	exactReadListWorkspaces: {}, exactReadListWorkflows: {}, exactReadListWorkflowSteps: {},
	exactReadListTasks: {}, exactReadGetTask: {}, exactReadListSessions: {},
	exactReadListInteractions: {}, exactReadGetInteraction: {}, exactReadListMessages: {},
	exactReadListTaskRelations: {}, exactReadGetTaskRelations: {}, exactReadListTaskInbox: {},
	exactReadGetTaskDirective: {}, exactReadListTaskDirectives: {}, exactReadListTaskTransitions: {},
	exactReadChangeEvidence: {}, exactReadTaskUsage: {},
}

//nolint:nestif // The operation capability check preserves the full authorization decision order.
func exactOperationUnavailableReason(
	operation *pluginsdk.CapabilityOperation,
	taskWriterAvailable, commandStoreAvailable, managedServiceAvailable, managedInputServiceAvailable bool,
) string {
	if operation.Method == exactTaskUpdateMethod {
		if !taskWriterAvailable {
			return "task_writer_unavailable"
		}
		if !commandStoreAvailable {
			return exactReasonCommandStoreUnavailable
		}
	}
	if requiresManagedInputService(operation.Method) {
		if !managedServiceAvailable {
			return exactReasonManagedConversationServiceUnavailable
		}
		if !managedInputServiceAvailable {
			return "managed_input_service_unavailable"
		}
		if operation.CapabilityID != "" && strings.HasPrefix(operation.CapabilityID, "host.v2.write:") && !commandStoreAvailable {
			return exactReasonCommandStoreUnavailable
		}
	} else if strings.Contains(operation.Method, "ManagedAgentConversation") {
		if !managedServiceAvailable {
			return exactReasonManagedConversationServiceUnavailable
		}
		if operation.CapabilityID != "" && strings.HasPrefix(operation.CapabilityID, "host.v2.write:") && !commandStoreAvailable {
			return exactReasonCommandStoreUnavailable
		}
	}
	return ""
}

func requiresManagedInputService(method string) bool {
	return strings.Contains(method, "ManagedAgentInput") || method == "DispatchManagedAgentConversationExact"
}

type exactTaskUpdateAdmission struct {
	writer   exactTaskWriter
	store    *state.CommandStore
	record   state.CommandRecord
	replayed bool
	unlock   func()
}

func (h *pluginHost) UpdateTaskExact(ctx context.Context, input pluginsdk.ExactTaskUpdate) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return h.updateTaskExact(ctx, input, exactTaskUpdateMethod, nil, nil)
}

func (h *pluginHost) SetTaskLabelsExact(ctx context.Context, input pluginsdk.ExactTaskLabels) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	labels := make([]string, len(input.Labels))
	copy(labels, input.Labels)
	update := pluginsdk.ExactTaskUpdate{
		RequestID: input.RequestID, WorkspaceID: input.WorkspaceID, TaskID: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	}
	return h.updateTaskExact(ctx, update, exactTaskLabelsMethod, &labels, nil)
}

func (h *pluginHost) AssignTaskExact(ctx context.Context, input pluginsdk.ExactTaskAssignment) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	assigneeUserID := input.AssigneeUserID
	update := pluginsdk.ExactTaskUpdate{
		RequestID: input.RequestID, WorkspaceID: input.WorkspaceID, TaskID: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	}
	return h.updateTaskExact(ctx, update, exactTaskAssignmentMethod, nil, &assigneeUserID)
}

func (h *pluginHost) updateTaskExact(
	ctx context.Context,
	input pluginsdk.ExactTaskUpdate,
	method string,
	labels *[]string,
	assigneeUserID *string,
) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	if err := validateExactTaskUpdate(input, labels, assigneeUserID); err != nil {
		return invalidExactTaskUpdateResult(), nil, nil
	}
	payloadDigest, err := exactUpdateDigest(input, labels, assigneeUserID)
	if err != nil {
		return invalidExactTaskUpdateResult(), nil, nil
	}
	admission, result := h.admitExactTaskUpdate(ctx, input, payloadDigest, method)
	if result != nil {
		return result, nil, nil
	}
	defer admission.unlock()
	if admission.replayed && admission.record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(admission.record), h.exactTaskUpdateReplayValue(ctx, input.TaskID), nil
	}
	return h.applyExactTaskUpdate(ctx, admission, input, payloadDigest, labels, assigneeUserID)
}

func invalidExactTaskUpdateResult() *pluginsdk.CommandResult {
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}
}

func (h *pluginHost) admitExactTaskUpdate(
	ctx context.Context,
	input pluginsdk.ExactTaskUpdate,
	payloadDigest string,
	method string,
) (*exactTaskUpdateAdmission, *pluginsdk.CommandResult) {
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
	writer, result := h.exactTaskUpdateWriter()
	if result != nil {
		unlock()
		return nil, result
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: exactReasonCommandStoreUnavailable}
	}
	record, replayed, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: method, CapabilityID: "host.v2.write:tasks",
		PayloadDigest: payloadDigest, TargetID: input.TaskID,
		ExpectedResourceVersion: input.ExpectedResourceVersion,
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
	return &exactTaskUpdateAdmission{writer: writer, store: store, record: record, replayed: replayed, unlock: unlock}, nil
}

func (h *pluginHost) exactTaskUpdateWriter() (exactTaskWriter, *pluginsdk.CommandResult) {
	if h.taskWriter == nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "task_writer_unavailable"}
	}
	writer, ok := h.taskWriter.(exactTaskWriter)
	if !ok {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "exact_task_update_unsupported"}
	}
	return writer, nil
}

func (h *pluginHost) exactTaskUpdateReplayValue(ctx context.Context, taskID string) *pluginsdk.Task {
	if h.taskData == nil {
		return nil
	}
	model, err := h.taskData.GetTask(ctx, taskID)
	if err != nil || model == nil {
		return nil
	}
	dto := taskModelToDTO(model)
	return &dto
}

func (h *pluginHost) applyExactTaskUpdate(
	ctx context.Context,
	admission *exactTaskUpdateAdmission,
	input pluginsdk.ExactTaskUpdate,
	payloadDigest string,
	labels *[]string,
	assigneeUserID *string,
) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	updated, alreadyApplied, err := admission.writer.UpdateTaskExact(ctx, ExactTaskUpdateInput{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ExpectedResourceVersion: input.ExpectedResourceVersion,
		OperationID:             admission.record.Intent.OperationID, PayloadDigest: payloadDigest,
		Title: input.Title, Description: input.Description, State: input.State, Priority: input.Priority,
		Labels: labels, AssigneeUserID: assigneeUserID,
		ClaimFence: taskmodels.TaskManagementClaimFence{
			InstallationID: h.installationID, InstanceKey: input.ManagementInstanceKey,
			Generation: input.ExpectedClaimGeneration,
		},
	})
	if err != nil {
		return h.completeExactTaskUpdateError(ctx, admission, input, err)
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	resourceVersion := ""
	var taskDTO *pluginsdk.Task
	if updated != nil {
		dto := taskModelToDTO(updated)
		taskDTO = &dto
		resourceVersion = dto.ResourceVersion
	}
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), "", input.TaskID, resourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, taskDTO, nil
	}
	return commandResultFromRecord(completed), taskDTO, nil
}

func (h *pluginHost) completeExactTaskUpdateError(
	ctx context.Context,
	admission *exactTaskUpdateAdmission,
	input pluginsdk.ExactTaskUpdate,
	updateErr error,
) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	resultStatus := exactTaskUpdateErrorStatus(updateErr)
	if resultStatus == pluginsdk.CommandUnavailable {
		return &pluginsdk.CommandResult{Status: resultStatus, Reason: "task_update_unavailable"}, nil, nil
	}
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), string(resultStatus), input.TaskID, "")
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil, nil
	}
	return commandResultFromRecord(completed), nil, nil
}

//nolint:cyclop // Each optional field has an explicit bound and native update mapping.
func validateExactTaskUpdate(input pluginsdk.ExactTaskUpdate, labels *[]string, assigneeUserID *string) error {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.TaskID) || !isBoundedApprovalIdentifier(input.IdempotencyKey) ||
		!isBoundedApprovalIdentifier(input.ExpectedResourceVersion) {
		return errors.New("required exact task update identity is missing or invalid")
	}
	if input.Title == nil && input.Description == nil && input.State == nil && input.Priority == nil && labels == nil && assigneeUserID == nil {
		return errors.New("exact task update has no changes")
	}
	if input.ApprovalRevision == 0 || len(input.ManifestDigest) != 64 {
		return errors.New("approval context is missing or invalid")
	}
	if !validManagementClaimFence(input.ManagementInstanceKey, input.ExpectedClaimGeneration) {
		return errors.New("management claim fence is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, input.ExpectedResourceVersion); err != nil {
		return fmt.Errorf("expected resource version is invalid: %w", err)
	}
	if input.State != nil && !validExactPluginTaskState(v1.TaskState(*input.State)) {
		return errors.New("invalid task state")
	}
	return nil
}

func exactUpdateDigest(input pluginsdk.ExactTaskUpdate, labels *[]string, assigneeUserID *string) (string, error) {
	canonical := struct {
		WorkspaceID             string    `json:"workspace_id"`
		TaskID                  string    `json:"task_id"`
		ExpectedResourceVersion string    `json:"expected_resource_version"`
		Title                   *string   `json:"title"`
		Description             *string   `json:"description"`
		State                   *string   `json:"state"`
		Priority                *string   `json:"priority"`
		Labels                  *[]string `json:"labels"`
		AssigneeUserID          *string   `json:"assignee_user_id"`
		ManagementInstanceKey   string    `json:"management_instance_key"`
		ExpectedClaimGeneration int64     `json:"expected_claim_generation"`
	}{
		WorkspaceID: input.WorkspaceID, TaskID: input.TaskID,
		ExpectedResourceVersion: input.ExpectedResourceVersion,
		Title:                   input.Title, Description: input.Description,
		State: input.State, Priority: input.Priority, Labels: labels, AssigneeUserID: assigneeUserID,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	if len(encoded) > 65536 {
		return "", errors.New("exact task update payload is too large")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validExactPluginTaskState(state v1.TaskState) bool {
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateInProgress,
		v1.TaskStateReview, v1.TaskStateBlocked, v1.TaskStateWaitingForInput,
		v1.TaskStateCompleted, v1.TaskStateFailed, v1.TaskStateCancelled:
		return true
	default:
		return false
	}
}

func exactTaskUpdateErrorStatus(err error) pluginsdk.CommandStatus {
	switch {
	case errors.Is(err, repoerrors.ErrTaskVersionConflict),
		errors.Is(err, repoerrors.ErrTaskOperationConflict),
		errors.Is(err, repoerrors.ErrTaskManagementClaimConflict):
		return pluginsdk.CommandConflict
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return pluginsdk.CommandNotFound
	default:
		if status.Code(err) == codes.InvalidArgument {
			return pluginsdk.CommandInvalid
		}
		return pluginsdk.CommandUnavailable
	}
}

func commandResultFromRecord(record state.CommandRecord) *pluginsdk.CommandResult {
	commandStatus := pluginsdk.CommandStatus(record.Receipt.ResultStatus)
	if commandStatus == "" {
		commandStatus = pluginsdk.CommandUnavailable
	}
	receipt := &pluginsdk.CommandReceipt{
		ID: record.Receipt.ID, OperationID: record.Receipt.OperationID,
		Status: commandStatus, TargetID: record.Receipt.TargetID,
		ResourceVersion:  record.Receipt.ResourceVersion,
		ApprovalRevision: record.Intent.ApprovalRevision,
		CreatedAt:        record.Receipt.CreatedAt, UpdatedAt: record.Receipt.UpdatedAt,
	}
	return &pluginsdk.CommandResult{Status: commandStatus, Reason: record.Receipt.Reason, Receipt: receipt}
}
