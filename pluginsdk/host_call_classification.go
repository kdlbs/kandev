package pluginsdk

import pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"

// Host domain calls must retain the verified server invocation that produced
// them. Plugin-owned capabilities keep their existing native rules so detached
// and background plugin work does not lose authority when collaboration
// enforcement is enabled. Both sets are explicit: every method in the generated
// Host service is classified exactly once, and an unclassified method defaults
// to requiring a caller.
var hostCallCallerExempt = map[string]struct{}{
	pluginv1.Host_GetCapabilityContext_FullMethodName:            {},
	pluginv1.Host_GetSourceIssueCapabilitiesExact_FullMethodName: {},
	pluginv1.Host_CommentSourceIssueExact_FullMethodName:         {},
	pluginv1.Host_TransitionSourceIssueExact_FullMethodName:      {},

	pluginv1.Host_EnsureManagedAgentConversationExact_FullMethodName:        {},
	pluginv1.Host_GetManagedAgentConversationStatusExact_FullMethodName:     {},
	pluginv1.Host_ListManagedAgentConversationsExact_FullMethodName:         {},
	pluginv1.Host_SetManagedAgentConversationPausedExact_FullMethodName:     {},
	pluginv1.Host_DeleteManagedAgentConversationExact_FullMethodName:        {},
	pluginv1.Host_EnqueueManagedAgentInputExact_FullMethodName:              {},
	pluginv1.Host_GetManagedAgentInputExact_FullMethodName:                  {},
	pluginv1.Host_ListManagedAgentInputsExact_FullMethodName:                {},
	pluginv1.Host_CancelManagedAgentInputExact_FullMethodName:               {},
	pluginv1.Host_DispatchManagedAgentConversationExact_FullMethodName:      {},
	pluginv1.Host_ListManagedConversationSchedulesExact_FullMethodName:      {},
	pluginv1.Host_CreateManagedConversationScheduleExact_FullMethodName:     {},
	pluginv1.Host_UpdateManagedConversationScheduleExact_FullMethodName:     {},
	pluginv1.Host_SetManagedConversationScheduleEnabledExact_FullMethodName: {},
	pluginv1.Host_DeleteManagedConversationScheduleExact_FullMethodName:     {},

	pluginv1.Host_GetState_FullMethodName:     {},
	pluginv1.Host_SetState_FullMethodName:     {},
	pluginv1.Host_DeleteState_FullMethodName:  {},
	pluginv1.Host_ListState_FullMethodName:    {},
	pluginv1.Host_GetConfig_FullMethodName:    {},
	pluginv1.Host_RevealSecret_FullMethodName: {},
	pluginv1.Host_GetSecret_FullMethodName:    {},
	pluginv1.Host_SetSecret_FullMethodName:    {},
	pluginv1.Host_DeleteSecret_FullMethodName: {},
	pluginv1.Host_EmitEvent_FullMethodName:    {},

	pluginv1.Host_ListAgentProfiles_FullMethodName:             {},
	pluginv1.Host_ListExecutorProfiles_FullMethodName:          {},
	pluginv1.Host_CheckpointExecutorResource_FullMethodName:    {},
	pluginv1.Host_ReportExecutorProgress_FullMethodName:        {},
	pluginv1.Host_ReadExecutorRuntimeArtifact_FullMethodName:   {},
	pluginv1.Host_InvokeUtilityAgent_FullMethodName:            {},
	pluginv1.Host_InvokeUtilityAgentWithOptions_FullMethodName: {},
	pluginv1.Host_PreviewPluginOwnedTaskTree_FullMethodName:    {},
	pluginv1.Host_DeletePluginOwnedTaskTree_FullMethodName:     {},
}

