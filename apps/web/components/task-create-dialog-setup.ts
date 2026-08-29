"use client";

import { FormEvent, useCallback, useState } from "react";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import type { Repository } from "@/lib/types/http";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { usePromptResultDelivery } from "@/hooks/use-prompt-result-delivery";
import { useTaskSubmitHandlers } from "@/components/task-create-dialog-submit";
import { useToast } from "@/components/toast-provider";
import { useRepositorySets } from "@/hooks/domains/workspace/use-repository-sets";
import { useApplyRepositorySet } from "@/components/task-create-dialog-repository-sets-apply";
import { selectedRepositoryIdsForSet } from "@/components/task-create-dialog-repository-sets";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  useDialogFormState,
  useTaskCreateDialogEffects,
  useDialogHandlers,
  useLockedFieldSync,
  useSessionRepoName,
  useTaskCreateDialogData,
  computeIsTaskStarted,
  type DialogFormState,
} from "@/components/task-create-dialog-state";
import type { TaskCreateDialogProps } from "@/components/task-create-dialog";
import { useResolvedTaskCreateWorkflowContext } from "@/components/task-create-dialog-workflow-context";
import { truncateRemoteTaskTitle } from "@/lib/task-title";
import { t } from "@/lib/i18n";
import { listRepositoryBranchPolicies } from "@/lib/api";

// Catalog key: module scope, so it is resolved at the call site.
const PROMPT_INSERTED_MESSAGE_KEY = "task:enhancedPromptInserted";

function useEnhanceForDialog(
  fs: DialogFormState,
  taskId: string | null | undefined,
  open: boolean,
) {
  const isConfigured = useIsUtilityConfigured();
  const { toast } = useToast();
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId: null,
    taskTitle: fs.taskName,
  });
  const applyDescription = useCallback(
    (value: string) => {
      const input = fs.descriptionInputRef.current;
      if (!input) return false;
      input.setValue(value);
      const applied = input.getValue() === value;
      if (applied) fs.setHasDescription(value.trim().length > 0);
      return applied;
    },
    [fs],
  );
  const promptDelivery = usePromptResultDelivery({
    scopeKey: `task-create:${open}:${fs.openCycle}:${taskId ?? ""}`,
    getCurrent: () => fs.descriptionInputRef.current?.getValue() ?? null,
    apply: applyDescription,
  });
  const onEnhance = useCallback(() => {
    const current = fs.descriptionInputRef.current?.getValue() ?? "";
    if (!current.trim()) return;
    const generation = promptDelivery.captureScope();
    void enhancePrompt(current, (result) => {
      const inserted = promptDelivery.deliver(current, result, generation);
      if (inserted) toast({ description: t(PROMPT_INSERTED_MESSAGE_KEY), variant: "success" });
      return inserted;
    });
  }, [enhancePrompt, fs.descriptionInputRef, promptDelivery, toast]);
  return {
    onEnhance,
    isLoading: isEnhancingPrompt,
    isConfigured,
    pendingResult: promptDelivery.pendingResult,
    onApplyPending: promptDelivery.applyPending,
    onCopyPending: promptDelivery.copyPending,
  };
}

function useJiraImportHandler(fs: DialogFormState, handleTaskNameChange: (value: string) => void) {
  return useCallback(
    (ticket: JiraTicket) => {
      handleTaskNameChange(truncateRemoteTaskTitle(`[${ticket.key}] ${ticket.summary}`));
      const description = ticket.description?.trim()
        ? `${ticket.description}\n\n---\nJira: ${ticket.url}`
        : `Jira: ${ticket.url}`;
      fs.descriptionInputRef.current?.setValue(description);
      fs.setHasDescription(true);
    },
    [fs, handleTaskNameChange],
  );
}

function useLinearImportHandler(
  fs: DialogFormState,
  handleTaskNameChange: (value: string) => void,
) {
  return useCallback(
    (issue: LinearIssue) => {
      handleTaskNameChange(truncateRemoteTaskTitle(`[${issue.identifier}] ${issue.title}`));
      const description = issue.description?.trim()
        ? `${issue.description}\n\n---\nLinear: ${issue.url}`
        : `Linear: ${issue.url}`;
      fs.descriptionInputRef.current?.setValue(description);
      fs.setHasDescription(true);
    },
    [fs, handleTaskNameChange],
  );
}

