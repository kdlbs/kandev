"use client";

/* eslint-disable max-lines -- create and edit submit flows share one lifecycle boundary. */

import { useCallback, useRef, FormEvent } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { updateTask } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useToast } from "@/components/toast-provider";
import { linkToTask } from "@/lib/links";
import type { SubmitHandlersDeps } from "@/components/task-create-dialog-types";
import { t } from "@/lib/i18n";
import { useFreshBranchConsent } from "@/components/task-create-dialog-fresh-branch-consent";
import { queueTaskCreateLastUsedFromPayload } from "@/components/task-create-dialog-handlers";
import { ApiError } from "@/lib/api/client";
import { getTaskDependencyCycle } from "@/lib/api/domains/task-dependencies-api";
import { isTaskDependencyUpdateFailure } from "@/hooks/domains/task/use-task-edit-dialog-dependencies";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import { switchTaskRunner } from "@/lib/api/domains/task-runner-api";
import { WebSocketRequestError } from "@/lib/ws/client";
import type { TaskCreateRecovery } from "@/components/task-create-dialog-recovery";
import { useLiveDraftMatch } from "@/components/task-create-dialog-live-draft";
import {
  isStaleBranchPolicyError,
  useAcceptCreatedTask,
  useNativeCreateTransport,
} from "@/components/task-create-dialog-native-handoff";
import {
  useNativeSubmission,
  useRecoveryLookupActions,
  useRecoveryTaskOpen,
} from "@/components/task-create-dialog-native-recovery";
import {
  finishTaskCreateNavigation,
  shouldNavigateAfterTaskCreate,
} from "@/components/task-create-dialog-navigation";

const GENERIC_ERROR_KEY = "common:anErrorOccurred";

import {
  activatePlanMode,
  buildCreateTaskPayload,
  buildRepositoriesPayload,
  computeIsTaskStarted,
  findDuplicateRemoteRepo,
  findUnresolvedProviderRemote,
  validateCreateInputs,
  hasPendingAttachmentUploads,
  toMessageAttachments,
  RUNNER_INELIGIBLE_REASON_KEYS,
} from "@/components/task-create-dialog-helpers";
import { hasRegisteredRepositoryProviderCandidate } from "@/lib/plugins/repository-provider-url-resolution";

/**
 * Safe stand-in for callers that have no native recovery owner (custom
 * transports and focused submit tests). The native branch only runs when a
 * real owner is present, so these methods are never on a product path.
 */
const NO_RECOVERY: TaskCreateRecovery = {
  receipt: null,
  busy: false,
  storageUnavailable: false,
  beginOperation: () => true,
  endOperation: () => undefined,
  beginSubmission: () => null,
  markUncertain: () => undefined,
  markHandoffAttempted: () => undefined,
  acknowledge: () => undefined,
  retire: () => undefined,
  lookup: async () => ({ kind: "error" }),
  applyLookupResult: () => null,
  applyLookupMiss: () => null,
  applyCreateResult: () => null,
  updateConsent: (candidate) => candidate,
  hydrateTask: async () => null,
  hydrateSessionId: async () => null,
  isCurrent: () => false,
  isOwnerCurrent: () => true,
  getGeneration: () => -1,
  getReceipt: () => null,
};

function hasNativeRecovery(
  recovery: TaskCreateRecovery | undefined,
  createTask: SubmitHandlersDeps["createTask"],
): boolean {
  return Boolean(recovery) && !createTask;
}

function selectRecoveryOwner(
  recovery: TaskCreateRecovery | undefined,
  createTask: SubmitHandlersDeps["createTask"],
): TaskCreateRecovery {
  return recovery && !createTask ? recovery : NO_RECOVERY;
}

function computeFreshBranchActive(
  freshBranchEnabled: boolean,
  isLocalExecutor: boolean,
  useRemote: boolean,
  repositoryLocalPath: string,
): boolean {
  return freshBranchEnabled && isLocalExecutor && !useRemote && repositoryLocalPath !== "";
}

function buildFreshBranchPayloadFor(
  isActive: boolean,
  consentedDirtyFiles: string[],
): { confirmDiscard: boolean; consentedDirtyFiles: string[] } | undefined {
  return isActive ? { confirmDiscard: true, consentedDirtyFiles } : undefined;
}

function notifyQueuedTask(
  response: { queued_for_step_id?: string | null },
  notify: (input: { title: string; description: string }) => unknown,
) {
  if (!response.queued_for_step_id) return;
  notify({
    title: t("task:taskQueued"),
    description: t("task:taskQueuedWipLimit"),
  });
}

type NoAgentTaskRequirements = {
  description: string;
  workspaceId: string;
  workflowId: string;
};

function hasNoAgentTaskRequirements(input: {
  description: string;
  workspaceId: string | null;
  workflowId: string | null;
}): input is NoAgentTaskRequirements {
  return Boolean(input.description && input.workspaceId && input.workflowId);
}

function resolveWorkspacePath(noRepository: boolean, workspacePath: string): string | undefined {
  if (!noRepository) return undefined;
  return workspacePath.trim() || undefined;
}

const REPOSITORY_SELECTION_ERROR_KEYS: Record<string, string> = {
  repository_selection_invalid: "task:repositorySelectionInvalid",
  repository_selection_not_found: "task:repositorySelectionNotFound",
  repository_selection_unavailable: "task:repositorySelectionUnavailable",
};

/**
 * Wraps a rejected `task.runner` switch: the save issues no other call once
 * this throws, so `performTaskUpdate` never reaches `updateTask`.
 */
class RunnerSwitchRejectedError extends Error {
  constructor(readonly cause: unknown) {
    super("runner switch rejected");
  }
}

