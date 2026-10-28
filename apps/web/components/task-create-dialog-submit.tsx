"use client";

/* eslint-disable max-lines -- create and edit submit flows share one lifecycle boundary. */

import { useCallback, useRef, FormEvent } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { updateTask, createTask as createTaskNative } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useToast } from "@/components/toast-provider";
import { linkToTask } from "@/lib/links";
import type { SubmitHandlersDeps } from "@/components/task-create-dialog-types";
import { t } from "@/lib/i18n";
import {
  dirtyFilesFromApiError,
  useFreshBranchConsent,
} from "@/components/task-create-dialog-fresh-branch-consent";
import { queueTaskCreateLastUsedFromPayload } from "@/components/task-create-dialog-handlers";
import { ApiError } from "@/lib/api/client";
import { getTaskDependencyCycle } from "@/lib/api/domains/task-dependencies-api";
import { isTaskDependencyUpdateFailure } from "@/hooks/domains/task/use-task-edit-dialog-dependencies";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import { switchTaskRunner } from "@/lib/api/domains/task-runner-api";
import { WebSocketRequestError } from "@/lib/ws/client";
import type { CreateTaskResponse, Task } from "@/lib/types/http";
import { decodeNativeCreateResponse, type TaskCreationReceipt } from "@/lib/task-creation-receipt";
import type { TaskCreateRecovery } from "@/components/task-create-dialog-recovery";

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
  isOwnerCurrent: () => false,
  getGeneration: () => -1,
  getReceipt: () => null,
};

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

function shouldNavigateAfterTaskCreate(
  withAgent: boolean,
  isPassthroughProfile: boolean,
  planMode: boolean | undefined,
  sessionId: string | null,
) {
  return (withAgent && isPassthroughProfile) || !!(planMode && sessionId);
}