type SubmitWiringArgs = {
  props: TaskCreateDialogProps;
  fs: ReturnType<typeof useDialogFormState>;
  computed: ReturnType<typeof useTaskCreateDialogData>["computed"];
  workspaceRepositories: ReturnType<typeof useTaskCreateDialogData>["repositories"];
  repositoryLocalPath: string;
  isSessionMode: boolean;
  isEditMode: boolean;
  autoTitle: boolean;
  refreshBranchPolicies: () => Promise<void>;
  preserveQueuedLastUsedOnClose: () => void;
};

function useSubmitHandlersWiring({
  props,
  fs,
  computed,
  workspaceRepositories,
  repositoryLocalPath,
  isSessionMode,
  isEditMode,
  autoTitle,
  refreshBranchPolicies,
  preserveQueuedLastUsedOnClose,
}: SubmitWiringArgs) {
  const {
    workspaceId,
    workflowId,
    editingTask,
    onSuccess,
    onCreateSession,
    onOpenChange,
    createTask,
  } = props;
  const { parentTaskId } = props;
  const taskId = props.taskId ?? null;
  return useTaskSubmitHandlers({
    isSessionMode,
    isEditMode,
    autoTitle,
    autopilot: fs.autopilot,
    isPassthroughProfile: computed.isPassthroughProfile,
    taskName: fs.taskName,
    workspaceId,
    workflowId,
    effectiveWorkflowId: computed.effectiveWorkflowId,
    repositories: fs.repositories,
    repositoriesDirty: fs.repositoriesDirty,
    discoveredRepositories: fs.discoveredRepositories,
    workspaceRepositories,
    useRemote: fs.useRemote,
    remoteRepos: fs.remoteRepos,
    prInfoByUrl: fs.prInfoByUrl,
    agentProfileId: computed.effectiveAgentProfileId,
    executorId: fs.executorId,
    executorProfileId: fs.executorProfileId,
    editingTask,
    onSuccess,
    onCreateSession,
    onOpenChange,
    createTask,
    refreshBranchPolicies,
    preserveTaskCreateLastUsedOnClose: preserveQueuedLastUsedOnClose,
    taskId,
    parentTaskId,
    descriptionInputRef: fs.descriptionInputRef,
    setIsCreatingSession: fs.setIsCreatingSession,
    setIsCreatingTask: fs.setIsCreatingTask,
    setHasTitle: fs.setHasTitle,
    setHasDescription: fs.setHasDescription,
    setTaskName: fs.setTaskName,
    setRepositories: fs.setRepositories,
    setRemoteRepos: fs.setRemoteRepos,
    setAgentProfileId: fs.setAgentProfileId,
    setExecutorId: fs.setExecutorId,
    setSelectedWorkflowId: fs.setSelectedWorkflowId,
    setFetchedSteps: fs.setFetchedSteps,
    clearDraft: fs.clearDraft,
    freshBranchEnabled: fs.freshBranchEnabled,
    isLocalExecutor: computed.isLocalExecutor,
    repositoryLocalPath,
    noRepository: fs.noRepository,
    workspacePath: fs.workspacePath,
    blockedBy: fs.blockedBy,
  });
}

function resolveSingleRowLocalPath(fs: DialogFormState, repositories: Repository[]): string {
  if (fs.repositories.length !== 1) return "";
  const row = fs.repositories[0];
  if (row.localPath) return row.localPath;
  if (row.repositoryId)
    return repositories.find((r) => r.id === row.repositoryId)?.local_path ?? "";
  return "";
}

