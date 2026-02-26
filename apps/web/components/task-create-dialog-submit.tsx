"use client";

import { useCallback, FormEvent } from "react";
import { useRouter } from "next/navigation";
import type { Task, LocalRepository } from "@/lib/types/http";
import { createTask, updateTask } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useToast } from "@/components/toast-provider";
import { linkToSession } from "@/lib/links";
import {
  activatePlanMode,
  buildCreateTaskPayload,
  validateCreateInputs,
  type CreateTaskParams,
} from "@/components/task-create-dialog-helpers";

const GENERIC_ERROR_MESSAGE = "An error occurred";

export type SubmitHandlersDeps = {
  isSessionMode: boolean;
  isEditMode: boolean;
  isPassthroughProfile: boolean;
  taskName: string;
  workspaceId: string | null;
  workflowId: string | null;
  effectiveWorkflowId: string | null;
  effectiveDefaultStepId: string | null;
  repositoryId: string;
  selectedLocalRepo: LocalRepository | null;
  branch: string;
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
  editingTask?: {
    id: string;
    title: string;
    description?: string;
    workflowStepId: string;
    state?: Task["state"];
    repositoryId?: string;
  } | null;
  onSuccess?: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null },
  ) => void;
  onCreateSession?: (data: { prompt: string; agentProfileId: string; executorId: string }) => void;
  onOpenChange: (open: boolean) => void;
  taskId: string | null;
  descriptionInputRef: React.RefObject<{ getValue: () => string } | null>;
  setIsCreatingSession: (v: boolean) => void;
  setIsCreatingTask: (v: boolean) => void;
  setHasTitle: (v: boolean) => void;
  setHasDescription: (v: boolean) => void;
  setTaskName: (v: string) => void;
  setRepositoryId: (v: string) => void;
  setBranch: (v: string) => void;
  setAgentProfileId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: null) => void;
};

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
  branch,
  agentProfileId,
  executorId,
  executorProfileId,
  editingTask,
  onSuccess,
  onCreateSession,
  onOpenChange,
  taskId,
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

  const getRepositoriesPayload = useCallback(() => {
    if (repositoryId) {
      return [{ repository_id: repositoryId, base_branch: branch || undefined }];
    }
    if (selectedLocalRepo) {
      return [
        {
          repository_id: "",
          base_branch: branch || undefined,
          local_path: selectedLocalRepo.path,
          default_branch: selectedLocalRepo.default_branch || undefined,
        },
      ];
    }
    return [];
  }, [repositoryId, branch, selectedLocalRepo]);

  const handleSessionSubmit = useCallback(async () => {
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
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
      });
      const response = await launchSession(request);

      const newSessionId = response?.session_id;
      if (newSessionId) {
        router.push(linkToSession(newSessionId));
      } else {
        onOpenChange(false);
      }
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
        }),
      );
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      onSuccess?.(taskResponse, "create", { taskSessionId: newSessionId });
      if (planMode && newSessionId) {
        onOpenChange(false);
        activatePlanMode({
          sessionId: newSessionId,
          taskId: taskResponse.id,
          setActiveDocument,
          setPlanMode,
          router,
        });
      } else if (isPassthroughProfile && newSessionId) {
        router.push(linkToSession(newSessionId));
      } else {
        onOpenChange(false);
      }
    },
    [
      workspaceId,
      effectiveWorkflowId,
      agentProfileId,
      executorId,
      executorProfileId,
      isPassthroughProfile,
      onSuccess,
      onOpenChange,
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
        }),
      );
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      onSuccess?.(taskResponse, "create", { taskSessionId: newSessionId });
      if (newSessionId) {
        activatePlanMode({
          sessionId: newSessionId,
          taskId: taskResponse.id,
          setActiveDocument,
          setPlanMode,
          router,
        });
      } else {
        onOpenChange(false);
      }
    },
    [
      workspaceId,
      effectiveWorkflowId,
      agentProfileId,
      executorId,
      executorProfileId,
      onSuccess,
      onOpenChange,
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
        if (
          !validateCreateInputs({
            trimmedTitle,
            workspaceId,
            effectiveWorkflowId,
            repositoryId,
            selectedLocalRepo,
            agentProfileId,
          })
        )
          return;
        await performCreateWithAgent(
          trimmedTitle,
          trimmedDescription,
          getRepositoriesPayload(),
          true,
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
    if (
      !validateCreateInputs({
        trimmedTitle,
        workspaceId,
        effectiveWorkflowId,
        repositoryId,
        selectedLocalRepo,
        agentProfileId,
      })
    )
      return;
    const repositoriesPayload = getRepositoriesPayload();
    setIsCreatingTask(true);
    try {
      if (trimmedDescription || isPassthroughProfile) {
        await performCreateWithAgent(trimmedTitle, trimmedDescription, repositoriesPayload);
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
    const description = descriptionInputRef.current?.getValue() ?? "";
    const trimmedDescription = description.trim();
    if (
      !validateCreateInputs({
        trimmedTitle,
        workspaceId,
        effectiveWorkflowId,
        repositoryId,
        selectedLocalRepo,
        agentProfileId,
      })
    )
      return;
    if (!trimmedDescription) return;
    const stepId = effectiveDefaultStepId;
    if (!stepId || !workspaceId || !effectiveWorkflowId) return;

    setIsCreatingTask(true);
    try {
      const reposPayload = getRepositoriesPayload();
      const taskResponse = await createTask({
        workspace_id: workspaceId,
        workflow_id: effectiveWorkflowId,
        workflow_step_id: stepId,
        title: trimmedTitle,
        description: trimmedDescription,
        repositories: reposPayload,
        state: "CREATED",
        agent_profile_id: agentProfileId || undefined,
        executor_id: executorId || undefined,
        executor_profile_id: executorProfileId || undefined,
      });
      onSuccess?.(taskResponse, "create");
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
    agentProfileId,
    effectiveDefaultStepId,
    executorId,
    executorProfileId,
    getRepositoriesPayload,
    onSuccess,
    onOpenChange,
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
