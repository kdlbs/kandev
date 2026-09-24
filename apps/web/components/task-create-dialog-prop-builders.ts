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
import type { TaskCreateDialogProps } from "@/components/task-create-dialog";
import type { useTaskCreateDialogSetup } from "@/components/task-create-dialog-setup";
import type { DialogFormBodyProps, DialogFormState } from "@/components/task-create-dialog-types";
import {
  resolveTaskCreateLaunchPreview,
  type TaskCreateLaunchPreview,
} from "@/components/task-create-dialog-launch-preview";
import {
  computeRunnerEditable,
  computeRunnerIneligibleReason,
} from "@/components/task-create-dialog-helpers";
import { hasUnavailablePickerRemoteProvider } from "@/components/task-create-dialog-remote-provider-readiness";

export function computeHasAllBranches(fs: DialogFormState): boolean {
  if (fs.noRepository) return true;
  if (fs.repositorySelections) {
    const selected = fs.repositorySelections.filter(hasSelectedSourceValue);
    return (
      selected.length > 0 &&
      selected.every(hasSelectedSourceBranch) &&
      !hasUnavailablePickerRemoteProvider(selected, fs.remoteProviderReadiness)
    );
  }
  if (fs.useRemote) {
    const rows = fs.remoteRepos.filter((r) => r.url.trim() !== "");
    return rows.length > 0 && rows.every((r) => !!r.branch);
  }
  return (
    fs.repositories.length > 0 && fs.repositories.every((r) => Boolean(r.baseBranch || r.branch))
  );
}

function hasSelectedSourceValue(
  selection: NonNullable<DialogFormState["repositorySelections"]>[number],
): boolean {
  if (selection.kind === "remote") return selection.url.trim() !== "";
  if (selection.kind === "folder") return Boolean(selection.localPath.trim());
  return Boolean(selection.repositoryId || selection.localPath);
}

function hasSelectedSourceBranch(
  selection: NonNullable<DialogFormState["repositorySelections"]>[number],
): boolean {
  if (selection.kind === "folder") return true;
  if (selection.kind === "local") return Boolean(selection.branch || selection.baseBranch);
  return Boolean(selection.branch);
}

export function localRepositoryCreationEnabled(isCreateMode: boolean, repoLocked: boolean) {
  return isCreateMode && !repoLocked;
}

export function resolveDialogLaunchPreview(
  isCreateMode: boolean,
  effectiveWorkflowId: string | null,
  fetchedSteps: DialogFormState["fetchedSteps"],
  snapshots: DialogFormBodyProps["snapshots"],
  hasDescription: boolean,
): TaskCreateLaunchPreview | null {
  if (!isCreateMode) return null;
  return resolveTaskCreateLaunchPreview({
    effectiveWorkflowId,
    fetchedSteps,
    snapshotSteps: effectiveWorkflowId ? snapshots[effectiveWorkflowId]?.steps : undefined,
    launchIntent: hasDescription ? "start-agent" : "plan-mode",
  });
}

