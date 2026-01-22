'use client';

import { useCallback, useMemo, useState } from 'react';
import { DragStartEvent, DragEndEvent } from '@dnd-kit/core';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useTaskActions } from '@/hooks/use-task-actions';
import type { Task } from '@/components/kanban-card';
import type { KanbanState } from '@/lib/state/slices';

/**
 * Custom hook that extracts drag-and-drop logic from the KanbanBoard component.
 * Manages drag state and provides handlers for drag operations.
 *
 * @param visibleTasks - The list of visible tasks (filtered by repository, etc.)
 * @returns Object with drag state and drag operation handlers
 */
export function useDragAndDrop(visibleTasks: Task[]) {
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

      const targetTasks = kanban.tasks
        .filter((task: KanbanState['tasks'][number]) => task.columnId === newStatus && task.id !== taskId)
        .sort((a: KanbanState['tasks'][number], b: KanbanState['tasks'][number]) => a.position - b.position);
      const nextPosition = targetTasks.length;

      // Optimistic update
      store.getState().hydrate({
        kanban: {
          ...kanban,
          tasks: kanban.tasks.map((task: KanbanState['tasks'][number]) =>
            task.id === taskId
              ? { ...task, columnId: newStatus, position: nextPosition }
              : task
          ),
        },
      });

      try {
        setIsMovingTask(true);
        await moveTaskById(taskId, {
          board_id: kanban.boardId,
          column_id: newStatus,
          position: nextPosition,
        });
      } catch {
        // Ignore move errors for now; WS updates or next snapshot will correct.
      } finally {
        setIsMovingTask(false);
      }
    },
    [kanban, isMovingTask, moveTaskById, store]
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
