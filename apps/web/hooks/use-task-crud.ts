"use client";

import { useCallback, useState } from "react";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useKanbanSnapshotMutator } from "@/hooks/domains/kanban/use-kanban-snapshots";
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
  const { getSnapshots, setSnapshot } = useKanbanSnapshotMutator();

  // Optimistically drop a task from every workflow snapshot in the TQ cache.
  const removeTaskFromCache = useCallback(
    (taskId: string) => {
      for (const [wfId, snapshot] of Object.entries(getSnapshots())) {
        if (snapshot.tasks.some((t: KanbanState["tasks"][number]) => t.id === taskId)) {
          setSnapshot(wfId, {
            ...snapshot,
            tasks: snapshot.tasks.filter((t: KanbanState["tasks"][number]) => t.id !== taskId),
          });
        }
      }
    },
    [getSnapshots, setSnapshot],
  );

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
        removeTaskFromCache(task.id); // Update UI AFTER successful delete
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, removeTaskFromCache],
  );

  const handleArchive = useCallback(
    async (task: Task, opts?: { cascade?: boolean }) => {
      setArchivingTaskId(task.id);
      try {
        await archiveTaskById(task.id, opts);
        removeTaskFromCache(task.id); // Remove from kanban view AFTER successful archive
      } finally {
        setArchivingTaskId(null);
      }
    },
    [archiveTaskById, removeTaskFromCache],
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
