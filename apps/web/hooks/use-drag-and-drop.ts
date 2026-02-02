'use client';

import { useCallback, useMemo, useRef, useState } from 'react';
import { DragStartEvent, DragEndEvent } from '@dnd-kit/core';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useTaskActions } from '@/hooks/use-task-actions';
import type { Task } from '@/components/kanban-card';
import type { KanbanState } from '@/lib/state/slices';
import type { WorkflowStepDTO } from '@/lib/types/http';
import { getWebSocketClient } from '@/lib/ws/connection';

export type WorkflowAutomation = {
  taskId: string;
  sessionId: string | null;
  workflowStep: WorkflowStepDTO;
  taskDescription: string;
};

export type MoveTaskError = {
  message: string;
  taskId: string;
  sessionId: string | null;
};

export type DragAndDropOptions = {
  visibleTasks: Task[];
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  onMoveError?: (error: MoveTaskError) => void;
};

/**
 * Custom hook that extracts drag-and-drop logic from the KanbanBoard component.
 * Manages drag state and provides handlers for drag operations.
 *
 * @param options - Configuration options for drag and drop
 * @returns Object with drag state and drag operation handlers
 */
export function useDragAndDrop({
  visibleTasks,
  onWorkflowAutomation,
  onMoveError,
}: DragAndDropOptions) {
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const [isMovingTask, setIsMovingTask] = useState(false);
  const { moveTaskById } = useTaskActions();
  const kanban = useAppStore((state) => state.kanban);
  const store = useAppStoreApi();

  // Use refs for callbacks to avoid stale closures in performTaskMove
  const onWorkflowAutomationRef = useRef(onWorkflowAutomation);
  const onMoveErrorRef = useRef(onMoveError);
  onWorkflowAutomationRef.current = onWorkflowAutomation;
  onMoveErrorRef.current = onMoveError;

  /**
   * Core move logic shared by both drag-and-drop and menu-based moves.
   * Handles optimistic updates, API calls, workflow automation, and error rollback.
   */
  const performTaskMove = useCallback(
    async (task: Task, targetColumnId: string) => {
      const currentKanban = store.getState().kanban;
      if (!currentKanban.boardId) return;

      // Calculate position in target column
      const targetTasks = currentKanban.tasks
        .filter((t: KanbanState['tasks'][number]) => t.workflowStepId === targetColumnId && t.id !== task.id)
        .sort((a: KanbanState['tasks'][number], b: KanbanState['tasks'][number]) => a.position - b.position);
      const nextPosition = targetTasks.length;

      // Save original state for rollback
      const originalTasks = currentKanban.tasks;

      // Optimistic update
      store.getState().hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.map((t: KanbanState['tasks'][number]) =>
            t.id === task.id
              ? { ...t, workflowStepId: targetColumnId, position: nextPosition }
              : t
          ),
        },
      });

      try {
        setIsMovingTask(true);
        const response = await moveTaskById(task.id, {
          board_id: currentKanban.boardId,
          workflow_step_id: targetColumnId,
          position: nextPosition,
        });

        // Handle workflow automation if target step has auto_start_agent enabled
        if (response?.workflow_step?.auto_start_agent) {
          const sessionId = task.primarySessionId ?? null;

          if (sessionId) {
            // Task has a session - auto-start with workflow step config
            const client = getWebSocketClient();
            if (client) {
              try {
                await client.request(
                  'orchestrator.start',
                  {
                    task_id: task.id,
                    session_id: sessionId,
                    workflow_step_id: response.workflow_step.id,
                  },
                  15000
                );
              } catch (err) {
                console.error('Failed to auto-start session for workflow step:', err);
              }
            }
          } else {
            // No session - notify parent to show session creation dialog
            onWorkflowAutomationRef.current?.({
              taskId: task.id,
              sessionId: null,
              workflowStep: response.workflow_step,
              taskDescription: task.description ?? '',
            });
          }
        }
      } catch (error) {
        // Revert optimistic update on error
        store.getState().hydrate({
          kanban: {
            ...store.getState().kanban,
            tasks: originalTasks,
          },
        });
        // Show error to user with task context
        const message = error instanceof Error ? error.message : 'Failed to move task';
        onMoveErrorRef.current?.({
          message,
          taskId: task.id,
          sessionId: task.primarySessionId ?? null,
        });
      } finally {
        setIsMovingTask(false);
      }
    },
    [moveTaskById, store]
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;

      setActiveTaskId(null);
      if (!over) return;

      const taskId = active.id as string;
      const targetColumnId = over.id as string;

      if (!kanban.boardId || isMovingTask) return;

      // Find the task being moved from visibleTasks (which has session info)
      const movedTask = visibleTasks.find((t) => t.id === taskId);
      if (!movedTask) return;

      await performTaskMove(movedTask, targetColumnId);
    },
    [kanban.boardId, isMovingTask, visibleTasks, performTaskMove]
  );

  const handleDragCancel = useCallback(() => {
    setActiveTaskId(null);
  }, []);

  /**
   * Move a task to a specific column via menu action (alternative to drag and drop).
   */
  const moveTaskToColumn = useCallback(
    async (task: Task, targetColumnId: string) => {
      if (!kanban.boardId || isMovingTask) return;

      // Don't move if already in target column
      if (task.workflowStepId === targetColumnId) return;

      await performTaskMove(task, targetColumnId);
    },
    [kanban.boardId, isMovingTask, performTaskMove]
  );

  const activeTask = useMemo(
    () => visibleTasks.find((task) => task.id === activeTaskId) ?? null,
    [visibleTasks, activeTaskId]
  );

  return {
    activeTaskId,
    activeTask,
    isMovingTask,
    handleDragStart,
    handleDragEnd,
    handleDragCancel,
    moveTaskToColumn,
  };
}
