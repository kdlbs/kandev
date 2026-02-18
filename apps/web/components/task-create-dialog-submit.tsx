'use client';

import { useCallback, FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import type { Task } from '@/lib/types/http';
import type { LocalRepository } from '@/lib/types/http';
import { createTask, updateTask } from '@/lib/api';
import { useAppStore } from '@/components/state-provider';
import type { AppState } from '@/lib/state/store';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useToast } from '@/components/toast-provider';
import { linkToSession } from '@/lib/links';
import { useDockviewStore } from '@/lib/state/dockview-store';
import { useContextFilesStore } from '@/lib/state/context-files-store';

type CreateTaskParams = Parameters<typeof createTask>[0];

const GENERIC_ERROR_MESSAGE = 'An error occurred';

interface OrchestratorStartResponse {
  success: boolean;
  task_id: string;
  agent_instance_id: string;
  session_id?: string;
  state: string;
}

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
  environmentId: string;
  executorId: string;
  editingTask?: { id: string; title: string; description?: string; workflowStepId: string; state?: Task['state']; repositoryId?: string } | null;
  onSuccess?: (task: Task, mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => void;
  onCreateSession?: (data: { prompt: string; agentProfileId: string; executorId: string; environmentId: string }) => void;
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
  setEnvironmentId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: null) => void;
};

type ActivatePlanModeArgs = {
  sessionId: string;
  taskId: string;
  setActiveDocument: AppState['setActiveDocument'];
  setPlanMode: AppState['setPlanMode'];
  router: ReturnType<typeof useRouter>;
};

function activatePlanMode({ sessionId, taskId, setActiveDocument, setPlanMode, router }: ActivatePlanModeArgs) {
  setActiveDocument(sessionId, { type: 'plan', taskId });
  useDockviewStore.getState().queuePanelAction({ id: 'plan', component: 'plan', title: 'Plan', placement: 'right', referencePanel: 'chat' });
  setPlanMode(sessionId, true);
  useContextFilesStore.getState().addFile(sessionId, { path: 'plan:context', name: 'Plan' });
  router.push(linkToSession(sessionId));
}

type BuildCreatePayloadArgs = {
  workspaceId: string;
  effectiveWorkflowId: string;
  trimmedTitle: string;
  trimmedDescription: string;
  repositoriesPayload: CreateTaskParams['repositories'];
  agentProfileId: string;
  executorId: string;
  withAgent: boolean;
};

function buildCreateTaskPayload({
  workspaceId, effectiveWorkflowId, trimmedTitle, trimmedDescription,
  repositoriesPayload, agentProfileId, executorId, withAgent,
}: BuildCreatePayloadArgs): CreateTaskParams {
  return {
    workspace_id: workspaceId,
    workflow_id: effectiveWorkflowId,
    title: trimmedTitle,
    description: trimmedDescription,
    repositories: repositoriesPayload,
    state: withAgent ? 'IN_PROGRESS' : 'CREATED',
    start_agent: withAgent ? true : undefined,
    prepare_session: withAgent ? undefined : true,
    agent_profile_id: agentProfileId || undefined,
    executor_id: executorId || undefined,
  };
}

type CreateInputs = {
  trimmedTitle: string; workspaceId: string | null; effectiveWorkflowId: string | null;
  repositoryId: string; selectedLocalRepo: LocalRepository | null; agentProfileId: string;
};

function validateCreateInputs({ trimmedTitle, workspaceId, effectiveWorkflowId, repositoryId, selectedLocalRepo, agentProfileId }: CreateInputs): boolean {
  return Boolean(trimmedTitle && workspaceId && effectiveWorkflowId && (repositoryId || selectedLocalRepo) && agentProfileId);
}

