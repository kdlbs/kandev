'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Task } from './kanban-card';
import { TaskCreateDialog } from './task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { Task as BackendTask } from '@/lib/types/http';
import type { BoardState } from '@/lib/state/slices';
import { useDragAndDrop, type WorkflowAutomation, type MoveTaskError } from '@/hooks/use-drag-and-drop';
import { KanbanBoardGrid } from './kanban-board-grid';
import { KanbanHeader } from './kanban/kanban-header';
import { useKanbanData, useKanbanActions, useKanbanNavigation } from '@/hooks/domains/kanban';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
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

  // Get initial state for actions hook
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const boardsState = useAppStore((state) => state.boards);
  const setActiveBoard = useAppStore((state) => state.setActiveBoard);
  const setBoards = useAppStore((state) => state.setBoards);

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
    handleBoardChange,
    deletingTaskId,
  } = useKanbanActions({ kanban, workspaceState, boardsState });

  // Data fetching and derived state
  const {
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    activeColumns,
    visibleTasksWithSessions,
    isMounted,
    setTaskSessionAvailability,
  } = useKanbanData({
    onWorkspaceChange: handleWorkspaceChange,
    onBoardChange: handleBoardChange,
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

  // Drag and drop with workflow automation callback
  const { activeTask, handleDragStart, handleDragEnd, handleDragCancel, moveTaskToColumn } = useDragAndDrop({
    visibleTasks: visibleTasksWithSessions,
    onWorkflowAutomation: handleWorkflowAutomation,
    onMoveError: handleMoveError,
  });

  // Ref for userSettings to avoid reactive deps in board selection effect
  const userSettingsRef = useRef(userSettings);
  useEffect(() => {
    userSettingsRef.current = userSettings;
  });

  // Board selection effect
  useEffect(() => {
    const workspaceId = workspaceState.activeId;
    if (!workspaceId) {
      if (boardsState.items.length || boardsState.activeId) {
        setBoards([]);
        setActiveBoard(null);
      }
      return;
    }
    const settings = userSettingsRef.current;
    const workspaceBoards = boardsState.items.filter(
      (board: BoardState['items'][number]) => board.workspaceId === workspaceId
    );
    const desiredBoardId =
      (settings.boardId && workspaceBoards.some((board: BoardState['items'][number]) => board.id === settings.boardId)
        ? settings.boardId
        : workspaceBoards[0]?.id) ?? null;
    setActiveBoard(desiredBoardId);
    if (settings.loaded && desiredBoardId !== settings.boardId) {
      commitSettings({
        workspaceId,
        boardId: desiredBoardId,
        repositoryIds: settings.repositoryIds,
      });
    }
    if (!desiredBoardId) {
      store.getState().hydrate({
        kanban: { boardId: null, steps: [], tasks: [] },
      });
    }
  }, [
    boardsState.activeId,
    boardsState.items,
    commitSettings,
    setActiveBoard,
    setBoards,
    store,
    workspaceState.activeId,
  ]);



  if (!isMounted) {
    return <div className="h-dvh w-full bg-background" />;
  }

  return (
    <div className="h-dvh w-full flex flex-col">
      <KanbanHeader
        onCreateTask={handleCreate}
        workspaceId={workspaceState.activeId ?? undefined}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
      />
      <TaskCreateDialog
        key={isDialogOpen ? 'open' : 'closed'}
        open={isDialogOpen}
        onOpenChange={handleDialogOpenChange}
        workspaceId={workspaceState.activeId}
        boardId={kanban.boardId}
        defaultColumnId={activeColumns[0]?.id ?? null}
        columns={activeColumns.map((column) => ({ id: column.id, title: column.title, autoStartAgent: column.autoStartAgent }))}
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
        navigateOnSessionCreate={false}
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
        submitLabel={editingTask ? 'Update' : 'Create'}
      />
      {/* Workflow automation session creation dialog */}
      <TaskCreateDialog
        key={isWorkflowDialogOpen ? 'workflow-open' : 'workflow-closed'}
        open={isWorkflowDialogOpen}
        onOpenChange={(open) => {
          setIsWorkflowDialogOpen(open);
          if (!open) setWorkflowAutomation(null);
        }}
        mode="session"
        workspaceId={workspaceState.activeId}
        boardId={kanban.boardId}
        defaultColumnId={workflowAutomation?.workflowStep.id ?? null}
        columns={activeColumns.map((column) => ({ id: column.id, title: column.title, autoStartAgent: column.autoStartAgent }))}
        editingTask={null}
        onCreateSession={handleWorkflowSessionCreate}
        initialValues={{
          title: '',
          description: workflowAutomation?.taskDescription ?? '',
        }}
        submitLabel="Start Agent"
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
      <KanbanBoardGrid
        columns={activeColumns}
        tasks={visibleTasksWithSessions}
        onPreviewTask={handleCardClick}
        onOpenTask={handleOpenTask}
        onEditTask={handleEdit}
        onDeleteTask={handleDelete}
        onMoveTask={moveTaskToColumn}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
        activeTask={activeTask}
        showMaximizeButton={enablePreviewOnClick}
        deletingTaskId={deletingTaskId}
        onCreateTask={handleCreate}
        isLoading={kanban.isLoading}
      />
    </div>
  );
}
