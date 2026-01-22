'use client';

import { useCallback, useState } from 'react';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useTaskActions } from '@/hooks/use-task-actions';
import type { Task } from '@/components/kanban-card';
import type { KanbanState } from '@/lib/state/slices';

/**
 * Custom hook that extracts task CRUD operations from the KanbanBoard component.
 * Manages dialog state and provides handlers for create, edit, and delete operations.
 *
 * @returns Object with dialog state and task operation handlers
 */
export function useTaskCRUD() {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const { deleteTaskById } = useTaskActions();
  const kanban = useAppStore((state) => state.kanban);
  const store = useAppStoreApi();

  const handleCreate = useCallback(() => {
    setEditingTask(null);
    setIsDialogOpen(true);
  }, []);

  const handleEdit = useCallback((task: Task) => {
    setEditingTask(task);
    setIsDialogOpen(true);
  }, []);

  const handleDelete = useCallback(
    async (task: Task) => {
      if (!kanban.boardId) return;

      // Optimistic update
      store.getState().hydrate({
        kanban: {
          ...kanban,
          tasks: kanban.tasks.filter((item: KanbanState['tasks'][number]) => item.id !== task.id),
        },
      });

      try {
        await deleteTaskById(task.id);
      } catch {
        // Ignore delete errors for now.
      }
    },
    [deleteTaskById, kanban, store]
  );

  const handleDialogOpenChange = useCallback((open: boolean) => {
    setIsDialogOpen(open);
    if (!open) {
      setEditingTask(null);
    }
  }, []);

  return {
    isDialogOpen,
    setIsDialogOpen,
    editingTask,
    setEditingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleDialogOpenChange,
  };
}