// eslint-disable-next-line max-lines-per-function -- one projection keeps the form contract assembled in one place.
export function buildDialogFormBodyProps(
  setup: ReturnType<typeof useTaskCreateDialogSetup>,
  props: TaskCreateDialogProps,
): DialogFormBodyProps {
  const { fs, computed, handlers } = setup;
  const repoLocked = !!props.lockedFields?.repository;
  const effectiveWorkflowId = computed.effectiveWorkflowId ?? null;
  const workflowAgentOverrideState = setup.workflowAgentOverrideValidation;
  return {
    isSessionMode: setup.isSessionMode,
    isCreateMode: setup.isCreateMode,
    isEditMode: setup.isEditMode,
    autoTitle: setup.autoTitle,
    isTaskStarted: setup.isTaskStarted,
    onTaskNameChange: handlers.handleTaskNameChange,
    onRowRepositoryChange: handlers.handleRowRepositoryChange,
    onRowBranchChange: handlers.handleRowBranchChange,
    onRowPolicyChange: handlers.handleRowPolicyChange,
    repositoryLocked: repoLocked,
    branchLocked: !!props.lockedFields?.branch,
    initialDescription: fs.currentDefaults.description,
    workspaceId: props.workspaceId,
    onJiraImport: setup.handleJiraImport,
    onLinearImport: setup.handleLinearImport,
    agentProfileOptions: computed.agentProfileOptions,
    executorProfileOptions: computed.executorProfileOptions,
    agentProfiles: setup.agentProfiles,
    agentProfilesLoading: computed.agentProfilesLoading,
    executorsLoading: computed.executorsLoading,
    isCreatingSession: fs.isCreatingSession,
    isCreatingTask: fs.isCreatingTask,
    workflows: setup.workflows,
    snapshots: setup.snapshots,
    effectiveWorkflowId,
    launchPreview: resolveDialogLaunchPreview(
      setup.isCreateMode,
      effectiveWorkflowId,
      fs.fetchedSteps,
      setup.snapshots,
      fs.hasDescription,
    ),
    fs,
    editDependencies: setup.editDependencies,
    handleKeyDown: setup.handleKeyDown,
    onAgentProfileChange: handlers.handleAgentProfileChange,
    onExecutorProfileChange: handlers.handleExecutorProfileChange,
    onWorkflowChange: handlers.handleWorkflowChange,
    onToggleRemote: repoLocked ? undefined : handlers.handleToggleRemote,
    onToggleFreshBranch: handlers.handleToggleFreshBranch,
    onToggleNoRepository: repoLocked ? undefined : handlers.handleToggleNoRepository,
    onWorkspacePathChange: handlers.handleWorkspacePathChange,
    onFolderSelectionAdded: handlers.onFolderSelectionAdded,
    onRepositorySelectionAdded: handlers.onRepositorySelectionAdded,
    onAllWorkspaceSourcesRemoved: handlers.onAllWorkspaceSourcesRemoved,
    onRepositorySelectionRemoved: handlers.onRepositorySelectionRemoved,
    localRepositoryCreation: localRepositoryCreationEnabled(setup.isCreateMode, repoLocked)
      ? {
          executorSelection: handlers.directLocalExecutorSelection,
          onCreated: handlers.handleLocalRepositoryCreated,
        }
      : undefined,
    enhance: setup.enhance,
    workflowAgentLocked: computed.workflowAgentLocked,
    repositories: setup.repositories,
    onRefreshRepositories: setup.refreshRepositories,
    repositoriesRefreshing: setup.repositoriesLoading,
    lastUsedBranch: setup.taskCreateLastUsed.branch,
    userSettingsLoaded: setup.userSettingsLoaded,
    freshBranchAvailable: setup.freshBranchAvailable,
    // Applying a set writes into the same ordered draft as the repository picker.
    repositorySets: repoLocked ? undefined : setup.repositorySets,
    isLocalExecutor: computed.isLocalExecutor,
    executorSourcePolicy: computed.executorSourcePolicy,
    executorSourceNotice: computed.executorSourceNotice,
    remoteOriginStates: computed.remoteOriginStates,
    refreshRemoteOrigins: computed.refreshRemoteOrigins,
    agentCompatState: computed.agentCompatState,
    selectedAgentProfileName: computed.selectedAgentProfileName,
    effectiveWorkflowName: resolveWorkflowName(setup.workflows, computed.effectiveWorkflowId),
    executorProfileName: computed.selectedExecutorProfileName,
    extraFormSlot: props.extraFormSlot,
    aboveDescriptionSlot: props.aboveDescriptionSlot,
    bottomSlot: props.bottomSlot,
    descriptionPlaceholder: props.descriptionPlaceholder,
    workflowLocked: props.lockedFields?.workflow,
    workflowAgentOverrideRows: workflowAgentOverrideState.rows,
    workflowAgentOverrideOptions: workflowAgentOverrideState.options,
    workflowAgentOverridesLoading: workflowAgentOverrideState.loading,
    workflowAgentOverridesInvalid: workflowAgentOverrideState.invalid,
    workflowAgentOverridesError: workflowAgentOverrideState.error,
    onWorkflowAgentOverrideChange: (sourceProfileId, replacementProfileId) => {
      const next = { ...fs.workflowAgentOverrides };
      if (replacementProfileId) next[sourceProfileId] = replacementProfileId;
      else delete next[sourceProfileId];
      fs.setWorkflowAgentOverrides(next);
    },
    onResetWorkflowAgentOverrides: () => fs.setWorkflowAgentOverrides({}),
    onRetryWorkflowAgentOverrides: () => setup.refreshWorkspaceSnapshots(true),
    runnerEditable: computeRunnerEditable(setup.isEditMode, props.editingTask),
    runnerIneligibleReason: computeRunnerIneligibleReason(props.editingTask),
  };
}

/** Name of the effective workflow, for copy that has to name it. */
export function resolveWorkflowName(
  workflows: ReadonlyArray<{ id: string; name: string }>,
  effectiveWorkflowId: string | null | undefined,
): string | null {
  if (!effectiveWorkflowId) return null;
  return workflows.find((workflow) => workflow.id === effectiveWorkflowId)?.name ?? null;
}

export function buildDialogFooterProps(
  setup: ReturnType<typeof useTaskCreateDialogSetup>,
  props: TaskCreateDialogProps,
  pendingAttachmentUploadReason?: string | null,
) {
  const { fs, computed, submitHandlers } = setup;
  const workflowAgentOverridesBlockedReason = setup.workflowAgentOverrideValidation.blockedReason;
  return {
    isSessionMode: setup.isSessionMode,
    isCreateMode: setup.isCreateMode,
    isEditMode: setup.isEditMode,
    autoTitle: setup.autoTitle,
    isTaskStarted: setup.isTaskStarted,
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
    executorSourceNotice: computed.executorSourceNotice,
    noCompatibleAgent: computed.noCompatibleAgent,
    agentCompatState: computed.agentCompatState,
    selectedAgentProfileName: computed.selectedAgentProfileName,
    executorProfileName: computed.selectedExecutorProfileName,
    onCancel: submitHandlers.handleCancel,
    onUpdateWithoutAgent: submitHandlers.handleUpdateWithoutAgent,
    onCreateWithoutAgent: submitHandlers.handleCreateWithoutAgent,
    onCreateWithPlanMode: submitHandlers.handleCreateWithPlanMode,
    submitBlockedReason:
      props.submitBlockedReason ??
      pendingAttachmentUploadReason ??
      setup.savedBaseSubmitBlockedReason ??
      workflowAgentOverridesBlockedReason ??
      (computed.executorSourcePolicy?.incompatible ? computed.sourcePolicyReason : null),
    editDependenciesReady: setup.isEditMode ? setup.editDependencies.ready : undefined,
  };
}
