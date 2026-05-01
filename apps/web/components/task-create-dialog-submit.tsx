"use client";

import { useCallback, FormEvent } from "react";
import { useRouter } from "next/navigation";
import { updateTask } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useToast } from "@/components/toast-provider";
import { linkToTask } from "@/lib/links";
import type { SubmitHandlersDeps } from "@/components/task-create-dialog-types";
import { useFreshBranchConsent } from "@/components/task-create-dialog-fresh-branch-consent";

import {
  activatePlanMode,
  buildCreateTaskPayload,
  buildRepositoriesPayload,
  validateCreateInputs,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";

const GENERIC_ERROR_MESSAGE = "An error occurred";

// eslint-disable-next-line max-lines-per-function
export function useTaskSubmitHandlers({
  isSessionMode,
  isEditMode,
  isPassthroughProfile,
  taskName,
  workspaceId,
  workflowId,
  effectiveWorkflowId,
  effectiveDefaultStepId,
  repositories,
  discoveredRepositories,
  useGitHubUrl,
  githubUrl,
  githubPrHeadBranch,
  githubBranch,
  agentProfileId,
  executorId,
  executorProfileId,
  editingTask,
  onSuccess,
  onCreateSession,
  onOpenChange,
  taskId,
  parentTaskId,
  descriptionInputRef,
  setIsCreatingSession,
  setIsCreatingTask,
  setHasTitle,
  setHasDescription,
  setTaskName,
  setRepositories,
  setGitHubBranch,
  setAgentProfileId,
  setExecutorId,
  setSelectedWorkflowId,
  setFetchedSteps,
  clearDraft,
  freshBranchEnabled,
  isLocalExecutor,
  repositoryLocalPath,
  transformDescriptionBeforeSubmit,
}: SubmitHandlersDeps) {
  const router = useRouter();
  const { toast } = useToast();
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);

  const isFreshBranchActive =
    freshBranchEnabled && isLocalExecutor && !useGitHubUrl && repositoryLocalPath !== "";
  const { pendingDiscard, ensureFreshBranchConsent, createTaskWithFreshBranchRetry } =
    useFreshBranchConsent({
      isFreshBranchActive,
      workspaceId,
      repositoryLocalPath,
      toast,
    });

  const buildFreshBranchPayload = (consentedDirtyFiles: string[]) =>
    isFreshBranchActive ? { confirmDiscard: true, consentedDirtyFiles } : undefined;

  const validateForCreate = useCallback(
    (trimmedTitle: string) =>
      validateCreateInputs({
        trimmedTitle,
        workspaceId,
        effectiveWorkflowId,
        repositories,
        githubUrl,
        agentProfileId,
      }),
    [workspaceId, effectiveWorkflowId, repositories, githubUrl, agentProfileId],
  );

  const resetForm = useCallback(() => {
    setHasTitle(false);
    setHasDescription(false);
    setTaskName("");
    setRepositories([]);
    setGitHubBranch("");
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
    setGitHubBranch,
    setAgentProfileId,
    setExecutorId,
    setSelectedWorkflowId,
    setFetchedSteps,
  ]);

  const getRepositoriesPayload = useCallback(
    (consentedDirtyFiles: string[] = []) =>
      buildRepositoriesPayload({
        useGitHubUrl,
        githubUrl,
        githubBranch,
        githubPrHeadBranch,
        repositories,
        discoveredRepositories,
        freshBranch: buildFreshBranchPayload(consentedDirtyFiles),
      }),
    // buildFreshBranchPayload is a closure over current scope; lint exception kept narrow.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      useGitHubUrl,
      githubUrl,
      githubBranch,
      githubPrHeadBranch,
      repositories,
      discoveredRepositories,
      isFreshBranchActive,
    ],
  );

  const handleSessionSubmit = useCallback(async () => {
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
    const attachments = descriptionInputRef.current?.getAttachments() ?? [];
    if (!agentProfileId) return;
    if (!trimmedDescription && !isPassthroughProfile) return;

    if (onCreateSession) {
      onCreateSession({ prompt: trimmedDescription, agentProfileId, executorId });
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
        title: "Failed to create session",
        description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      setIsCreatingSession(false);
    }
  }, [
    agentProfileId,
    executorId,
    executorProfileId,
    isPassthroughProfile,
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
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
    const repositoriesPayload = getRepositoriesPayload();

    const updatePayload: Parameters<typeof updateTask>[1] = {
      title: trimmedTitle,
      description: trimmedDescription,
      ...(repositoriesPayload.length > 0 && { repositories: repositoriesPayload }),
    };

    const updatedTask = await updateTask(editingTask.id, updatePayload);
    return { updatedTask, trimmedDescription };
  }, [editingTask, taskName, descriptionInputRef, getRepositoriesPayload]);

  const handleEditSubmit = useCallback(async () => {
    setIsCreatingTask(true);
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
      toast({
        title: "Failed to update task",
        description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      onOpenChange(false);
      setIsCreatingTask(false);
    }
  }, [
    performTaskUpdate,
    agentProfileId,
    executorId,
    executorProfileId,
    onSuccess,
    onOpenChange,
    toast,
    setIsCreatingTask,
  ]);

  const handleUpdateWithoutAgent = useCallback(async () => {
    setIsCreatingTask(true);
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      onSuccess?.(result.updatedTask, "edit");
    } catch (error) {
      toast({
        title: "Failed to update task",
        description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      onOpenChange(false);
      setIsCreatingTask(false);
    }
  }, [performTaskUpdate, onSuccess, onOpenChange, toast, setIsCreatingTask]);

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
      const buildPayload = (c: string[]) =>
        buildCreateTaskPayload({
          workspaceId,
          effectiveWorkflowId,
          trimmedTitle: opts.trimmedTitle,
          trimmedDescription: opts.trimmedDescription,
          repositoriesPayload: getRepositoriesPayload(c),
          agentProfileId,
          executorId,
          executorProfileId,
          withAgent: opts.withAgent,
          planMode: opts.planMode,
          attachments: opts.attachments,
          parentId: parentTaskId,
        });
      const taskResponse = await createTaskWithFreshBranchRetry(buildPayload, opts.consented);
      if (!taskResponse) return;
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      onSuccess?.(taskResponse, "create", { taskSessionId: newSessionId });
      clearDraft();
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
      agentProfileId,
      executorId,
      executorProfileId,
      isPassthroughProfile,
      parentTaskId,
      onSuccess,
      onOpenChange,
      clearDraft,
      setActiveDocument,
      setPlanMode,
      router,
      getRepositoriesPayload,
      createTaskWithFreshBranchRetry,
    ],
  );

  const handleCreatePlanMode = useCallback(
    (trimmedTitle: string, consented: string[]) =>
      performCreate({
        trimmedTitle,
        trimmedDescription: "",
        consented,
        withAgent: false,
        planMode: true,
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
        toast({
          title: "Failed to start task in plan mode",
          description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
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
    const attachments = toMessageAttachments(descriptionInputRef.current?.getAttachments() ?? []);
    if (!validateForCreate(trimmedTitle)) return;
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
      toast({
        title: "Failed to start task in plan mode",
        description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
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
    ensureFreshBranchConsent,
    performCreate,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
  ]);

  const handleCreateSubmit = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
    const attachments = toMessageAttachments(descriptionInputRef.current?.getAttachments() ?? []);
    if (!validateForCreate(trimmedTitle)) return;
    const consent = await ensureFreshBranchConsent();
    if (consent === null) return;
    setIsCreatingTask(true);
    try {
      if (trimmedDescription || isPassthroughProfile) {
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
      } else {
        await handleCreatePlanMode(trimmedTitle, consent);
      }
    } catch (error) {
      toast({
        title: "Failed to create task",
        description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      setIsCreatingTask(false);
    }
  }, [
    taskName,
    isPassthroughProfile,
    validateForCreate,
    ensureFreshBranchConsent,
    performCreate,
    handleCreatePlanMode,
    transformDescriptionBeforeSubmit,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
  ]);

  const handleCreateWithoutAgent = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const trimmedDescription = (descriptionInputRef.current?.getValue() ?? "").trim();
    if (!validateForCreate(trimmedTitle)) return;
    if (!trimmedDescription || !effectiveDefaultStepId || !workspaceId || !effectiveWorkflowId)
      return;

    const consent = await ensureFreshBranchConsent();
    if (consent === null) return;
    setIsCreatingTask(true);
    try {
      const buildPayload = (c: string[]) => {
        const p = buildCreateTaskPayload({
          workspaceId,
          effectiveWorkflowId,
          trimmedTitle,
          trimmedDescription,
          repositoriesPayload: getRepositoriesPayload(c),
          agentProfileId,
          executorId,
          executorProfileId,
          withAgent: false,
        });
        p.workflow_step_id = effectiveDefaultStepId;
        return p;
      };
      const taskResponse = await createTaskWithFreshBranchRetry(buildPayload, consent);
      if (!taskResponse) return;
      onSuccess?.(taskResponse, "create");
      clearDraft();
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Failed to create task",
        description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      setIsCreatingTask(false);
    }
  }, [
    taskName,
    workspaceId,
    effectiveWorkflowId,
    agentProfileId,
    effectiveDefaultStepId,
    executorId,
    executorProfileId,
    validateForCreate,
    getRepositoriesPayload,
    ensureFreshBranchConsent,
    createTaskWithFreshBranchRetry,
    onSuccess,
    onOpenChange,
    clearDraft,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
  ]);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (isSessionMode) return handleSessionSubmit();
      if (isEditMode) return handleEditSubmit();
      return handleCreateSubmit();
    },
    [isSessionMode, isEditMode, handleSessionSubmit, handleEditSubmit, handleCreateSubmit],
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