function finishTaskCreateNavigation(args: {
  planMode: boolean | undefined;
  newSessionId: string | null;
  autoFocusNewTasks: boolean;
  withAgent: boolean;
  isPassthroughProfile: boolean;
  activatePlan: (sessionId: string) => void;
  openPassthroughTask: () => void;
}) {
  if (args.planMode && args.newSessionId) {
    args.activatePlan(args.newSessionId);
  } else if (args.autoFocusNewTasks && args.withAgent && args.isPassthroughProfile) {
    args.openPassthroughTask();
  }
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

function isStaleBranchPolicyError(error: unknown): boolean {
  return (
    error instanceof ApiError && error.status === 400 && error.errorCode === "branch_policy_stale"
  );
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

function isDefinitiveCreateRejection(error: unknown, receipt: TaskCreationReceipt): boolean {
  if (!(error instanceof ApiError)) return false;
  // A later rejection cannot prove an earlier uncertain attempt stopped, so
  // it can never retire that submission's identity.
  if (receipt.everUncertain || receipt.taskId) return false;
  const body = error.body as { creation_rejected?: unknown } | null;
  return body?.creation_rejected === true;
}

type ComparableAttachment = {
  type?: string;
  attachment_id?: string;
  name?: string;
  mime_type: string;
  size_bytes?: number;
  data?: string;
  delivery_mode?: string;
};

function attachmentSignature(attachments?: ComparableAttachment[]): string {
  return (attachments ?? [])
    .map((attachment) =>
      [
        attachment.attachment_id ?? "inline",
        attachment.name ?? "",
        attachment.mime_type,
        String(attachment.size_bytes ?? 0),
        attachment.data ? "data" : "descriptor",
        attachment.delivery_mode ?? "",
      ].join("\u0000"),
    )
    .join("\u0001");
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

// eslint-disable-next-line max-lines-per-function, complexity -- one submit owner composes edit, create, and native recovery branches.
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
  const recoveryOwner = recovery ?? NO_RECOVERY;
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

  const isFreshBranchActive =
    freshBranchEnabled && isLocalExecutor && !useRemote && repositoryLocalPath !== "";
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
    isFreshBranchActive ? { confirmDiscard: true, consentedDirtyFiles } : undefined;

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

  const submissionMatchesLiveDraft = useCallback(
    (candidate: TaskCreationReceipt) => {
      const submittedTitle =
        typeof (candidate.payload as { title?: unknown }).title === "string"
          ? ((candidate.payload as { title: string }).title ?? "").trim()
          : "";
      if (taskNameRef.current.trim() !== submittedTitle) return false;
      const currentDescription = (descriptionInputRef.current?.getValue() ?? "").trim();
      if (currentDescription !== (candidate.payload.description ?? "").trim()) return false;
      const currentAttachments = toMessageAttachments(
        descriptionInputRef.current?.getAttachments() ?? [],
      );
      return (
        attachmentSignature(currentAttachments) ===
        attachmentSignature(candidate.payload.attachments)
      );
    },
    [descriptionInputRef],
  );

  /* eslint-disable complexity, sonarjs/cognitive-complexity -- accepted handoff rechecks owner and live draft before each side effect. */
  const acceptCreatedTask = useCallback(
    async (
      candidate: TaskCreationReceipt,
      task: Task | CreateTaskResponse | null,
      generation: number,
    ) => {
      if (candidate.state !== "accepted" || !recoveryOwner.isCurrent(candidate, generation)) {
        return;
      }
      // A persisted or previously failed handoff has already recorded the
      // task. Reopen it instead of replaying creation callbacks.
      if (candidate.handoffAttempted) return;
      // A newer draft stays open with Open task. Do not run creation
      // callbacks that close or navigate this dialog.
      if (!submissionMatchesLiveDraft(candidate)) return;
      const withAgent = candidate.payload.start_agent === true;
      const planMode = candidate.payload.plan_mode === true;
      let newSessionId = candidate.taskSessionId;
      if (!newSessionId && (planMode || (withAgent && isPassthroughProfile))) {
        newSessionId = await recoveryOwner.hydrateSessionId(candidate);
        if (!recoveryOwner.isCurrent(candidate, generation)) return;
      }
      const willNavigate =
        autoFocusNewTasks &&
        shouldNavigateAfterTaskCreate(withAgent, isPassthroughProfile, planMode, newSessionId);
      const response = task ?? (await recoveryOwner.hydrateTask(candidate));
      if (!recoveryOwner.isCurrent(candidate, generation)) return;
      if (!submissionMatchesLiveDraft(candidate)) return;
      const taskId = candidate.taskId;
      if (!response || !taskId) return;
      recoveryOwner.markHandoffAttempted(candidate);
      try {
        onSuccess?.(response, "create", {
          taskSessionId: newSessionId,
          willNavigate,
          autoFocus: autoFocusNewTasks,
        });
      } catch {
        // The task identity is already recorded. Keep the dialog open so the
        // operator can open that task instead of submitting again.
        return;
      }
      if (!recoveryOwner.isCurrent(candidate, generation)) return;
      if (!submissionMatchesLiveDraft(candidate)) return;
      queueTaskCreateLastUsedFromPayload(candidate.payload);
      preserveTaskCreateLastUsedOnClose?.();
      clearDraft();
      onOpenChange(false);
      finishTaskCreateNavigation({
        planMode,
        newSessionId,
        autoFocusNewTasks,
        withAgent,
        isPassthroughProfile,
        activatePlan: (sessionId) =>
          activatePlanMode({
            sessionId,
            taskId,
            autoFocus: autoFocusNewTasks,
            setActiveDocument,
            setPlanMode,
            router,
          }),
        openPassthroughTask: () => router.push(linkToTask(taskId)),
      });
      recoveryOwner.acknowledge(candidate);
    },
    [
      autoFocusNewTasks,
      clearDraft,
      isPassthroughProfile,
      onOpenChange,
      onSuccess,
      preserveTaskCreateLastUsedOnClose,
      recoveryOwner,
      router,
      setActiveDocument,
      setPlanMode,
      submissionMatchesLiveDraft,
    ],
  );
  /* eslint-enable complexity, sonarjs/cognitive-complexity */

  const applyNativeObservation = useCallback(
    async (
      candidate: TaskCreationReceipt,
      observation: ReturnType<typeof decodeNativeCreateResponse>,
      response: CreateTaskResponse | undefined,
      generation: number,
    ) => {
      const next = recoveryOwner.applyCreateResult(candidate, observation);
      if (!next || !recoveryOwner.isCurrent(next, generation)) return;
      if (next.state !== "accepted") return;
      notifyQueuedTask(response ?? {}, toast);
      await acceptCreatedTask(next, response ?? null, generation);
    },
    [acceptCreatedTask, recoveryOwner, toast],
  );

  /* eslint-disable sonarjs/cognitive-complexity -- native create classifies branch consent, stale policy, and allocation evidence in one transport boundary. */
  const sendNativeCreate = useCallback(
    async (
      candidate: TaskCreationReceipt,
      reconsented: boolean,
      generation: number,
    ): Promise<
      | { kind: "response"; response: CreateTaskResponse }
      | { kind: "uncertain" }
      | { kind: "cancelled" }
      | { kind: "rejected"; error: unknown }
      | { kind: "stale" }
    > => {
      if (!recoveryOwner.isOwnerCurrent(generation)) return { kind: "stale" };
      try {
        const response = await createTaskNative(candidate.payload);
        return { kind: "response", response };
      } catch (error) {
        if (!reconsented && isFreshBranchActive) {
          const dirty = dirtyFilesFromApiError(error);
          if (dirty) {
            const consented = await confirmForDirtyFiles(dirty);
            if (!recoveryOwner.isOwnerCurrent(generation)) {
              recoveryOwner.markUncertain(candidate);
              return { kind: "stale" };
            }
            if (consented === null) {
              recoveryOwner.markUncertain(candidate);
              return { kind: "cancelled" };
            }
            const updated = recoveryOwner.updateConsent(candidate, consented);
            return sendNativeCreate(updated, true, generation);
          }
        }
        if (isStaleBranchPolicyError(error)) {
          if (isDefinitiveCreateRejection(error, candidate)) {
            recoveryOwner.retire(candidate);
            return { kind: "rejected", error };
          }
          await refreshStaleBranchPolicies(error);
          if (!recoveryOwner.isOwnerCurrent(generation)) {
            recoveryOwner.markUncertain(candidate);
            return { kind: "stale" };
          }
          recoveryOwner.markUncertain(candidate);
          return { kind: "uncertain" };
        }
        if (isDefinitiveCreateRejection(error, candidate)) {
          recoveryOwner.retire(candidate);
          return { kind: "rejected", error };
        }
        recoveryOwner.markUncertain(candidate);
        return { kind: "uncertain" };
      }
    },
    [confirmForDirtyFiles, isFreshBranchActive, recoveryOwner, refreshStaleBranchPolicies],
  );
  /* eslint-enable sonarjs/cognitive-complexity */

  const presentAcceptedLookup = useCallback(
    async (next: TaskCreationReceipt, generation: number) => {
      if (!recoveryOwner.isCurrent(next, generation)) return;
      const task = await recoveryOwner.hydrateTask(next);
      if (!recoveryOwner.isCurrent(next, generation)) return;
      await acceptCreatedTask(next, task, generation);
    },
    [acceptCreatedTask, recoveryOwner],
  );

  const checkRecoveryStatus = useCallback(async () => {
    const generation = recoveryOwner.getGeneration();
    const current = recoveryOwner.getReceipt();
    if (!current || !recoveryOwner.beginOperation()) return;
    try {
      const observation = await recoveryOwner.lookup(current);
      const next = recoveryOwner.applyLookupResult(current, observation);
      if (next?.state === "accepted") await presentAcceptedLookup(next, generation);
    } finally {
      recoveryOwner.endOperation();
    }
  }, [presentAcceptedLookup, recoveryOwner]);

  const runRetry = useCallback(
    async (generation: number) => {
      const current = recoveryOwner.getReceipt();
      if (!current) return;
      const observation = await recoveryOwner.lookup(current);
      if (observation.kind !== "absent") {
        const next = recoveryOwner.applyLookupResult(current, observation);
        if (next?.state === "accepted") await presentAcceptedLookup(next, generation);
        return;
      }
      const next = recoveryOwner.applyLookupMiss(current);
      if (!next || !recoveryOwner.isOwnerCurrent(generation)) return;
      if (!next.replayable || next.taskId) return;
      const result = await sendNativeCreate(next, false, generation);
      if (result.kind === "response") {
        await applyNativeObservation(
          next,
          decodeNativeCreateResponse(result.response, next),
          result.response,
          generation,
        );
      } else if (result.kind === "rejected" && recoveryOwner.isOwnerCurrent(generation)) {
        toast({
          title: t("task:failedToCreateTask"),
          description: taskSubmitErrorMessage(result.error),
          variant: "error",
        });
      }
    },
    [applyNativeObservation, presentAcceptedLookup, recoveryOwner, sendNativeCreate, toast],
  );

  const retryRecovery = useCallback(async () => {
    const generation = recoveryOwner.getGeneration();
    if (!recoveryOwner.beginOperation()) return;
    try {
      await runRetry(generation);
    } finally {
      recoveryOwner.endOperation();
    }
  }, [recoveryOwner, runRetry]);

  const runNativeSubmission = useCallback(
    async (
      buildPayload: (consented: string[]) => ReturnType<typeof buildCreateTaskPayload>,
      consent: string[],
    ) => {
      const payload = buildPayload(consent);
      const began = recoveryOwner.beginSubmission(payload);
      if (!began) return;
      if (began.retrying) {
        await runRetry(began.generation);
        return;
      }
      const result = await sendNativeCreate(began.receipt, false, began.generation);
      if (result.kind === "rejected") {
        if (recoveryOwner.isOwnerCurrent(began.generation)) throw result.error;
        return;
      }
      if (result.kind !== "response") return;
      await applyNativeObservation(
        began.receipt,
        decodeNativeCreateResponse(result.response, began.receipt),
        result.response,
        began.generation,
      );
    },
    [applyNativeObservation, recoveryOwner, runRetry, sendNativeCreate],
  );

  const openRecoveryTask = useCallback(async () => {
    const generation = recoveryOwner.getGeneration();
    const current = recoveryOwner.getReceipt();
    if (!current?.taskId || current.state === "unavailable" || !recoveryOwner.beginOperation()) {
      return;
    }
    try {
      const task = await recoveryOwner.hydrateTask(current);
      if (!task || !recoveryOwner.isOwnerCurrent(generation)) return;
      const latest = recoveryOwner.getReceipt();
      if (!latest?.taskId || latest.state === "unavailable") return;
      onOpenChange(false);
      const href =
        latest.state !== "accepted"
          ? linkToTask(latest.taskId, {
              searchParams: new URLSearchParams({ creation: "pending" }),
            })
          : linkToTask(latest.taskId);
      router.push(href);
      recoveryOwner.acknowledge(latest);
    } finally {
      recoveryOwner.endOperation();
    }
  }, [onOpenChange, recoveryOwner, router]);

  const closeRecovery = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);

  const performCreate = useCallback(
    async (opts: {
      trimmedTitle: string;
      trimmedDescription: string;
      consented: string[];
      withAgent: boolean;
      planMode?: boolean;
      attachments?: ReturnType<typeof toMessageAttachments>;
    }) => {
      if (!isSessionMode && !isEditMode && workflowAgentOverridesBlockedReason) return;
      if (!workspaceId || !effectiveWorkflowId) return;
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
      if (!createTask && recovery) {
        await runNativeSubmission(buildPayload, opts.consented);
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
      recovery,
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
      attachments?: ReturnType<typeof toMessageAttachments>,
    ) =>
      performCreate({
        trimmedTitle,
        trimmedDescription: "",
        consented,
        withAgent: false,
        planMode: true,
        attachments,
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
    if (!recoveryOwner.beginOperation()) return;
    try {
      const trimmedTitle = taskName.trim();
      const description = descriptionInputRef.current?.getValue() ?? "";
      const trimmedDescription = description.trim();
      const selectedAttachments = descriptionInputRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      const attachments = toMessageAttachments(selectedAttachments);
      if (!validateForCreate(trimmedTitle, trimmedDescription)) return;
      if (hasRemoteSubmitBlocker()) return;
      const consent = await ensureFreshBranchConsent();
      if (consent === null) return;
      setIsCreatingTask(true);
      try {
        await performCreate({
          trimmedTitle,
          trimmedDescription,
          consented: consent,
          withAgent: true,
          planMode: true,
          attachments,
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
    } finally {
      recoveryOwner.endOperation();
    }
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
    }: {
      trimmedTitle: string;
      trimmedDescription: string;
      consent: string[];
      attachments: ReturnType<typeof toMessageAttachments>;
    }) => {
      if (trimmedDescription) {
        const finalDescription = transformDescriptionBeforeSubmit
          ? await transformDescriptionBeforeSubmit(trimmedDescription)
          : trimmedDescription;
        await performCreate({
          trimmedTitle,
          trimmedDescription: finalDescription,
          consented: consent,
          withAgent: true,
          attachments,
        });
        return;
      }
      if (!autoTitle) {
        await handleCreatePlanMode(trimmedTitle, consent, attachments);
      }
    },
    [autoTitle, handleCreatePlanMode, performCreate, transformDescriptionBeforeSubmit],
  );

  const handleCreateSubmit = useCallback(async () => {
    if (workflowAgentOverridesBlockedReason) return;
    if (!recoveryOwner.beginOperation()) return;
    try {
      const trimmedTitle = taskName.trim();
      const description = descriptionInputRef.current?.getValue() ?? "";
      const trimmedDescription = description.trim();
      const selectedAttachments = descriptionInputRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      const attachments = toMessageAttachments(selectedAttachments);
      if (!validateForCreate(trimmedTitle, trimmedDescription)) return;
      if (hasRemoteSubmitBlocker()) return;
      const consent = await ensureFreshBranchConsent();
      if (consent === null) return;
      setIsCreatingTask(true);
      try {
        await submitCreateTask({ trimmedTitle, trimmedDescription, consent, attachments });
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
    } finally {
      recoveryOwner.endOperation();
    }
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
  ]);

  const submitNoAgentCreate = useCallback(
    async ({
      trimmedTitle,
      trimmedDescription,
      consent,
      attachments,
    }: {
      trimmedTitle: string;
      trimmedDescription: string;
      consent: string[];
      attachments: ReturnType<typeof toMessageAttachments>;
    }) => {
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
        if (!createTask && recovery) {
          await runNativeSubmission(buildPayload, consent);
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
      recovery,
    ],
  );

  const handleCreateWithoutAgent = useCallback(async () => {
    if (workflowAgentOverridesBlockedReason) return;
    if (!recoveryOwner.beginOperation()) return;
    try {
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
      if (consent === null) return;
      await submitNoAgentCreate({ trimmedTitle, trimmedDescription, consent, attachments });
    } finally {
      recoveryOwner.endOperation();
    }
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