// eslint-disable-next-line max-lines-per-function
export function useTaskSubmitHandlers({
  isSessionMode, isEditMode, isPassthroughProfile,
  taskName, workspaceId, workflowId, effectiveWorkflowId, effectiveDefaultStepId,
  repositoryId, selectedLocalRepo, branch, agentProfileId, environmentId, executorId,
  editingTask, onSuccess, onCreateSession, onOpenChange, taskId, descriptionInputRef,
  setIsCreatingSession, setIsCreatingTask, setHasTitle, setHasDescription, setTaskName,
  setRepositoryId, setBranch, setAgentProfileId, setEnvironmentId, setExecutorId,
  setSelectedWorkflowId, setFetchedSteps,
}: SubmitHandlersDeps) {
  const router = useRouter();
  const { toast } = useToast();
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);

  const resetForm = useCallback(() => {
    setHasTitle(false);
    setHasDescription(false);
    setTaskName('');
    setRepositoryId('');
    setBranch('');
    setAgentProfileId('');
    setEnvironmentId('');
    setExecutorId('');
    setSelectedWorkflowId(workflowId);
    setFetchedSteps(null);
    // State setters are stable; only workflowId can change
  }, [workflowId, setHasTitle, setHasDescription, setTaskName, setRepositoryId, setBranch, setAgentProfileId, setEnvironmentId, setExecutorId, setSelectedWorkflowId, setFetchedSteps]);

  const getRepositoriesPayload = useCallback(() => {
    if (repositoryId) {
      return [{ repository_id: repositoryId, base_branch: branch || undefined }];
    }
    if (selectedLocalRepo) {
      return [{
        repository_id: '',
        base_branch: branch || undefined,
        local_path: selectedLocalRepo.path,
        default_branch: selectedLocalRepo.default_branch || undefined,
      }];
    }
    return [];
  }, [repositoryId, branch, selectedLocalRepo]);

  const handleSessionSubmit = useCallback(async () => {
    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    if (!agentProfileId) return;
    if (!trimmedDescription && !isPassthroughProfile) return;

    if (onCreateSession) {
      onCreateSession({ prompt: trimmedDescription, agentProfileId, executorId, environmentId });
      resetForm();
      onOpenChange(false);
      return;
    }

    if (!taskId) return;

    setIsCreatingSession(true);
    try {
      const client = getWebSocketClient();
      if (!client) throw new Error('WebSocket client not available');

      const response = await client.request<OrchestratorStartResponse>(
        'orchestrator.start',
        { task_id: taskId, agent_profile_id: agentProfileId, executor_id: executorId, prompt: trimmedDescription },
        15000,
      );

      resetForm();
      const newSessionId = response?.session_id;
      if (newSessionId) {
        router.push(linkToSession(newSessionId));
      } else {
        onOpenChange(false);
      }
    } catch (error) {
      toast({ title: 'Failed to create session', description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE, variant: 'error' });
    } finally {
      setIsCreatingSession(false);
    }
  }, [agentProfileId, environmentId, executorId, isPassthroughProfile, onCreateSession, onOpenChange, resetForm, router, taskId, toast, descriptionInputRef, setIsCreatingSession]);

  const performTaskUpdate = useCallback(async () => {
    if (!editingTask) return null;
    const trimmedTitle = taskName.trim();
    if (!trimmedTitle) return null;
    const description = descriptionInputRef.current?.getValue() ?? '';
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
        const client = getWebSocketClient();
        if (client) {
          try {
            const response = await client.request<OrchestratorStartResponse>(
              'orchestrator.start',
              {
                task_id: updatedTask.id,
                agent_profile_id: agentProfileId,
                executor_id: executorId,
                prompt: trimmedDescription || '',
              },
              15000,
            );
            taskSessionId = response?.session_id ?? null;
          } catch (error) {
            console.error('[TaskCreateDialog] failed to start agent:', error);
          }
        }
      }

      onSuccess?.(updatedTask, 'edit', { taskSessionId });
    } catch (error) {
      toast({ title: 'Failed to update task', description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE, variant: 'error' });
    } finally {
      resetForm();
      setIsCreatingTask(false);
      onOpenChange(false);
    }
  }, [performTaskUpdate, agentProfileId, executorId, onSuccess, resetForm, onOpenChange, toast, setIsCreatingTask]);

  const handleUpdateWithoutAgent = useCallback(async () => {
    setIsCreatingTask(true);
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      onSuccess?.(result.updatedTask, 'edit');
    } catch (error) {
      toast({ title: 'Failed to update task', description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE, variant: 'error' });
    } finally {
      resetForm();
      setIsCreatingTask(false);
      onOpenChange(false);
    }
  }, [performTaskUpdate, onSuccess, onOpenChange, resetForm, toast, setIsCreatingTask]);

  const handleCreateWithAgent = useCallback(async (
    trimmedTitle: string, trimmedDescription: string, repositoriesPayload: CreateTaskParams['repositories'],
  ) => {
    if (!workspaceId || !effectiveWorkflowId) return;
    const taskResponse = await createTask(buildCreateTaskPayload({
      workspaceId, effectiveWorkflowId, trimmedTitle, trimmedDescription,
      repositoriesPayload, agentProfileId, executorId, withAgent: true,
    }));
    const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
    onSuccess?.(taskResponse, 'create', { taskSessionId: newSessionId });
    resetForm();
    if (isPassthroughProfile && newSessionId) {
      router.push(linkToSession(newSessionId));
    } else {
      onOpenChange(false);
    }
  }, [workspaceId, effectiveWorkflowId, agentProfileId, executorId, isPassthroughProfile, onSuccess, resetForm, onOpenChange, router]);

  const handleCreatePlanMode = useCallback(async (
    trimmedTitle: string, repositoriesPayload: CreateTaskParams['repositories'],
  ) => {
    if (!workspaceId || !effectiveWorkflowId) return;
    const taskResponse = await createTask(buildCreateTaskPayload({
      workspaceId, effectiveWorkflowId, trimmedTitle, trimmedDescription: '',
      repositoriesPayload, agentProfileId, executorId, withAgent: false,
    }));
    const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
    onSuccess?.(taskResponse, 'create', { taskSessionId: newSessionId });
    resetForm();
    if (newSessionId) {
      activatePlanMode({ sessionId: newSessionId, taskId: taskResponse.id, setActiveDocument, setPlanMode, router });
    } else {
      onOpenChange(false);
    }
  }, [workspaceId, effectiveWorkflowId, agentProfileId, executorId, onSuccess, resetForm, onOpenChange, setActiveDocument, setPlanMode, router]);

  const handleCreateSubmit = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    if (!validateCreateInputs({ trimmedTitle, workspaceId, effectiveWorkflowId, repositoryId, selectedLocalRepo, agentProfileId })) return;
    const repositoriesPayload = getRepositoriesPayload();
    setIsCreatingTask(true);
    try {
      if (trimmedDescription || isPassthroughProfile) {
        await handleCreateWithAgent(trimmedTitle, trimmedDescription, repositoriesPayload);
      } else {
        await handleCreatePlanMode(trimmedTitle, repositoriesPayload);
      }
    } catch (error) {
      toast({ title: 'Failed to create task', description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE, variant: 'error' });
    } finally {
      setIsCreatingTask(false);
    }
  }, [
    taskName, workspaceId, effectiveWorkflowId, repositoryId, selectedLocalRepo, agentProfileId,
    isPassthroughProfile, getRepositoriesPayload, handleCreateWithAgent, handleCreatePlanMode,
    toast, descriptionInputRef, setIsCreatingTask,
  ]);

  const handleCreateWithoutAgent = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    if (!validateCreateInputs({ trimmedTitle, workspaceId, effectiveWorkflowId, repositoryId, selectedLocalRepo, agentProfileId })) return;
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
        state: 'CREATED',
        agent_profile_id: agentProfileId || undefined,
        executor_id: executorId || undefined,
      });
      onSuccess?.(taskResponse, 'create');
      resetForm();
      onOpenChange(false);
    } catch (error) {
      toast({ title: 'Failed to create task', description: error instanceof Error ? error.message : GENERIC_ERROR_MESSAGE, variant: 'error' });
    } finally {
      setIsCreatingTask(false);
    }
  }, [
    taskName, workspaceId, effectiveWorkflowId, repositoryId, selectedLocalRepo,
    agentProfileId, effectiveDefaultStepId, executorId, getRepositoriesPayload,
    onSuccess, onOpenChange, resetForm, toast, descriptionInputRef, setIsCreatingTask,
  ]);

  const handleSubmit = useCallback(async (e: FormEvent) => {
    e.preventDefault();
    if (isSessionMode) return handleSessionSubmit();
    if (isEditMode) return handleEditSubmit();
    return handleCreateSubmit();
  }, [isSessionMode, isEditMode, handleSessionSubmit, handleEditSubmit, handleCreateSubmit]);

  const handleCancel = useCallback(() => {
    resetForm();
    onOpenChange(false);
  }, [resetForm, onOpenChange]);

  return {
    resetForm,
    handleSubmit,
    handleUpdateWithoutAgent,
    handleCreateWithoutAgent,
    handleCancel,
  };
}
