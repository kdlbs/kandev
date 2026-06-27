"use client";

import { useCallback, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { Task } from "@/components/kanban-card";
import { removeTasksFromWorkflowSnapshotQueries } from "@/lib/query/workflow-snapshot-cache";

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
  const queryClient = useQueryClient();

  const handleCreate = useCallback(() => {
    setEditingTask(null);
    setIsDialogOpen(true);
  }, []);

  const handleEdit = useCallback((task: Task) => {
    setEditingTask(task);
    setIsDialogOpen(true);
  }, []);

  const handleDelete = useCallback(
    async (task: Task, opts?: { cascade?: boolean }) => {
      setDeletingTaskId(task.id);
      try {
        await deleteTaskById(task.id, opts);
        removeTasksFromWorkflowSnapshotQueries(queryClient, new Set([task.id]));
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, queryClient],
  );

  const handleArchive = useCallback(
    async (task: Task, opts?: { cascade?: boolean }) => {
      setArchivingTaskId(task.id);
      try {
        await archiveTaskById(task.id, opts);
        removeTasksFromWorkflowSnapshotQueries(queryClient, new Set([task.id]));
      } finally {
        setArchivingTaskId(null);
      }
    },
    [archiveTaskById, queryClient],
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
