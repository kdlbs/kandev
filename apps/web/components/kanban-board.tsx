'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Task } from './kanban-card';
import { TaskCreateDialog } from './task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { Task as BackendTask } from '@/lib/types/http';
import type { WorkflowsState } from '@/lib/state/slices';
import { type WorkflowAutomation, type MoveTaskError } from '@/hooks/use-drag-and-drop';
import { SwimlaneContainer } from './kanban/swimlane-container';
import { KanbanHeader } from './kanban/kanban-header';
import { useKanbanData, useKanbanActions, useKanbanNavigation } from '@/hooks/domains/kanban';
import { useAllWorkflowSnapshots } from '@/hooks/domains/kanban/use-all-workflow-snapshots';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
import { HomepageCommands } from './homepage-commands';
import { getWebSocketClient } from '@/lib/ws/connection';
import { linkToSession } from '@/lib/links';
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from '@kandev/ui/alert-dialog';
import { IconAlertTriangle } from '@tabler/icons-react';

interface KanbanBoardProps {
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task, sessionId: string) => void;
}

export function KanbanBoard({ onPreviewTask, onOpenTask }: KanbanBoardProps = {}) {
  // Store access
  const store = useAppStoreApi();
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();

  // Search state
  const [searchQuery, setSearchQuery] = useState('');

  // View mode from user settings
  const kanbanViewMode = useAppStore((state) => state.userSettings.kanbanViewMode);

  // Get initial state for actions hook
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const workflowsState = useAppStore((state) => state.workflows);
  const setActiveWorkflow = useAppStore((state) => state.setActiveWorkflow);
  const setWorkflows = useAppStore((state) => state.setWorkflows);

  // Load all workflow snapshots for swimlane views
  useAllWorkflowSnapshots(workspaceState.activeId);

  // Consolidated actions hook
  const {
    isDialogOpen,
    editingTask,
    setIsDialogOpen,
    setEditingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleDialogOpenChange,
    handleDialogSuccess,
    handleWorkspaceChange,
    handleWorkflowChange,
    deletingTaskId,
  } = useKanbanActions({ workspaceState, workflowsState });

  // Data fetching and derived state
  const {
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    activeSteps,
    isMounted,
    setTaskSessionAvailability,
  } = useKanbanData({
    onWorkspaceChange: handleWorkspaceChange,
    onWorkflowChange: handleWorkflowChange,
    searchQuery,
  });

  // Navigation hook
  const { handleOpenTask, handleCardClick } = useKanbanNavigation({
    enablePreviewOnClick,
    isMobile,
    onPreviewTask,
    onOpenTask,
    setEditingTask,
    setIsDialogOpen,
    setTaskSessionAvailability,
  });

  // Workflow automation state for session creation dialog
  const [workflowAutomation, setWorkflowAutomation] = useState<WorkflowAutomation | null>(null);
  const [isWorkflowDialogOpen, setIsWorkflowDialogOpen] = useState(false);

  // Move error state for approval warning modal
  const [moveError, setMoveError] = useState<MoveTaskError | null>(null);

  // Handle workflow automation when task is moved to auto-start step
  const handleWorkflowAutomation = useCallback((automation: WorkflowAutomation) => {
    setWorkflowAutomation(automation);
    setIsWorkflowDialogOpen(true);
  }, []);

  // Handle move error - show approval warning modal
  const handleMoveError = useCallback((error: MoveTaskError) => {
    setMoveError(error);
  }, []);

  // Handle navigation to task session from approval warning modal
  const handleGoToTask = useCallback(() => {
    if (moveError?.sessionId) {
      router.push(linkToSession(moveError.sessionId));
    }
    setMoveError(null);
  }, [moveError, router]);

  // Handle session creation from workflow automation dialog
  const handleWorkflowSessionCreate = useCallback(
    async (data: { prompt: string; agentProfileId: string; executorId: string; environmentId: string }) => {
      if (!workflowAutomation) return;

      const client = getWebSocketClient();
      if (!client) return;

      try {
        // Start the task with workflow step configuration
        await client.request(
          'orchestrator.start',
          {
            task_id: workflowAutomation.taskId,
            agent_profile_id: data.agentProfileId,
            executor_id: data.executorId,
            prompt: data.prompt.trim(),
            workflow_step_id: workflowAutomation.workflowStep.id,
          },
          15000
        );
      } catch (err) {
        console.error('Failed to start session for workflow step:', err);
      } finally {
        setWorkflowAutomation(null);
        setIsWorkflowDialogOpen(false);
      }
    },
    [workflowAutomation]
  );

  // Ref for userSettings to avoid reactive deps in workflow selection effect
  const userSettingsRef = useRef(userSettings);
  useEffect(() => {
    userSettingsRef.current = userSettings;
  });

  // Workflow selection effect
  useEffect(() => {
    const workspaceId = workspaceState.activeId;
    if (!workspaceId) {
      if (workflowsState.items.length || workflowsState.activeId) {
        setWorkflows([]);
        setActiveWorkflow(null);
      }
      return;
    }
    const settings = userSettingsRef.current;
    const workspaceWorkflows = workflowsState.items.filter(
      (workflow: WorkflowsState['items'][number]) => workflow.workspaceId === workspaceId
    );

    // Default to "All Workflows" (null) unless settings explicitly specify a workflow
    const desiredWorkflowId =
      settings.workflowId && workspaceWorkflows.some((workflow: WorkflowsState['items'][number]) => workflow.id === settings.workflowId)
        ? settings.workflowId
        : null;
    setActiveWorkflow(desiredWorkflowId);
    if (!desiredWorkflowId) {
      store.getState().hydrate({
        kanban: { workflowId: null, steps: [], tasks: [] },
      });
    }
  }, [
    workflowsState.activeId,
    workflowsState.items,
    commitSettings,
    setActiveWorkflow,
    setWorkflows,
    store,
    workspaceState.activeId,
  ]);



  if (!isMounted) {
    return <div className="h-dvh w-full bg-background" />;
  }

  return (
    <div className="h-dvh w-full flex flex-col">
      <HomepageCommands onCreateTask={handleCreate} />
      <KanbanHeader
        onCreateTask={handleCreate}
        workspaceId={workspaceState.activeId ?? undefined}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
      />
      <TaskCreateDialog
        open={isDialogOpen}
        onOpenChange={handleDialogOpenChange}
        workspaceId={workspaceState.activeId}
        workflowId={kanban.workflowId}
        defaultStepId={activeSteps[0]?.id ?? null}
        steps={activeSteps.map((step) => ({ id: step.id, title: step.title, events: step.events }))}
        editingTask={
          editingTask
            ? {
              id: editingTask.id,
              title: editingTask.title,
              description: editingTask.description,
              workflowStepId: editingTask.workflowStepId,
              state: editingTask.state as BackendTask['state'],
              repositoryId: editingTask.repositoryId,
            }
            : null
        }
        onSuccess={handleDialogSuccess}
        initialValues={
          editingTask
            ? {
              title: editingTask.title,
              description: editingTask.description,
              state: editingTask.state as BackendTask['state'],
              repositoryId: editingTask.repositoryId,
            }
            : undefined
        }
        mode={editingTask ? 'edit' : 'create'}
      />
      {/* Workflow automation session creation dialog */}
      <TaskCreateDialog
        open={isWorkflowDialogOpen}
        onOpenChange={(open) => {
          setIsWorkflowDialogOpen(open);
          if (!open) setWorkflowAutomation(null);
        }}
        mode="session"
        workspaceId={workspaceState.activeId}
        workflowId={kanban.workflowId}
        defaultStepId={workflowAutomation?.workflowStep.id ?? null}
        steps={activeSteps.map((step) => ({ id: step.id, title: step.title, events: step.events }))}
        editingTask={null}
        onCreateSession={handleWorkflowSessionCreate}
        initialValues={{
          title: '',
          description: workflowAutomation?.taskDescription ?? '',
        }}
      />
      {/* Approval warning modal */}
      <AlertDialog open={!!moveError} onOpenChange={(open) => !open && setMoveError(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2">
              <IconAlertTriangle className="h-5 w-5 text-amber-500" />
              Approval Required
            </AlertDialogTitle>
            <AlertDialogDescription>
              {moveError?.message}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Dismiss</AlertDialogCancel>
            {moveError?.sessionId && (
              <AlertDialogAction onClick={handleGoToTask}>
                Go to Task
              </AlertDialogAction>
            )}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <SwimlaneContainer
        viewMode={kanbanViewMode || ''}
        workflowFilter={workflowsState.activeId}
        onPreviewTask={handleCardClick}
        onOpenTask={handleOpenTask}
        onEditTask={handleEdit}
        onDeleteTask={handleDelete}
        onMoveError={handleMoveError}
        onWorkflowAutomation={handleWorkflowAutomation}
        deletingTaskId={deletingTaskId}
        searchQuery={searchQuery}
        selectedRepositoryIds={userSettings.repositoryIds}
      />
    </div>
  );
}