/**
 * Wraps a failure in the sequence AFTER a runner switch already committed:
 * the switch is not rolled back, so this exists to tell that state apart
 * from an ordinary save failure.
 */
class TaskUpdateAfterRunnerSwitchError extends Error {
  constructor(readonly cause: unknown) {
    super("task update failed after runner switch committed");
  }
}

/**
 * Wraps a session-launch failure that follows an already-committed task
 * save: the save is not rolled back, so this exists to tell that state
 * apart from an ordinary save failure and keep the dialog reporting the
 * truth (saved, but the agent didn't start) instead of a silent success.
 */
class LaunchAfterTaskUpdateError extends Error {
  constructor(readonly cause: unknown) {
    super("session launch failed after task update committed");
  }
}

// Maps a rejected task.runner switch to outcome-specific text: a typed
// mutability conflict reuses the same reason copy as the read-side
// projection; an untyped outcome (invalid, not-found,
// evaluation-unavailable) gets text for that class instead of the raw wire
// code.
function runnerSwitchErrorMessage(error: unknown): string {
  if (error instanceof WebSocketRequestError) {
    const errorCode =
      typeof error.details?.error_code === "string" ? error.details.error_code : undefined;
    if (errorCode === "target_cannot_materialize_repository") {
      return t("task:runnerConflictTargetCannotMaterializeRepository");
    }
    const reasonKey = errorCode ? RUNNER_INELIGIBLE_REASON_KEYS[errorCode] : undefined;
    if (reasonKey) return t(reasonKey);
    if (error.code === "NOT_FOUND") return t("task:runnerSwitchNotFound");
    if (error.code === "VALIDATION_ERROR") return t("task:runnerSwitchInvalid");
    if (error.code === "UNAVAILABLE") return t("task:runnerReasonEvaluationUnavailable");
  }
  return error instanceof Error ? error.message : t(GENERIC_ERROR_KEY);
}

export function taskSubmitErrorMessage(error: unknown): string {
  if (isTaskDependencyUpdateFailure(error)) {
    const cycle = getTaskDependencyCycle(error.cause);
    if (cycle?.length) return t("task:dependencyCycleError", { cycle: cycle.join(" -> ") });
    return t("task:dependencyUpdateFailed");
  }
  if (error instanceof RunnerSwitchRejectedError) return runnerSwitchErrorMessage(error.cause);
  if (error instanceof TaskUpdateAfterRunnerSwitchError)
    return t("task:runnerSwitchPartiallySaved");
  if (error instanceof LaunchAfterTaskUpdateError) return t("task:launchFailedAfterTaskSaved");
  if (error instanceof ApiError) {
    const key = REPOSITORY_SELECTION_ERROR_KEYS[error.errorCode ?? ""];
    if (key) return t(key);
  }
  return error instanceof Error ? error.message : t(GENERIC_ERROR_KEY);
}

type EditDependencySaveArgs = {
  editDependencies?: SubmitHandlersDeps["editDependencies"];
  updatedTask: Awaited<ReturnType<typeof updateTask>>;
  isStartedEdit: boolean;
  descriptionInputRef: SubmitHandlersDeps["descriptionInputRef"];
  setTaskName: SubmitHandlersDeps["setTaskName"];
  setHasDescription: SubmitHandlersDeps["setHasDescription"];
};

async function saveEditedTaskDependencies({
  editDependencies,
  updatedTask,
  isStartedEdit,
  descriptionInputRef,
  setTaskName,
  setHasDescription,
}: EditDependencySaveArgs): Promise<void> {
  if (!editDependencies?.isDirty) return;
  try {
    await editDependencies.save();
  } catch (error) {
    setTaskName(updatedTask.title);
    if (!isStartedEdit) {
      const confirmedDescription = updatedTask.description?.trim() ?? "";
      descriptionInputRef.current?.setValue(confirmedDescription);
      setHasDescription(confirmedDescription.length > 0);
    }
    throw error;
  }
}

function areEditDependenciesReady(
  isEditMode: boolean,
  editDependencies: SubmitHandlersDeps["editDependencies"],
): boolean {
  return !isEditMode || editDependencies?.ready !== false;
}

// Only a final selection that differs from the last confirmed stored runner
// counts as a user change. The baseline advances after a successful switch,
// so a retry can persist a deliberate change back to the previous profile.
function computeRunnerChanged(
  confirmedExecutorProfileId: string | null,
  executorProfileId: string,
): boolean {
  return (
    confirmedExecutorProfileId !== null &&
    executorProfileId !== "" &&
    executorProfileId !== confirmedExecutorProfileId
  );
}

// Issued first. A rejection here must leave every other field unsaved, so
// the caller never reaches the rest of the save sequence.
async function issueRunnerSwitchIfChanged(
  runnerChanged: boolean,
  taskId: string,
  executorProfileId: string,
  markRunnerConfirmed: (executorProfileId: string) => void,
): Promise<void> {
  if (!runnerChanged) return;
  try {
    await switchTaskRunner(taskId, executorProfileId);
    markRunnerConfirmed(executorProfileId);
  } catch (error) {
    throw new RunnerSwitchRejectedError(error);
  }
}

type SaveEditedTaskFieldsArgs = {
  editingTask: { id: string };
  updatePayload: Parameters<typeof updateTask>[1];
  trimmedDescription: string;
  runnerChanged: boolean;
  mcpServerIds: string[];
  mcpServerIdsDirty: boolean;
  saveTaskMCPSelections?: (definitionIds: string[]) => Promise<unknown>;
} & Omit<EditDependencySaveArgs, "updatedTask">;

