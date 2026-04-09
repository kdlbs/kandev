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
  const { deleteTaskById, archiveTaskById, moveTaskById } = useTaskActions();
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

  const handleClearLane = useCallback(
    async (tasks: Task[]) => {
      if (!kanban.workflowId || tasks.length === 0) return;

      await Promise.all(tasks.map((task) => deleteTaskById(task.id)));

      const deletedIds = new Set(tasks.map((t) => t.id));
      store.getState().hydrate({
        kanban: {
          ...store.getState().kanban,
          tasks: store
            .getState()
            .kanban.tasks.filter((item: KanbanState["tasks"][number]) => !deletedIds.has(item.id)),
        },
      });
    },
    [deleteTaskById, kanban.workflowId, store],
  );

  const handleMoveLane = useCallback(
    async (tasks: Task[], targetStepId: string) => {
      if (!kanban.workflowId || tasks.length === 0) return;

      const currentTasks = store.getState().kanban.tasks;
      const movedIds = new Set(tasks.map((t) => t.id));
      const maxTargetPos = currentTasks
        .filter(
          (t: KanbanState["tasks"][number]) =>
            t.workflowStepId === targetStepId && !movedIds.has(t.id),
        )
        .reduce(
          (max: number, t: KanbanState["tasks"][number]) => Math.max(max, t.position ?? 0),
          -1,
        );

      await Promise.all(
        tasks.map((task, i) =>
          moveTaskById(task.id, {
            workflow_id: kanban.workflowId!,
            workflow_step_id: targetStepId,
            position: maxTargetPos + 1 + i,
          }),
        ),
      );

      store.getState().hydrate({
        kanban: {
          ...store.getState().kanban,
          tasks: store.getState().kanban.tasks.map((t: KanbanState["tasks"][number]) => {
            if (!movedIds.has(t.id)) return t;
            const idx = tasks.findIndex((m) => m.id === t.id);
            return { ...t, workflowStepId: targetStepId, position: maxTargetPos + 1 + idx };
          }),
        },
      });
    },
    [moveTaskById, kanban.workflowId, store],
  );

  const handleArchiveLane = useCallback(
    async (tasks: Task[]) => {
      if (!kanban.workflowId || tasks.length === 0) return;

      await Promise.all(tasks.map((task) => archiveTaskById(task.id)));

      const archivedIds = new Set(tasks.map((t) => t.id));
      store.getState().hydrate({
        kanban: {
          ...store.getState().kanban,
          tasks: store
            .getState()
            .kanban.tasks.filter((item: KanbanState["tasks"][number]) => !archivedIds.has(item.id)),
        },
      });
    },
    [archiveTaskById, kanban.workflowId, store],
  );

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
    handleClearLane,
    handleArchiveLane,
    handleMoveLane,
  };
}
