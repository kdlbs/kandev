"use client";

import { useCallback, useState } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useTaskActions, type TaskActionOptions } from "@/hooks/use-task-actions";
import { useTaskRemoval, useTaskRemovalSuccessNotifier } from "@/hooks/use-task-removal";
import type { Task } from "@/components/kanban-card";

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
  const store = useAppStoreApi();
  const notifySuccess = useTaskRemovalSuccessNotifier();
  const { runTaskRemoval } = useTaskRemoval({ store, notifySuccess });

  const handleCreate = useCallback(() => {
    setEditingTask(null);
    setIsDialogOpen(true);
  }, []);

  const handleEdit = useCallback((task: Task) => {
    setEditingTask(task);
    setIsDialogOpen(true);
  }, []);

  const handleDelete = useCallback(
    async (task: Task, opts?: TaskActionOptions) => {
      setDeletingTaskId(task.id);
      try {
        await runTaskRemoval(
          "delete",
          { taskId: task.id, mutate: () => deleteTaskById(task.id, opts) },
          { cascade: opts?.cascade },
        );
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, runTaskRemoval],
  );

  const handleArchive = useCallback(
    async (task: Task, opts?: TaskActionOptions) => {
      setArchivingTaskId(task.id);
      try {
        await runTaskRemoval(
          "archive",
          { taskId: task.id, mutate: () => archiveTaskById(task.id, opts) },
          { cascade: opts?.cascade },
        );
      } finally {
        setArchivingTaskId(null);
      }
    },
    [archiveTaskById, runTaskRemoval],
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
