"use client";

import { useCallback, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { Task } from "@/components/kanban-card";
import type { KanbanState } from "@/lib/state/slices";

/**
 * Custom hook that extracts task CRUD operations from the Kanban component.
 * Manages dialog state and provides handlers for create, edit, delete, and archive operations.
 *
 * @returns Object with dialog state and task operation handlers
 */
export function useTaskCRUD() {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const [archivingTaskId, setArchivingTaskId] = useState<string | null>(null);
  const { deleteTaskById, archiveTaskById } = useTaskActions();
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
      if (!kanban.workflowId) return;

      setDeletingTaskId(task.id);
      try {
        await deleteTaskById(task.id);

        // Update UI AFTER successful delete
        store.getState().hydrate({
          kanban: {
            ...store.getState().kanban,
            tasks: store
              .getState()
              .kanban.tasks.filter((item: KanbanState["tasks"][number]) => item.id !== task.id),
          },
        });
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, kanban.workflowId, store],
  );

  const handleArchive = useCallback(
    async (task: Task) => {
      if (!kanban.workflowId) return;

      setArchivingTaskId(task.id);
      try {
        await archiveTaskById(task.id);

        // Update UI AFTER successful archive - remove from kanban view
        store.getState().hydrate({
          kanban: {
            ...store.getState().kanban,
            tasks: store
              .getState()
              .kanban.tasks.filter((item: KanbanState["tasks"][number]) => item.id !== task.id),
          },
        });
      } finally {
        setArchivingTaskId(null);
      }
    },
    [archiveTaskById, kanban.workflowId, store],
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
    handleArchive,
    handleDialogOpenChange,
    deletingTaskId,
    archivingTaskId,
  };
}
