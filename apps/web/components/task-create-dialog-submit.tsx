"use client";

import { useCallback, FormEvent } from "react";
import { useRouter } from "next/navigation";
import { createTask, updateTask } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useToast } from "@/components/toast-provider";
import { linkToTask } from "@/lib/links";
import type { SubmitHandlersDeps } from "@/components/task-create-dialog-types";

import {
  activatePlanMode,
  buildCreateTaskPayload,
  buildRepositoriesPayload,
  validateCreateInputs,
  toMessageAttachments,
  type CreateTaskParams,
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
  repositoryId,
  selectedLocalRepo,
  useGitHubUrl,
  githubUrl,
  githubPrHeadBranch,
  branch,
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
  setRepositoryId,
  setBranch,
  setAgentProfileId,
  setExecutorId,
  setSelectedWorkflowId,
  setFetchedSteps,
  clearDraft,
}: SubmitHandlersDeps) {
  const router = useRouter();
  const { toast } = useToast();
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);

  const resetForm = useCallback(() => {
    setHasTitle(false);
    setHasDescription(false);
    setTaskName("");
    setRepositoryId("");
    setBranch("");
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
    setRepositoryId,
    setBranch,
    setAgentProfileId,
    setExecutorId,
    setSelectedWorkflowId,
    setFetchedSteps,
  ]);

  const getRepositoriesPayload = useCallback(
    () =>
      buildRepositoriesPayload({
        useGitHubUrl,
        githubUrl,
        branch,
        githubPrHeadBranch,
        repositoryId,
        selectedLocalRepo,
      }),
    [useGitHubUrl, repositoryId, branch, githubUrl, githubPrHeadBranch, selectedLocalRepo],
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

  const performCreateWithAgent = useCallback(
    async (
      trimmedTitle: string,
      trimmedDescription: string,
      repositoriesPayload: CreateTaskParams["repositories"],
      planMode?: boolean,
      attachments?: ReturnType<typeof toMessageAttachments>,
    ) => {
      if (!workspaceId || !effectiveWorkflowId) return;
      const taskResponse = await createTask(
        buildCreateTaskPayload({
          workspaceId,
          effectiveWorkflowId,
          trimmedTitle,
          trimmedDescription,
          repositoriesPayload,
          agentProfileId,
          executorId,
          executorProfileId,
          withAgent: true,
          planMode,
          attachments,
          parentId: parentTaskId,
        }),
      );
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      onSuccess?.(taskResponse, "create", { taskSessionId: newSessionId });
      clearDraft();
      onOpenChange(false);
      if (planMode && newSessionId) {
        activatePlanMode({
          sessionId: newSessionId,
          taskId: taskResponse.id,
          setActiveDocument,
          setPlanMode,
          router,
        });
      } else if (isPassthroughProfile) {
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
    ],
  );

  const handleCreatePlanMode = useCallback(
    async (trimmedTitle: string, repositoriesPayload: CreateTaskParams["repositories"]) => {
      if (!workspaceId || !effectiveWorkflowId) return;
      const taskResponse = await createTask(
        buildCreateTaskPayload({
          workspaceId,
          effectiveWorkflowId,
          trimmedTitle,
          trimmedDescription: "",
          repositoriesPayload,
          agentProfileId,
          executorId,
          executorProfileId,
          withAgent: false,
          planMode: true,
          parentId: parentTaskId,
        }),
      );
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      onSuccess?.(taskResponse, "create", { taskSessionId: newSessionId });
      clearDraft();
      onOpenChange(false);
      if (newSessionId) {
        activatePlanMode({
          sessionId: newSessionId,
          taskId: taskResponse.id,
          setActiveDocument,
          setPlanMode,
          router,
        });
      }
    },
    [
      workspaceId,
      effectiveWorkflowId,
      agentProfileId,
      executorId,
      executorProfileId,
      parentTaskId,
      onSuccess,
      onOpenChange,
      clearDraft,
      setActiveDocument,
      setPlanMode,
      router,
    ],
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
    setIsCreatingTask(true);
    try {
      if (isEditMode) {
        await performEditWithPlanMode();
      } else {
        const trimmedTitle = taskName.trim();
        const description = descriptionInputRef.current?.getValue() ?? "";
        const trimmedDescription = description.trim();
        const attachments = toMessageAttachments(
          descriptionInputRef.current?.getAttachments() ?? [],
        );
        if (
          !validateCreateInputs({
            trimmedTitle,
            workspaceId,
            effectiveWorkflowId,
            repositoryId,
            selectedLocalRepo,
            githubUrl,
            agentProfileId,
          })
        )
          return;
        await performCreateWithAgent(
          trimmedTitle,
          trimmedDescription,
          getRepositoriesPayload(),
          true,
          attachments,
        );
      }
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
    workspaceId,
    effectiveWorkflowId,
    repositoryId,
    selectedLocalRepo,
    githubUrl,
    agentProfileId,
    getRepositoriesPayload,
    performCreateWithAgent,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
  ]);

  const handleCreateSubmit = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
    const attachments = toMessageAttachments(descriptionInputRef.current?.getAttachments() ?? []);
    if (
      !validateCreateInputs({
        trimmedTitle,
        workspaceId,
        effectiveWorkflowId,
        repositoryId,
        selectedLocalRepo,
        githubUrl,
        agentProfileId,
      })
    )
      return;
    const repositoriesPayload = getRepositoriesPayload();
    setIsCreatingTask(true);
    try {
      if (trimmedDescription || isPassthroughProfile) {
        await performCreateWithAgent(
          trimmedTitle,
          trimmedDescription,
          repositoriesPayload,
          undefined,
          attachments,
        );
      } else {
        await handleCreatePlanMode(trimmedTitle, repositoriesPayload);
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
    workspaceId,
    effectiveWorkflowId,
    repositoryId,
    selectedLocalRepo,
    githubUrl,
    agentProfileId,
    isPassthroughProfile,
    getRepositoriesPayload,
    performCreateWithAgent,
    handleCreatePlanMode,
    toast,
    descriptionInputRef,
    setIsCreatingTask,
  ]);

  const handleCreateWithoutAgent = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const trimmedDescription = (descriptionInputRef.current?.getValue() ?? "").trim();
    if (
      !validateCreateInputs({
        trimmedTitle,
        workspaceId,
        effectiveWorkflowId,
        repositoryId,
        selectedLocalRepo,
        githubUrl,
        agentProfileId,
      })
    )
      return;
    if (!trimmedDescription || !effectiveDefaultStepId || !workspaceId || !effectiveWorkflowId)
      return;

    setIsCreatingTask(true);
    try {
      const payload = buildCreateTaskPayload({
        workspaceId,
        effectiveWorkflowId,
        trimmedTitle,
        trimmedDescription,
        repositoriesPayload: getRepositoriesPayload(),
        agentProfileId,
        executorId,
        executorProfileId,
        withAgent: false,
      });
      payload.workflow_step_id = effectiveDefaultStepId;
      const taskResponse = await createTask(payload);
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
    repositoryId,
    selectedLocalRepo,
    githubUrl,
    agentProfileId,
    effectiveDefaultStepId,
    executorId,
    executorProfileId,
    getRepositoriesPayload,
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
  };
}