function useDialogSetupData(
  props: TaskCreateDialogProps,
  fs: ReturnType<typeof useDialogFormState>,
) {
  const { open, workspaceId, workflowId, defaultStepId, initialValues } = props;
  const { toast } = useToast();
  const storeApi = useAppStoreApi();
  const upsertWorkspaceRepository = useAppStore((state) => state.upsertRepository);
  const setRepositoryBranchPolicies = useAppStore((state) => state.setRepositoryBranchPolicies);
  const setRepositoryBranchPoliciesLoading = useAppStore(
    (state) => state.setRepositoryBranchPoliciesLoading,
  );
  const data = useTaskCreateDialogData({
    open,
    workspaceId,
    workflowId,
    defaultStepId,
    fs,
    lockedWorkflow: props.lockedFields?.workflow === true,
    agentProfileRecentUseContext:
      props.mode === "session" || props.mode === "edit" ? "task_session" : "task_create",
  });
  const {
    workflows,
    agentProfiles,
    executors,
    repositories,
    repositoriesLoading,
    taskCreateLastUsed,
    userSettingsLoaded,
    computed,
  } = data;
  useTaskCreateDialogEffects(fs, {
    open,
    workspaceId,
    workflowId,
    effectiveWorkflowId: computed.effectiveWorkflowId,
    repositories,
    repositoriesLoading,
    agentProfiles,
    compatibleAgentProfiles: computed.compatibleAgentProfiles,
    authLoaded: computed.authLoaded,
    executors,
    workspaceDefaults: computed.workspaceDefaults,
    toast,
    workflows,
    isLocalExecutor: computed.isLocalExecutor,
    lastUsedRepositoryId: taskCreateLastUsed.repositoryId,
    userSettingsLoaded,
    lastUsedAgentProfileId: taskCreateLastUsed.agentProfileId,
    lastUsedExecutorProfileId: taskCreateLastUsed.executorProfileId,
    lastUsedBranch: taskCreateLastUsed.branch,
    preserveBranch: initialValues?.checkoutBranch || initialValues?.branch,
  });
  useLockedFieldSync(open, workflowId, initialValues, fs, props.lockedFields?.workflow === true);
  const handlers = useDialogHandlers(fs, repositories, {
    workspaceId,
    executors,
    upsertWorkspaceRepository,
  });
  const refreshBranchPolicies = useCallback(async () => {
    const repositoryIds = [
      ...new Set(
        fs.repositories
          .map((row) => row.repositoryId)
          .filter((repositoryId): repositoryId is string => Boolean(repositoryId)),
      ),
    ];
    await Promise.all(
      repositoryIds.map(async (repositoryId) => {
        const requestRevision =
          storeApi.getState().repositoryBranchPolicies.revisionByRepositoryId[repositoryId] ?? 0;
        setRepositoryBranchPoliciesLoading(repositoryId, true);
        try {
          const response = await listRepositoryBranchPolicies(repositoryId, { cache: "no-store" });
          setRepositoryBranchPolicies(
            repositoryId,
            response.repository_branch_policies,
            requestRevision,
          );
        } catch {
          // Keep the original task error visible when recovery cannot refresh.
        } finally {
          setRepositoryBranchPoliciesLoading(repositoryId, false);
        }
      }),
    );
  }, [fs.repositories, setRepositoryBranchPolicies, setRepositoryBranchPoliciesLoading, storeApi]);
  return {
    ...data,
    handlers,
    refreshBranchPolicies,
    repositoryLocalPath: resolveSingleRowLocalPath(fs, repositories),
  };
}

