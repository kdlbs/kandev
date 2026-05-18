/**
 * Pure prop-assembly helpers for the task-create dialog. Extracted from
 * task-create-dialog.tsx so the orchestrator stays under the per-file line
 * cap — these are non-React, no-JSX projections from the setup hook's
 * result + dialog props, so they belong outside the component module.
 */
// Both names are used only in type positions (interface + `typeof` in
// ReturnType<>), so the `import type` form makes the otherwise-circular
// dependency with task-create-dialog.tsx explicitly type-only — bundlers
// and analysis tools won't treat it as a real runtime cycle.
import type {
  TaskCreateDialogProps,
  useTaskCreateDialogSetup,
} from "@/components/task-create-dialog";
import type { DialogFormBodyProps, DialogFormState } from "@/components/task-create-dialog-types";

export function computeHasAllBranches(fs: DialogFormState): boolean {
  if (fs.noRepository) return true;
  if (fs.useGitHubUrl) return !!fs.githubBranch;
  return fs.repositories.length > 0 && fs.repositories.every((r) => !!r.branch);
}

export function buildDialogFormBodyProps(
  setup: ReturnType<typeof useTaskCreateDialogSetup>,
  props: TaskCreateDialogProps,
): DialogFormBodyProps {
  const { fs, computed, handlers } = setup;
  const repoLocked = !!props.lockedFields?.repository;
  return {
    isSessionMode: setup.isSessionMode,
    isCreateMode: setup.isCreateMode,
    isEditMode: setup.isEditMode,
    isTaskStarted: setup.isTaskStarted,
    onTaskNameChange: handlers.handleTaskNameChange,
    onRowRepositoryChange: handlers.handleRowRepositoryChange,
    onRowBranchChange: handlers.handleRowBranchChange,
    isPassthroughProfile: computed.isPassthroughProfile,
    initialDescription: fs.currentDefaults.description,
    hasDescription: fs.hasDescription,
    workspaceId: props.workspaceId,
    onJiraImport: setup.handleJiraImport,
    onLinearImport: setup.handleLinearImport,
    agentProfileOptions: computed.agentProfileOptions,
    executorProfileOptions: computed.executorProfileOptions,
    agentProfiles: setup.agentProfiles,
    agentProfilesLoading: computed.agentProfilesLoading,
    executorsLoading: computed.executorsLoading,
    isCreatingSession: fs.isCreatingSession,
    workflows: setup.workflows,
    snapshots: setup.snapshots,
    effectiveWorkflowId: computed.effectiveWorkflowId ?? null,
    fs,
    handleKeyDown: setup.handleKeyDown,
    onAgentProfileChange: handlers.handleAgentProfileChange,
    onExecutorProfileChange: handlers.handleExecutorProfileChange,
    onWorkflowChange: handlers.handleWorkflowChange,
    onToggleGitHubUrl: repoLocked ? undefined : handlers.handleToggleGitHubUrl,
    onGitHubUrlChange: handlers.handleGitHubUrlChange,
    onToggleFreshBranch: handlers.handleToggleFreshBranch,
    onToggleNoRepository: repoLocked ? undefined : handlers.handleToggleNoRepository,
    onWorkspacePathChange: handlers.handleWorkspacePathChange,
    enhance: setup.enhance,
    workflowAgentLocked: computed.workflowAgentLocked,
    repositories: setup.repositories,
    freshBranchAvailable: setup.freshBranchAvailable,
    isLocalExecutor: computed.isLocalExecutor,
    noCompatibleAgent: computed.noCompatibleAgent,
    executorProfileName: computed.selectedExecutorProfileName,
    extraFormSlot: props.extraFormSlot,
    aboveDescriptionSlot: props.aboveDescriptionSlot,
    bottomSlot: props.bottomSlot,
    descriptionPlaceholder: props.descriptionPlaceholder,
    workflowLocked: props.lockedFields?.workflow,
  };
}

export function buildDialogFooterProps(
  setup: ReturnType<typeof useTaskCreateDialogSetup>,
  props: TaskCreateDialogProps,
) {
  const { fs, computed, submitHandlers } = setup;
  return {
    isSessionMode: setup.isSessionMode,
    isCreateMode: setup.isCreateMode,
    isEditMode: setup.isEditMode,
    isTaskStarted: setup.isTaskStarted,
    isPassthroughProfile: computed.isPassthroughProfile,
    isCreatingSession: fs.isCreatingSession,
    isCreatingTask: fs.isCreatingTask,
    hasTitle: fs.hasTitle,
    hasDescription: fs.hasDescription,
    hasRepositorySelection: computed.hasRepositorySelection,
    hasAllBranches: computeHasAllBranches(fs),
    agentProfileId: computed.effectiveAgentProfileId,
    workspaceId: props.workspaceId,
    effectiveWorkflowId: computed.effectiveWorkflowId ?? null,
    executorHint: computed.executorHint,
    noCompatibleAgent: computed.noCompatibleAgent,
    executorProfileName: computed.selectedExecutorProfileName,
    onCancel: submitHandlers.handleCancel,
    onUpdateWithoutAgent: submitHandlers.handleUpdateWithoutAgent,
    onCreateWithoutAgent: submitHandlers.handleCreateWithoutAgent,
    onCreateWithPlanMode: submitHandlers.handleCreateWithPlanMode,
    submitBlockedReason: props.submitBlockedReason,
  };
}