var hostCallCallerRequired = map[string]struct{}{
	pluginv1.Host_UpdateTaskExact_FullMethodName:                    {},
	pluginv1.Host_CreateTaskExact_FullMethodName:                    {},
	pluginv1.Host_SetTaskLabelsExact_FullMethodName:                 {},
	pluginv1.Host_AssignTaskExact_FullMethodName:                    {},
	pluginv1.Host_MoveTaskExact_FullMethodName:                      {},
	pluginv1.Host_ArchiveTaskExact_FullMethodName:                   {},
	pluginv1.Host_AddTaskRelationExact_FullMethodName:               {},
	pluginv1.Host_RemoveTaskRelationExact_FullMethodName:            {},
	pluginv1.Host_SendTaskMessageExact_FullMethodName:               {},
	pluginv1.Host_IssueTaskDirectiveExact_FullMethodName:            {},
	pluginv1.Host_ResolveTaskDirectiveExact_FullMethodName:          {},
	pluginv1.Host_AcquireTaskManagementClaimExact_FullMethodName:    {},
	pluginv1.Host_ReleaseTaskManagementClaimExact_FullMethodName:    {},
	pluginv1.Host_TransferTaskManagementClaimExact_FullMethodName:   {},
	pluginv1.Host_SetTaskCompletionCriteriaExact_FullMethodName:     {},
	pluginv1.Host_VerifyTaskCompletionCriterionExact_FullMethodName: {},
	pluginv1.Host_ApplyWorkspaceAdministrationExact_FullMethodName:  {},

	pluginv1.Host_EnsureTaskRunExact_FullMethodName:               {},
	pluginv1.Host_StopTaskRunExact_FullMethodName:                 {},
	pluginv1.Host_RecoverSessionExact_FullMethodName:              {},
	pluginv1.Host_CancelPendingTaskTransitionExact_FullMethodName: {},
	pluginv1.Host_GetSessionModeContextExact_FullMethodName:       {},
	pluginv1.Host_SetSessionModeExact_FullMethodName:              {},
	pluginv1.Host_RespondPermissionExact_FullMethodName:           {},
	pluginv1.Host_AnswerClarificationExact_FullMethodName:         {},

	pluginv1.Host_ListWorkspacesExact_FullMethodName:             {},
	pluginv1.Host_ListWorkflowsExact_FullMethodName:              {},
	pluginv1.Host_ListWorkflowStepsExact_FullMethodName:          {},
	pluginv1.Host_ListTasksExact_FullMethodName:                  {},
	pluginv1.Host_GetTaskExact_FullMethodName:                    {},
	pluginv1.Host_ListSessionsExact_FullMethodName:               {},
	pluginv1.Host_ListPendingInteractionsExact_FullMethodName:    {},
	pluginv1.Host_GetInteractionExact_FullMethodName:             {},
	pluginv1.Host_ListSanitizedMessagesExact_FullMethodName:      {},
	pluginv1.Host_ListTaskInboxExact_FullMethodName:              {},
	pluginv1.Host_GetTaskDirectiveExact_FullMethodName:           {},
	pluginv1.Host_ListTaskDirectivesExact_FullMethodName:         {},
	pluginv1.Host_GetTaskRelationsExact_FullMethodName:           {},
	pluginv1.Host_ListTaskRelationsExact_FullMethodName:          {},
	pluginv1.Host_ListPendingTaskTransitionsExact_FullMethodName: {},
	pluginv1.Host_ListChangeRequestEvidenceExact_FullMethodName:  {},
	pluginv1.Host_ListTaskUsageExact_FullMethodName:              {},
	pluginv1.Host_ListAutomationDefinitionsExact_FullMethodName:  {},
	pluginv1.Host_ListAutomationRunsExact_FullMethodName:        {},
	pluginv1.Host_GetAutomationRunExact_FullMethodName:           {},

	pluginv1.Host_ListTasks_FullMethodName:                    {},
	pluginv1.Host_GetTask_FullMethodName:                      {},
	pluginv1.Host_ListTaskStepTransitions_FullMethodName:      {},
	pluginv1.Host_ListWorkspaces_FullMethodName:               {},
	pluginv1.Host_ListWorkflows_FullMethodName:                {},
	pluginv1.Host_ListWorkflowSteps_FullMethodName:            {},
	pluginv1.Host_ListWorkflowTransitionGroups_FullMethodName: {},
	pluginv1.Host_ListRepositories_FullMethodName:             {},
	pluginv1.Host_ListSessions_FullMethodName:                 {},
	pluginv1.Host_ListSessionCodeStats_FullMethodName:         {},
	pluginv1.Host_ListMessages_FullMethodName:                 {},
	pluginv1.Host_ListPendingInteractions_FullMethodName:      {},
	pluginv1.Host_GetInteraction_FullMethodName:               {},

	pluginv1.Host_CreateTask_FullMethodName:  {},
	pluginv1.Host_UpdateTask_FullMethodName:  {},
	pluginv1.Host_MoveTask_FullMethodName:    {},
	pluginv1.Host_SendMessage_FullMethodName: {},

	pluginv1.Host_EnsureAgentConversation_FullMethodName:   {},
	pluginv1.Host_DispatchAgentConversation_FullMethodName: {},
	pluginv1.Host_DeleteAgentConversation_FullMethodName:   {},
	pluginv1.Host_RespondToPermission_FullMethodName:       {},
	pluginv1.Host_AnswerClarification_FullMethodName:       {},
	pluginv1.Host_CancelClarification_FullMethodName:       {},
}

// hostOperationNeedsCaller reports whether a Host method carries conversation
// domain authority. Unclassified methods fail closed by requiring a caller.
func hostOperationNeedsCaller(method string) bool {
	if _, exempt := hostCallCallerExempt[method]; exempt {
		return false
	}
	return true
}