export function useTaskCreateDialogSetup(
  props: TaskCreateDialogProps,
  options: { preserveQueuedLastUsedOnClose?: () => void } = {},
) {
  const resolvedProps = useResolvedTaskCreateWorkflowContext(props);
  const { open, mode = "create", workspaceId, workflowId } = resolvedProps;
  const { editingTask, initialValues } = resolvedProps;
  const isSessionMode = mode === "session";
  const isEditMode = mode === "edit";
  const isTaskStarted = computeIsTaskStarted(isEditMode, editingTask);
  const agentGeneratedTaskTitles = useAppStore(
    (state) => state.userSettings.agentGeneratedTaskTitles,
  );
  const autoTitle = mode === "create" && agentGeneratedTaskTitles;
  const fs = useDialogFormState(
    open,
    workspaceId,
    workflowId,
    initialValues,
    resolvedProps.lockedFields?.workflow === true,
  );
  const sessionRepoName = useSessionRepoName(isSessionMode);
  const data = useDialogSetupData(resolvedProps, fs);
  const {
    workflows,
    agentProfiles,
    snapshots,
    repositories,
    repositoriesLoading,
    refreshRepositories,
    taskCreateLastUsed,
    userSettingsLoaded,
    computed,
    handlers,
    repositoryLocalPath,
    refreshBranchPolicies,
  } = data;
  const submitHandlers = useSubmitHandlersWiring({
    props: resolvedProps,
    fs,
    computed,
    workspaceRepositories: repositories,
    repositoryLocalPath,
    isSessionMode,
    isEditMode,
    autoTitle,
    refreshBranchPolicies,
    preserveQueuedLastUsedOnClose: options.preserveQueuedLastUsedOnClose ?? (() => undefined),
  });
  const guardedHandleSubmit = useGuardedSubmit(
    submitHandlers.handleSubmit,
    resolvedProps.submitBlockedReason,
  );
  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    guardedHandleSubmit(event as unknown as FormEvent);
  });
  const enhance = useEnhanceForDialog(fs, resolvedProps.taskId, resolvedProps.open);
  const handleJiraImport = useJiraImportHandler(fs, data.handlers.handleTaskNameChange);
  const handleLinearImport = useLinearImportHandler(fs, data.handlers.handleTaskNameChange);
  const freshBranchAvailable =
    !fs.useRemote && computed.isLocalExecutor && fs.repositories.length === 1;
  const repositorySets = useRepositorySetsForDialog({
    workspaceId: resolvedProps.workspaceId ?? null,
    open: resolvedProps.open,
    rows: fs.repositories,
    repositories,
    setRepositories: fs.setRepositories,
    setRepositoriesDirty: fs.setRepositoriesDirty,
    userSettingsLoaded,
  });
  return {
    fs,
    isSessionMode,
    isEditMode,
    isCreateMode: mode === "create",
    autoTitle,
    isTaskStarted,
    sessionRepoName,
    workflows,
    agentProfiles,
    snapshots,
    repositories,
    repositoriesLoading,
    refreshRepositories,
    computed,
    handlers,
    submitHandlers,
    handleKeyDown,
    freshBranchAvailable,
    repositorySets,
    taskCreateLastUsed,
    userSettingsLoaded,
    guardedHandleSubmit,
    enhance,
    handleJiraImport,
    handleLinearImport,
  };
}

type RepositorySetsForDialogArgs = {
  workspaceId: string | null;
  open: boolean;
  rows: DialogFormState["repositories"];
  repositories: Repository[];
  setRepositories: DialogFormState["setRepositories"];
  setRepositoriesDirty: DialogFormState["setRepositoriesDirty"];
  userSettingsLoaded: boolean;
};

/**
 * Assembles the repository-set props the picker needs: the workspace's sets, why
 * applying one is unavailable, and the apply handler.
 *
 * Gated on `userSettingsLoaded` because the repository auto-select effect writes
 * rows again once user settings arrive; offering the control before then lets a
 * user apply a set that autopick immediately overwrites.
 */
function useRepositorySetsForDialog({
  workspaceId,
  open,
  rows,
  repositories,
  setRepositories,
  setRepositoriesDirty,
  userSettingsLoaded,
}: RepositorySetsForDialogArgs) {
  const { sets } = useRepositorySets(workspaceId, open);
  const onApply = useApplyRepositorySet({
    rows,
    repositories,
    setRepositories,
    setRepositoriesDirty,
  });
  const [saveOpen, setSaveOpen] = useState(false);
  // Offer "Save as set" only when there is a workspace-repository selection worth
  // saving, so the action is never a dead end.
  const canSave = Boolean(workspaceId) && selectedRepositoryIdsForSet(rows).length > 0;
  if (!userSettingsLoaded) return undefined;
  return {
    sets,
    onApply,
    save:
      canSave && workspaceId ? { workspaceId, rows, open: saveOpen, setOpen: setSaveOpen } : null,
  };
}

function useGuardedSubmit(
  handleSubmit: (e: FormEvent) => void,
  blockedReason: string | null | undefined,
) {
  const blocked = Boolean(blockedReason);
  return useCallback(
    (e: FormEvent) => {
      if (blocked) e.preventDefault();
      else handleSubmit(e);
    },
    [blocked, handleSubmit],
  );
}