// A runner switch that already committed is never rolled back; tag a
// failure here so the caller can report the true partial state instead of
// implying the whole save was rejected.
async function saveEditedTaskFields({
  editingTask,
  updatePayload,
  trimmedDescription,
  runnerChanged,
  mcpServerIds,
  mcpServerIdsDirty,
  saveTaskMCPSelections,
  ...dependencySaveArgs
}: SaveEditedTaskFieldsArgs) {
  try {
    const updatedTask = await updateTask(editingTask.id, updatePayload);
    await saveMCPSelectionIfNeeded({
      isStartedEdit: dependencySaveArgs.isStartedEdit,
      dirty: mcpServerIdsDirty,
      definitionIds: mcpServerIds,
      save: saveTaskMCPSelections,
    });
    await saveEditedTaskDependencies({ ...dependencySaveArgs, updatedTask });
    return { updatedTask, trimmedDescription };
  } catch (error) {
    if (runnerChanged) throw new TaskUpdateAfterRunnerSwitchError(error);
    throw error;
  }
}

async function saveMCPSelectionIfNeeded({
  isStartedEdit,
  dirty,
  definitionIds,
  save,
}: {
  isStartedEdit: boolean;
  dirty: boolean;
  definitionIds: string[];
  save?: (definitionIds: string[]) => Promise<unknown>;
}) {
  if (isStartedEdit || !dirty || !save) return;
  await save(definitionIds);
}

