"use client";

/* eslint-disable max-lines -- create and edit submit flows share one lifecycle boundary. */

import { useCallback, FormEvent } from "react";
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
} from "@/components/task-create-dialog-helpers";
import { hasRegisteredRepositoryProviderCandidate } from "@/lib/plugins/repository-provider-url-resolution";

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

export function taskSubmitErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    const key = REPOSITORY_SELECTION_ERROR_KEYS[error.errorCode ?? ""];
    if (key) return t(key);
  }
  return error instanceof Error ? error.message : t(GENERIC_ERROR_KEY);
}

function isRepositorySelectionError(error: unknown): boolean {
  return (
    error instanceof ApiError && Boolean(REPOSITORY_SELECTION_ERROR_KEYS[error.errorCode ?? ""])
  );
}

// eslint-disable-next-line max-lines-per-function
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
  setSelectedWorkflowId,
  setFetchedSteps,
  clearDraft,
  freshBranchEnabled,
  isLocalExecutor,
  repositoryLocalPath,
  noRepository,
  workspacePath,
  blockedBy,
  transformDescriptionBeforeSubmit,
}: SubmitHandlersDeps) {
  const router = useRouter();
  const { toast } = useToast();
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);
  const isStartedEdit = computeIsTaskStarted(isEditMode, editingTask);

  const isFreshBranchActive =
    freshBranchEnabled && isLocalExecutor && !useRemote && repositoryLocalPath !== "";
  const { pendingDiscard, ensureFreshBranchConsent, createTaskWithFreshBranchRetry } =
    useFreshBranchConsent({
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
      });
      await launchSession(request);

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
  ]);

  const performTaskUpdate = useCallback(async () => {
    if (!editingTask) return null;
    const trimmedTitle = taskName.trim();
    if (!trimmedTitle) return null;
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

    const updatedTask = await updateTask(editingTask.id, updatePayload);
    return { updatedTask, trimmedDescription };
  }, [
    editingTask,
    taskName,
    descriptionInputRef,
    getRepositoriesPayload,
    isStartedEdit,
    repositoriesDirty,
  ]);

  const handleEditSubmit = useCallback(async () => {
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
        } catch (error) {
          console.error("[TaskCreateDialog] failed to start agent:", error);
        }
      }

      onSuccess?.(updatedTask, "edit", { taskSessionId });
    } catch (error) {
      closeDialog = !(
        isRepositorySelectionError(error) || (await refreshStaleBranchPolicies(error))
      );
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
    toast,
    setIsCreatingTask,
  ]);

  const handleUpdateWithoutAgent = useCallback(async () => {
    if (checkRemoteResolution()) return;
    setIsCreatingTask(true);
    let closeDialog = true;
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      onSuccess?.(result.updatedTask, "edit");
    } catch (error) {
      closeDialog = !(
        isRepositorySelectionError(error) || (await refreshStaleBranchPolicies(error))
      );
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
    toast,
    setIsCreatingTask,
  ]);

  const performCreate = useCallback(
    async (opts: {
      trimmedTitle: string;
      trimmedDescription: string;
      consented: string[];
      withAgent: boolean;
      planMode?: boolean;
      attachments?: ReturnType<typeof toMessageAttachments>;
    }) => {
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
          blockedBy,
        });
        submittedPayload = payload;
        return payload;
      };
      const taskResponse = await createTaskWithFreshBranchRetry(buildPayload, opts.consented);
      if (!taskResponse) return;
      notifyQueuedTask(taskResponse, toast);
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      const willNavigate =
        (opts.withAgent && isPassthroughProfile) || !!(opts.planMode && newSessionId);
      onSuccess?.(taskResponse, "create", { taskSessionId: newSessionId, willNavigate });
      clearDraft();
      queueTaskCreateLastUsedFromPayload(submittedPayload);
      preserveTaskCreateLastUsedOnClose?.();
      onOpenChange(false);
      if (opts.planMode && newSessionId) {
        activatePlanMode({
          sessionId: newSessionId,
          taskId: taskResponse.id,
          setActiveDocument,
          setPlanMode,
          router,
        });
      } else if (opts.withAgent && isPassthroughProfile) {
        router.push(linkToTask(taskResponse.id));
      }
    },
    [
      workspaceId,
      effectiveWorkflowId,
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
      onSuccess,
      onOpenChange,
      preserveTaskCreateLastUsedOnClose,
      clearDraft,
      setActiveDocument,
      setPlanMode,
      router,
      getRepositoriesPayload,
      createTaskWithFreshBranchRetry,
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
  ]);

  const handleCreateWithPlanMode = useCallback(async () => {
    if (isEditMode) {
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
  }, [
    isEditMode,
    performEditWithPlanMode,
    taskName,
    validateForCreate,
    hasRemoteSubmitBlocker,
    ensureFreshBranchConsent,
    performCreate,
    refreshStaleBranchPolicies,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
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
  ]);

  const handleCreateWithoutAgent = useCallback(async () => {
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
    setIsCreatingTask(true);
    try {
      let submittedPayload: ReturnType<typeof buildCreateTaskPayload> | null = null;
      const buildPayload = (c: string[]) => {
        const p = buildCreateTaskPayload({
          workspaceId: requirements.workspaceId,
          effectiveWorkflowId: requirements.workflowId,
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
          blockedBy,
        });
        submittedPayload = p;
        return p;
      };
      const taskResponse = await createTaskWithFreshBranchRetry(buildPayload, consent);
      if (!taskResponse) return;
      notifyQueuedTask(taskResponse, toast);
      onSuccess?.(taskResponse, "create");
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
  }, [
    taskName,
    autoTitle,
    workspaceId,
    effectiveWorkflowId,
    agentProfileId,
    executorId,
    executorProfileId,
    noRepository,
    autopilot,
    workspacePath,
    validateForCreate,
    hasRemoteSubmitBlocker,
    getRepositoriesPayload,
    ensureFreshBranchConsent,
    createTaskWithFreshBranchRetry,
    refreshStaleBranchPolicies,
    onSuccess,
    onOpenChange,
    preserveTaskCreateLastUsedOnClose,
    clearDraft,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
    blockedBy,
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

  return {
    resetForm,
    handleSubmit,
    handleUpdateWithoutAgent,
    handleCreateWithoutAgent,
    handleCreateWithPlanMode,
    handleCancel,
    pendingDiscard,
  };
}
