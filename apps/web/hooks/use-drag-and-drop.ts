'use client';

import { useCallback, useMemo, useState } from 'react';
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

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;

      setActiveTaskId(null);
      if (!over) return;

      const taskId = active.id as string;
      const newStatus = over.id as string;

      if (!kanban.boardId || isMovingTask) {
        return;
      }

      // Find the task being moved from visibleTasks (which has session info)
      const movedTask = visibleTasks.find((t) => t.id === taskId);
      if (!movedTask) return;

      const targetTasks = kanban.tasks
        .filter((task: KanbanState['tasks'][number]) => task.workflowStepId === newStatus && task.id !== taskId)
        .sort((a: KanbanState['tasks'][number], b: KanbanState['tasks'][number]) => a.position - b.position);
      const nextPosition = targetTasks.length;

      // Save original state for rollback
      const originalTasks = kanban.tasks;

      // Optimistic update
      store.getState().hydrate({
        kanban: {
          ...kanban,
          tasks: kanban.tasks.map((task: KanbanState['tasks'][number]) =>
            task.id === taskId
              ? { ...task, workflowStepId: newStatus, position: nextPosition }
              : task
          ),
        },
      });

      try {
        setIsMovingTask(true);
        const response = await moveTaskById(taskId, {
          board_id: kanban.boardId,
          workflow_step_id: newStatus,
          position: nextPosition,
        });

        // Check if target step has auto_start_agent enabled
        if (response?.workflow_step?.auto_start_agent) {
          const sessionId = movedTask.primarySessionId ?? null;

          if (sessionId) {
            // Task has a session - auto-start with workflow step config
            const client = getWebSocketClient();
            if (client) {
              try {
                await client.request(
                  'orchestrator.start',
                  {
                    task_id: taskId,
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
            onWorkflowAutomation?.({
              taskId,
              sessionId: null,
              workflowStep: response.workflow_step,
              taskDescription: movedTask.description ?? '',
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
        onMoveError?.({
          message,
          taskId,
          sessionId: movedTask.primarySessionId ?? null,
        });
      } finally {
        setIsMovingTask(false);
      }
    },
    [kanban, isMovingTask, moveTaskById, store, onWorkflowAutomation, onMoveError, visibleTasks]
  );

  const handleDragCancel = useCallback(() => {
    setActiveTaskId(null);
  }, []);

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
  };
}