// eslint-disable-next-line max-lines-per-function -- one submit owner composes the edit and create lifecycle.
export function useTaskSubmitHandlers({
  isSessionMode,
  isEditMode,
  autoTitle = false,
  autopilot = false,
  isPassthroughProfile,
  taskName,
  workspaceId,
  workflowId,
  effectiveWorkflowId,
  repositories,
  repositoriesDirty,
  discoveredRepositories,
  workspaceRepositories,
  useRemote,
  remoteRepos,
  prInfoByUrl,
  agentProfileId,
  executorId,
  executorProfileId,
  seededExecutorProfileId,
  editingTask,
  onSuccess,
  onCreateSession,
  onOpenChange,
  createTask,
  refreshBranchPolicies,
  preserveTaskCreateLastUsedOnClose,
  taskId,
  parentTaskId,
  descriptionInputRef,
  setIsCreatingSession,
  setIsCreatingTask,
  setHasTitle,
  setHasDescription,
  setTaskName,
  setRepositories,
  setRemoteRepos,
  setAgentProfileId,
  setExecutorId,
  setMcpServerIds,
  setMcpServerIdsDirty,
  setSelectedWorkflowId,
  setFetchedSteps,
  setInitialWorkspaceLayout,
  clearDraft,
  freshBranchEnabled,
  isLocalExecutor,
  repositoryLocalPath,
  noRepository,
  workspacePath,
  priority,
  workflowAgentOverrides,
  workflowAgentOverridesBlockedReason,
  initialWorkspaceLayout,
  blockedBy,
  mcpServerIds = [],
  saveTaskMCPSelections,
  mcpServerIdsDirty,
  editDependencies,
  transformDescriptionBeforeSubmit,
  recovery,
}: SubmitHandlersDeps) {
  const router = useRouter();
  const autoFocusNewTasks = useAppStore((state) => state.userSettings.autoFocusNewTasks) !== false;
  const { toast } = useToast();
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);
  const applyAgentProfileRecentUse = useAppStore((state) => state.applyAgentProfileRecentUse);
  const isStartedEdit = computeIsTaskStarted(isEditMode, editingTask);
  const nativeRecoveryActive = hasNativeRecovery(recovery, createTask);
  const recoveryOwner = selectRecoveryOwner(recovery, createTask);
  const taskNameRef = useRef(taskName);
  taskNameRef.current = taskName;
  const seededRunnerRef = useRef(seededExecutorProfileId);
  const confirmedRunnerRef = useRef<string | null>(seededExecutorProfileId);
  if (seededRunnerRef.current !== seededExecutorProfileId) {
    seededRunnerRef.current = seededExecutorProfileId;
    confirmedRunnerRef.current = seededExecutorProfileId;
  }
  const markRunnerConfirmed = useCallback((profileId: string) => {
    confirmedRunnerRef.current = profileId;
  }, []);

  const isFreshBranchActive = computeFreshBranchActive(
    freshBranchEnabled,
    isLocalExecutor,
    useRemote,
    repositoryLocalPath,
  );
  const {
    pendingDiscard,
    ensureFreshBranchConsent,
    confirmForDirtyFiles,
    createTaskWithFreshBranchRetry,
  } = useFreshBranchConsent({
    isFreshBranchActive,
    workspaceId,
    repositoryLocalPath,
    toast,
    createTask,
  });

  const refreshStaleBranchPolicies = useCallback(
    async (error: unknown) => {
      if (!isStaleBranchPolicyError(error)) return false;
      if (refreshBranchPolicies) await refreshBranchPolicies().catch(() => undefined);
      return true;
    },
    [refreshBranchPolicies],
  );

  const buildFreshBranchPayload = (consentedDirtyFiles: string[]) =>
    buildFreshBranchPayloadFor(isFreshBranchActive, consentedDirtyFiles);

  const validateForCreate = useCallback(
    (trimmedTitle: string, trimmedDescription = "") =>
      validateCreateInputs({
        trimmedTitle,
        trimmedDescription,
        autoTitle,
        workspaceId,
        effectiveWorkflowId,
        repositories,
        remoteRepos: useRemote ? remoteRepos : undefined,
        agentProfileId,
        noRepository,
      }),
    [
      workspaceId,
      effectiveWorkflowId,
      repositories,
      useRemote,
      remoteRepos,
      agentProfileId,
      noRepository,
      autoTitle,
    ],
  );

  // Blocks submit when two Remote rows resolve to the same GitHub repo (same
  // PR URL twice, or two PRs of one repo). Surfaces a repo-named toast before
  // the backend round-trip so the user never sees the raw-UUID dedup error.
  // Returns true when a duplicate was found (caller should abort).
  const checkRemoteDuplicates = useCallback((): boolean => {
    if (!useRemote) return false;
    const duplicate = findDuplicateRemoteRepo(remoteRepos);
    if (!duplicate) return false;
    toast({
      title: t("task:duplicateRepository"),
      description: t("task:duplicateRepositoryDescription", { repository: duplicate }),
      variant: "error",
    });
    return true;
  }, [useRemote, remoteRepos, toast]);

  const checkRemoteResolution = useCallback((): boolean => {
    if (!useRemote) return false;
    const unresolved = findUnresolvedProviderRemote(remoteRepos, (url) => {
      if (!hasRegisteredRepositoryProviderCandidate(url)) return false;
      return (
        !prInfoByUrl.settled(url) ||
        Boolean(prInfoByUrl.inspection?.(url)) ||
        Boolean(prInfoByUrl.error(url))
      );
    });
    if (!unresolved) return false;
    const resolutionError = prInfoByUrl.error(unresolved.url);
    toast({
      title: t("task:repositoryStillBeingVerified"),
      description: resolutionError
        ? t("task:repositoryProviderVerificationFailed")
        : t("task:repositoryProviderVerificationPending"),
      variant: "error",
    });
    return true;
  }, [prInfoByUrl, remoteRepos, toast, useRemote]);

  const hasRemoteSubmitBlocker = useCallback(
    () => checkRemoteResolution() || checkRemoteDuplicates(),
    [checkRemoteDuplicates, checkRemoteResolution],
  );

  const resetForm = useCallback(() => {
    setHasTitle(false);
    setHasDescription(false);
    setTaskName("");
    setRepositories([]);
    setRemoteRepos([]);
    setAgentProfileId("");
    setExecutorId("");
    setSelectedWorkflowId(workflowId);
    setFetchedSteps(null);
    setMcpServerIds([]);
    setMcpServerIdsDirty(false);
    setInitialWorkspaceLayout("repository");
    // State setters are stable; only workflowId can change
  }, [
    workflowId,
    setHasTitle,
    setHasDescription,
    setTaskName,
    setRepositories,
    setRemoteRepos,
    setAgentProfileId,
    setExecutorId,
    setSelectedWorkflowId,
    setFetchedSteps,
    setMcpServerIds,
    setMcpServerIdsDirty,
    setInitialWorkspaceLayout,
  ]);

  const getRepositoriesPayload = useCallback(
    (consentedDirtyFiles: string[] = []) => {
      if (noRepository) return [];
      return buildRepositoriesPayload({
        useRemote,
        remoteRepos,
        prInfoByUrl,
        repositories,
        discoveredRepositories,
        workspaceRepositories,
        isLocalExecutor,
        freshBranch: buildFreshBranchPayload(consentedDirtyFiles),
      });
    },
    // buildFreshBranchPayload is a closure over current scope; dependencies stay explicit below.
    [
      noRepository,
      useRemote,
      remoteRepos,
      prInfoByUrl,
      repositories,
      discoveredRepositories,
      workspaceRepositories,
      isLocalExecutor,
      isFreshBranchActive,
    ],
  );

  const handleSessionSubmit = useCallback(async () => {
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
    const attachments = descriptionInputRef.current?.getAttachments() ?? [];
    if (hasPendingAttachmentUploads(attachments)) return;
    if (!agentProfileId) return;
    if (!trimmedDescription) return;

    if (onCreateSession) {
      onCreateSession({
        prompt: trimmedDescription,
        agentProfileId,
        executorId,
        mcpServerIds,
        attachments: toMessageAttachments(attachments),
      });
      onOpenChange(false);
      return;
    }

    if (!taskId) return;

    setIsCreatingSession(true);
    try {
      const { request } = buildStartRequest(taskId, agentProfileId, {
        executorId,
        executorProfileId: executorProfileId || undefined,
        prompt: trimmedDescription,
        attachments: toMessageAttachments(attachments),
        mcpServerIds,
      });
      const response = await launchSession(request);
      if (response.session_id) {
        recordAgentProfileRecentUseBestEffort(
          "task_session",
          response.agent_profile_id ?? agentProfileId,
          (record) => applyAgentProfileRecentUse("task_session", record),
        );
      }

      onOpenChange(false);
      router.push(linkToTask(taskId));
    } catch (error) {
      toast({
        title: t("task:failedToCreateSession"),
        description: taskSubmitErrorMessage(error),
        variant: "error",
      });
    } finally {
      setIsCreatingSession(false);
    }
  }, [
    agentProfileId,
    executorId,
    executorProfileId,
    onCreateSession,
    onOpenChange,
    router,
    taskId,
    toast,
    descriptionInputRef,
    setIsCreatingSession,
    applyAgentProfileRecentUse,
    mcpServerIds,
  ]);

  const performTaskUpdate = useCallback(async () => {
    if (!editingTask) return null;
    if (!areEditDependenciesReady(isEditMode, editDependencies)) return null;
    const trimmedTitle = taskName.trim();
    if (!trimmedTitle) return null;

    const runnerChanged = computeRunnerChanged(confirmedRunnerRef.current, executorProfileId);
    await issueRunnerSwitchIfChanged(
      runnerChanged,
      editingTask.id,
      executorProfileId,
      markRunnerConfirmed,
    );

    const description = isStartedEdit
      ? (editingTask.description ?? "")
      : (descriptionInputRef.current?.getValue() ?? "");
    const trimmedDescription = description.trim();
    const repositoriesPayload = !isStartedEdit && repositoriesDirty ? getRepositoriesPayload() : [];
    const titleChanged = trimmedTitle !== editingTask.title;

    const updatePayload: Parameters<typeof updateTask>[1] = {
      ...(titleChanged && { title: trimmedTitle }),
      ...(!isStartedEdit && { description: trimmedDescription }),
      ...(!isStartedEdit && repositoriesDirty && { repositories: repositoriesPayload }),
    };

    return saveEditedTaskFields({
      editingTask,
      updatePayload,
      trimmedDescription,
      runnerChanged,
      mcpServerIds,
      mcpServerIdsDirty,
      saveTaskMCPSelections,
      editDependencies,
      isStartedEdit,
      descriptionInputRef,
      setTaskName,
      setHasDescription,
    });
  }, [
    editingTask,
    taskName,
    descriptionInputRef,
    getRepositoriesPayload,
    isStartedEdit,
    repositoriesDirty,
    mcpServerIds,
    mcpServerIdsDirty,
    saveTaskMCPSelections,
    editDependencies,
    isEditMode,
    setTaskName,
    setHasDescription,
    executorProfileId,
    markRunnerConfirmed,
  ]);

  const handleEditSubmit = useCallback(async () => {
    if (!areEditDependenciesReady(isEditMode, editDependencies)) return;
    if (checkRemoteResolution()) return;
    setIsCreatingTask(true);
    let closeDialog = true;
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      const { updatedTask, trimmedDescription } = result;

      let taskSessionId: string | null = null;
      if (agentProfileId) {
        try {
          const { request } = buildStartRequest(updatedTask.id, agentProfileId, {
            executorId,
            executorProfileId: executorProfileId || undefined,
            prompt: trimmedDescription || "",
          });
          const response = await launchSession(request);
          taskSessionId = response?.session_id ?? null;
          if (taskSessionId) {
            recordAgentProfileRecentUseBestEffort(
              "task_session",
              response.agent_profile_id ?? agentProfileId,
              (record) => applyAgentProfileRecentUse("task_session", record),
            );
          }
        } catch (error) {
          // The task save already committed by this point (performTaskUpdate
          // resolved above); the launch is the last call in AC-004.4c's
          // ordered sequence, so its failure must be reported as a partial
          // save rather than swallowed into an apparent full success.
          throw new LaunchAfterTaskUpdateError(error);
        }
      }

      onSuccess?.(updatedTask, "edit", { taskSessionId });
    } catch (error) {
      closeDialog = false;
      await refreshStaleBranchPolicies(error);
      toast({
        title: t("task:failedToUpdateTask"),
        description: taskSubmitErrorMessage(error),
        variant: "error",
      });
    } finally {
      if (closeDialog) onOpenChange(false);
      setIsCreatingTask(false);
    }
  }, [
    performTaskUpdate,
    checkRemoteResolution,
    agentProfileId,
    executorId,
    executorProfileId,
    onSuccess,
    onOpenChange,
    refreshStaleBranchPolicies,
    mcpServerIdsDirty,
    saveTaskMCPSelections,
    toast,
    setIsCreatingTask,
    applyAgentProfileRecentUse,
    editDependencies,
    isEditMode,
  ]);

  const handleUpdateWithoutAgent = useCallback(async () => {
    if (!areEditDependenciesReady(isEditMode, editDependencies)) return;
    if (checkRemoteResolution()) return;
    setIsCreatingTask(true);
    let closeDialog = true;
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      onSuccess?.(result.updatedTask, "edit");
    } catch (error) {
      closeDialog = false;
      await refreshStaleBranchPolicies(error);
      toast({
        title: t("task:failedToUpdateTask"),
        description: taskSubmitErrorMessage(error),
        variant: "error",
      });
    } finally {
      if (closeDialog) onOpenChange(false);
      setIsCreatingTask(false);
    }
  }, [
    checkRemoteResolution,
    performTaskUpdate,
    onSuccess,
    onOpenChange,
    refreshStaleBranchPolicies,
    mcpServerIdsDirty,
    saveTaskMCPSelections,
    toast,
    setIsCreatingTask,
    editDependencies,
    isEditMode,
  ]);

  const submissionMatchesLiveDraft = useLiveDraftMatch({
    taskNameRef,
    descriptionInputRef,
  });
  const sendNativeCreate = useNativeCreateTransport({
    recovery: recoveryOwner,
    isFreshBranchActive,
    confirmForDirtyFiles,
    refreshStaleBranchPolicies,
  });
  const acceptCreatedTask = useAcceptCreatedTask({
    recovery: recoveryOwner,
    matchesLiveDraft: submissionMatchesLiveDraft,
    autoFocusNewTasks,
    isPassthroughProfile,
    onSuccess,
    onOpenChange,
    clearDraft,
    queueLastUsed: queueTaskCreateLastUsedFromPayload,
    preserveLastUsedOnClose: preserveTaskCreateLastUsedOnClose,
    activatePlan: (taskId, sessionId) =>
      activatePlanMode({
        sessionId,
        taskId,
        autoFocus: autoFocusNewTasks,
        setActiveDocument,
        setPlanMode,
        router,
      }),
    openPassthroughTask: (taskId) => router.push(linkToTask(taskId)),
  });
  const { applyNativeObservation, runNativeSubmission } = useNativeSubmission({
    recovery: recoveryOwner,
    acceptCreatedTask,
    sendNativeCreate,
    getDraftTitle: () => taskNameRef.current,
    errorMessage: taskSubmitErrorMessage,
    toast,
  });
  const { checkStatus: checkRecoveryStatus, retry: retryRecovery } = useRecoveryLookupActions({
    recovery: recoveryOwner,
    acceptCreatedTask,
    sendNativeCreate,
    applyObservation: applyNativeObservation,
    errorMessage: taskSubmitErrorMessage,
    toast,
  });
  const openRecoveryTask = useRecoveryTaskOpen({
    recovery: recoveryOwner,
    onOpenChange,
    router,
  });
  const closeRecovery = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);
  const runGuardedCreate = useCallback(
    async (operation: () => Promise<void>) => {
      if (!recoveryOwner.beginOperation()) return;
      try {
        await operation();
      } finally {
        recoveryOwner.endOperation();
      }
    },
    [recoveryOwner],
  );

  const performCreate = useCallback(
    async (opts: {
      trimmedTitle: string;
      trimmedDescription: string;
      consented: string[];
      withAgent: boolean;
      planMode?: boolean;
      attachments?: ReturnType<typeof toMessageAttachments>;
      generation: number;
    }) => {
      if (!isSessionMode && !isEditMode && workflowAgentOverridesBlockedReason) return;
      if (!workspaceId || !effectiveWorkflowId) return;
      if (!recoveryOwner.isOwnerCurrent(opts.generation)) return;
      let submittedPayload: ReturnType<typeof buildCreateTaskPayload> | null = null;
      const buildPayload = (c: string[]) => {
        const payload = buildCreateTaskPayload({
          workspaceId,
          effectiveWorkflowId,
          trimmedTitle: opts.trimmedTitle,
          trimmedDescription: opts.trimmedDescription,
          autoTitle,
          repositoriesPayload: getRepositoriesPayload(c),
          agentProfileId,
          executorId,
          executorProfileId,
          withAgent: opts.withAgent,
          planMode: opts.planMode,
          attachments: opts.attachments,
          parentId: parentTaskId,
          // Pass undefined (not "") for an empty trimmed path so the JSON
          // payload omits the key entirely — matches the noRepository=false
          // case and keeps "no path provided" semantically distinct from
          // "empty path string" on the wire.
          workspacePath: resolveWorkspacePath(noRepository, workspacePath),
          autopilot,
          priority,
          workflowAgentOverrides,
          initialWorkspaceLayout,
          blockedBy,
          mcpServerIds,
        });
        submittedPayload = payload;
        return payload;
      };
      if (nativeRecoveryActive) {
        await runNativeSubmission(buildPayload, opts.consented, opts.generation);
        return;
      }
      const taskResponse = await createTaskWithFreshBranchRetry(buildPayload, opts.consented);
      if (!taskResponse) return;
      notifyQueuedTask(taskResponse, toast);
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      const willNavigate =
        autoFocusNewTasks &&
        shouldNavigateAfterTaskCreate(
          opts.withAgent,
          isPassthroughProfile,
          opts.planMode,
          newSessionId,
        );
      onSuccess?.(taskResponse, "create", {
        taskSessionId: newSessionId,
        willNavigate,
        autoFocus: autoFocusNewTasks,
      });
      clearDraft();
      queueTaskCreateLastUsedFromPayload(submittedPayload);
      preserveTaskCreateLastUsedOnClose?.();
      onOpenChange(false);
      finishTaskCreateNavigation({
        planMode: opts.planMode,
        newSessionId,
        autoFocusNewTasks,
        withAgent: opts.withAgent,
        isPassthroughProfile,
        activatePlan: (sessionId) =>
          activatePlanMode({
            sessionId,
            taskId: taskResponse.id,
            autoFocus: autoFocusNewTasks,
            setActiveDocument,
            setPlanMode,
            router,
          }),
        openPassthroughTask: () => router.push(linkToTask(taskResponse.id)),
      });
    },
    [
      workspaceId,
      effectiveWorkflowId,
      autoFocusNewTasks,
      autoTitle,
      blockedBy,
      agentProfileId,
      executorId,
      executorProfileId,
      isPassthroughProfile,
      parentTaskId,
      autopilot,
      noRepository,
      workspacePath,
      priority,
      initialWorkspaceLayout,
      onSuccess,
      onOpenChange,
      preserveTaskCreateLastUsedOnClose,
      clearDraft,
      setActiveDocument,
      setPlanMode,
      router,
      getRepositoriesPayload,
      createTaskWithFreshBranchRetry,
      createTask,
      nativeRecoveryActive,
      runNativeSubmission,
      isSessionMode,
      isEditMode,
      workflowAgentOverrides,
      workflowAgentOverridesBlockedReason,
      mcpServerIds,
    ],
  );

  const handleCreatePlanMode = useCallback(
    (
      trimmedTitle: string,
      consented: string[],
      attachments: ReturnType<typeof toMessageAttachments> | undefined,
      generation: number,
    ) =>
      performCreate({
        trimmedTitle,
        trimmedDescription: "",
        consented,
        withAgent: false,
        planMode: true,
        attachments,
        generation,
      }),
    [performCreate],
  );

  const performEditWithPlanMode = useCallback(async () => {
    const result = await performTaskUpdate();
    if (!result) return;
    const { updatedTask, trimmedDescription } = result;
    const { request } = buildStartRequest(updatedTask.id, agentProfileId, {
      executorId,
      executorProfileId: executorProfileId || undefined,
      prompt: trimmedDescription || "",
      planMode: true,
    });
    const response = await launchSession(request);
    const newSessionId = response?.session_id ?? null;
    if (newSessionId) {
      recordAgentProfileRecentUseBestEffort(
        "task_session",
        response.agent_profile_id ?? agentProfileId,
        (record) => applyAgentProfileRecentUse("task_session", record),
      );
    }
    onSuccess?.(updatedTask, "edit", { taskSessionId: newSessionId });
    onOpenChange(false);
    if (newSessionId) {
      activatePlanMode({
        sessionId: newSessionId,
        taskId: updatedTask.id,
        setActiveDocument,
        setPlanMode,
        router,
      });
    }
  }, [
    performTaskUpdate,
    agentProfileId,
    executorId,
    executorProfileId,
    onSuccess,
    onOpenChange,
    setActiveDocument,
    setPlanMode,
    router,
    applyAgentProfileRecentUse,
  ]);

  const submitPlanModeCreate = useCallback(async () => {
    if (workflowAgentOverridesBlockedReason) return;
    await runGuardedCreate(async () => {
      const generation = recoveryOwner.getGeneration();
      const trimmedTitle = taskName.trim();
      const description = descriptionInputRef.current?.getValue() ?? "";
      const trimmedDescription = description.trim();
      const selectedAttachments = descriptionInputRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      const attachments = toMessageAttachments(selectedAttachments);
      if (!validateForCreate(trimmedTitle, trimmedDescription)) return;
      if (hasRemoteSubmitBlocker()) return;
      const consent = await ensureFreshBranchConsent();
      if (consent === null || !recoveryOwner.isOwnerCurrent(generation)) return;
      setIsCreatingTask(true);
      try {
        await performCreate({
          trimmedTitle,
          trimmedDescription,
          consented: consent,
          withAgent: true,
          planMode: true,
          attachments,
          generation,
        });
      } catch (error) {
        await refreshStaleBranchPolicies(error);
        toast({
          title: t("task:failedToStartTaskPlanMode"),
          description: taskSubmitErrorMessage(error),
          variant: "error",
        });
      } finally {
        setIsCreatingTask(false);
      }
    });
  }, [
    taskName,
    validateForCreate,
    hasRemoteSubmitBlocker,
    ensureFreshBranchConsent,
    performCreate,
    refreshStaleBranchPolicies,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
    workflowAgentOverridesBlockedReason,
    recoveryOwner,
    runGuardedCreate,
  ]);

  const handleCreateWithPlanMode = useCallback(async () => {
    if (isEditMode) {
      if (editDependencies && !editDependencies.ready) return;
      setIsCreatingTask(true);
      try {
        await performEditWithPlanMode();
      } catch (error) {
        await refreshStaleBranchPolicies(error);
        toast({
          title: t("task:failedToStartTaskPlanMode"),
          description: taskSubmitErrorMessage(error),
          variant: "error",
        });
      } finally {
        setIsCreatingTask(false);
      }
      return;
    }
    await submitPlanModeCreate();
  }, [
    isEditMode,
    performEditWithPlanMode,
    submitPlanModeCreate,
    refreshStaleBranchPolicies,
    toast,
    setIsCreatingTask,
    editDependencies,
  ]);

  const submitCreateTask = useCallback(
    async ({
      trimmedTitle,
      trimmedDescription,
      consent,
      attachments,
      generation,
    }: {
      trimmedTitle: string;
      trimmedDescription: string;
      consent: string[];
      attachments: ReturnType<typeof toMessageAttachments>;
      generation: number;
    }) => {
      if (!recoveryOwner.isOwnerCurrent(generation)) return;
      if (trimmedDescription) {
        const finalDescription = transformDescriptionBeforeSubmit
          ? await transformDescriptionBeforeSubmit(trimmedDescription)
          : trimmedDescription;
        if (!recoveryOwner.isOwnerCurrent(generation)) return;
        await performCreate({
          trimmedTitle,
          trimmedDescription: finalDescription,
          consented: consent,
          withAgent: true,
          attachments,
          generation,
        });
        return;
      }
      if (!autoTitle) {
        await handleCreatePlanMode(trimmedTitle, consent, attachments, generation);
      }
    },
    [
      autoTitle,
      handleCreatePlanMode,
      performCreate,
      recoveryOwner,
      transformDescriptionBeforeSubmit,
    ],
  );

  const handleCreateSubmit = useCallback(async () => {
    if (workflowAgentOverridesBlockedReason) return;
    await runGuardedCreate(async () => {
      const generation = recoveryOwner.getGeneration();
      const trimmedTitle = taskName.trim();
      const description = descriptionInputRef.current?.getValue() ?? "";
      const trimmedDescription = description.trim();
      const selectedAttachments = descriptionInputRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      const attachments = toMessageAttachments(selectedAttachments);
      if (!validateForCreate(trimmedTitle, trimmedDescription)) return;
      if (hasRemoteSubmitBlocker()) return;
      const consent = await ensureFreshBranchConsent();
      if (consent === null || !recoveryOwner.isOwnerCurrent(generation)) return;
      setIsCreatingTask(true);
      try {
        await submitCreateTask({
          trimmedTitle,
          trimmedDescription,
          consent,
          attachments,
          generation,
        });
      } catch (error) {
        await refreshStaleBranchPolicies(error);
        toast({
          title: t("task:failedToCreateTask"),
          description: taskSubmitErrorMessage(error),
          variant: "error",
        });
      } finally {
        setIsCreatingTask(false);
      }
    });
  }, [
    taskName,
    validateForCreate,
    hasRemoteSubmitBlocker,
    ensureFreshBranchConsent,
    submitCreateTask,
    refreshStaleBranchPolicies,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
    workflowAgentOverridesBlockedReason,
    recovery,
    runGuardedCreate,
  ]);

  const submitNoAgentCreate = useCallback(
    async ({
      trimmedTitle,
      trimmedDescription,
      consent,
      attachments,
      generation,
    }: {
      trimmedTitle: string;
      trimmedDescription: string;
      consent: string[];
      attachments: ReturnType<typeof toMessageAttachments>;
      generation: number;
    }) => {
      if (!recoveryOwner.isOwnerCurrent(generation)) return;
      setIsCreatingTask(true);
      try {
        let submittedPayload: ReturnType<typeof buildCreateTaskPayload> | null = null;
        const buildPayload = (c: string[]) => {
          const p = buildCreateTaskPayload({
            workspaceId: workspaceId as string,
            effectiveWorkflowId: effectiveWorkflowId as string,
            trimmedTitle,
            trimmedDescription,
            autoTitle,
            repositoriesPayload: getRepositoriesPayload(c),
            agentProfileId,
            executorId,
            executorProfileId,
            withAgent: false,
            attachments,
            workspacePath: resolveWorkspacePath(noRepository, workspacePath),
            autopilot,
            priority,
            workflowAgentOverrides,
            initialWorkspaceLayout,
            blockedBy,
            mcpServerIds,
          });
          submittedPayload = p;
          return p;
        };
        if (nativeRecoveryActive) {
          await runNativeSubmission(buildPayload, consent, generation);
          return;
        }
        const taskResponse = await createTaskWithFreshBranchRetry(buildPayload, consent);
        if (!taskResponse) return;
        notifyQueuedTask(taskResponse, toast);
        onSuccess?.(taskResponse, "create", { autoFocus: autoFocusNewTasks });
        clearDraft();
        queueTaskCreateLastUsedFromPayload(submittedPayload);
        preserveTaskCreateLastUsedOnClose?.();
        onOpenChange(false);
      } catch (error) {
        await refreshStaleBranchPolicies(error);
        toast({
          title: t("task:failedToCreateTask"),
          description: taskSubmitErrorMessage(error),
          variant: "error",
        });
      } finally {
        setIsCreatingTask(false);
      }
    },
    [
      autoFocusNewTasks,
      autoTitle,
      workspaceId,
      effectiveWorkflowId,
      agentProfileId,
      executorId,
      executorProfileId,
      noRepository,
      autopilot,
      workspacePath,
      priority,
      getRepositoriesPayload,
      createTaskWithFreshBranchRetry,
      createTask,
      runNativeSubmission,
      refreshStaleBranchPolicies,
      onSuccess,
      onOpenChange,
      preserveTaskCreateLastUsedOnClose,
      clearDraft,
      toast,
      setIsCreatingTask,
      blockedBy,
      workflowAgentOverrides,
      mcpServerIds,
      initialWorkspaceLayout,
      nativeRecoveryActive,
    ],
  );

  const handleCreateWithoutAgent = useCallback(async () => {
    if (workflowAgentOverridesBlockedReason) return;
    await runGuardedCreate(async () => {
      const generation = recoveryOwner.getGeneration();
      const trimmedTitle = taskName.trim();
      const trimmedDescription = (descriptionInputRef.current?.getValue() ?? "").trim();
      const selectedAttachments = descriptionInputRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      const attachments = toMessageAttachments(selectedAttachments);
      if (!validateForCreate(trimmedTitle, trimmedDescription)) return;
      const requirements = {
        description: trimmedDescription,
        workspaceId,
        workflowId: effectiveWorkflowId,
      };
      if (!hasNoAgentTaskRequirements(requirements)) return;
      if (hasRemoteSubmitBlocker()) return;

      const consent = await ensureFreshBranchConsent();
      if (consent === null || !recoveryOwner.isOwnerCurrent(generation)) return;
      await submitNoAgentCreate({
        trimmedTitle,
        trimmedDescription,
        consent,
        attachments,
        generation,
      });
    });
  }, [
    taskName,
    workspaceId,
    effectiveWorkflowId,
    validateForCreate,
    hasRemoteSubmitBlocker,
    ensureFreshBranchConsent,
    submitNoAgentCreate,
    toast,
    descriptionInputRef,
    workflowAgentOverridesBlockedReason,
    recoveryOwner,
    runGuardedCreate,
  ]);

  const editSubmitHandler = isStartedEdit ? handleUpdateWithoutAgent : handleEditSubmit;
  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (isSessionMode) return handleSessionSubmit();
      if (isEditMode) return editSubmitHandler();
      return handleCreateSubmit();
    },
    [isSessionMode, isEditMode, handleSessionSubmit, editSubmitHandler, handleCreateSubmit],
  );

  const handleCancel = useCallback(() => {
    resetForm();
    onOpenChange(false);
  }, [resetForm, onOpenChange]);

  const recoveryReceipt = recoveryOwner.receipt;
  return {
    resetForm,
    handleSubmit,
    handleUpdateWithoutAgent,
    handleCreateWithoutAgent,
    handleCreateWithPlanMode,
    handleCancel,
    pendingDiscard,
    recovery: {
      receipt: recoveryReceipt,
      busy: recoveryOwner.busy,
      storageUnavailable: recoveryOwner.storageUnavailable,
      checkStatus: checkRecoveryStatus,
      retry: retryRecovery,
      openTask: openRecoveryTask,
      close: closeRecovery,
      canRetry: Boolean(
        recoveryReceipt?.replayable &&
        recoveryReceipt.state === "unknown" &&
        !recoveryReceipt.taskId,
      ),
      canOpen: Boolean(recoveryReceipt?.taskId && recoveryReceipt.state !== "unavailable"),
      canCheck: Boolean(
        recoveryReceipt &&
        recoveryReceipt.state !== "accepted" &&
        recoveryReceipt.state !== "submitting",
      ),
    },
  };
}
