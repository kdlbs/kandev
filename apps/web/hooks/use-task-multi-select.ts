"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useAppStoreApi } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";

function useTaskMultiSelectStore() {
  const store = useAppStoreApi();

  const removeTasksFromStore = useCallback(
    (ids: Set<string>) => {
      const currentKanban = store.getState().kanban;
      store.getState().hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.filter((t: KanbanState["tasks"][number]) => !ids.has(t.id)),
        },
      });
    },
    [store],
  );

  const applyMoveInStore = useCallback(
    (succeededIds: Set<string>, targetStepId: string) => {
      const currentKanban = store.getState().kanban;
      store.getState().hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.map((t: KanbanState["tasks"][number]) =>
            succeededIds.has(t.id) ? { ...t, workflowStepId: targetStepId } : t,
          ),
        },
      });
    },
    [store],
  );

  return { removeTasksFromStore, applyMoveInStore };
}

export function useTaskMultiSelect(workflowId: string | null) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const selectedIdsRef = useRef(selectedIds);
  selectedIdsRef.current = selectedIds;

  const [isMultiSelectEnabled, setIsMultiSelectEnabled] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isArchiving, setIsArchiving] = useState(false);
  const isProcessing = isDeleting || isArchiving;

  useEffect(() => {
    setSelectedIds(new Set());
    setIsMultiSelectEnabled(false);
    setIsDeleting(false);
    setIsArchiving(false);
  }, [workflowId]);

  const { moveTaskById, deleteTaskById, archiveTaskById } = useTaskActions();
  const { removeTasksFromStore, applyMoveInStore } = useTaskMultiSelectStore();

  const toggleSelect = useCallback((taskId: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(taskId)) {
        next.delete(taskId);
      } else {
        next.add(taskId);
      }
      return next;
    });
  }, []);

  const enableMultiSelect = useCallback(() => {
    setIsMultiSelectEnabled(true);
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
    setIsMultiSelectEnabled(false);
  }, []);

  const toggleMultiSelect = useCallback(() => {
    if (isMultiSelectEnabled || selectedIds.size > 0) {
      setSelectedIds(new Set());
      setIsMultiSelectEnabled(false);
    } else {
      setIsMultiSelectEnabled(true);
    }
  }, [isMultiSelectEnabled, selectedIds]);

  const bulkDelete = useCallback(async () => {
    const ids = selectedIdsRef.current;
    if (ids.size === 0) return;
    setIsDeleting(true);
    try {
      const idList = [...ids];
      const results = await Promise.allSettled(idList.map((id) => deleteTaskById(id)));
      const succeededIds = new Set(idList.filter((_, i) => results[i].status === "fulfilled"));
      removeTasksFromStore(succeededIds);
      setSelectedIds(new Set(idList.filter((_, i) => results[i].status === "rejected")));
    } finally {
      setIsDeleting(false);
    }
  }, [deleteTaskById, removeTasksFromStore]);

  const bulkArchive = useCallback(async () => {
    const ids = selectedIdsRef.current;
    if (ids.size === 0) return;
    setIsArchiving(true);
    try {
      const idList = [...ids];
      const results = await Promise.allSettled(idList.map((id) => archiveTaskById(id)));
      const succeededIds = new Set(idList.filter((_, i) => results[i].status === "fulfilled"));
      removeTasksFromStore(succeededIds);
      setSelectedIds(new Set(idList.filter((_, i) => results[i].status === "rejected")));
    } finally {
      setIsArchiving(false);
    }
  }, [archiveTaskById, removeTasksFromStore]);

  const bulkMove = useCallback(
    async (targetStepId: string) => {
      if (!workflowId) return;
      const idList = [...selectedIdsRef.current];
      if (idList.length === 0) return;
      const results = await Promise.allSettled(
        idList.map((id, i) =>
          moveTaskById(id, {
            workflow_id: workflowId,
            workflow_step_id: targetStepId,
            position: i,
          }),
        ),
      );
      const succeededIds = new Set(idList.filter((_, i) => results[i].status === "fulfilled"));
      applyMoveInStore(succeededIds, targetStepId);
    },
    [workflowId, moveTaskById, applyMoveInStore],
  );

  return {
    selectedIds,
    isMultiSelectMode: isMultiSelectEnabled || selectedIds.size > 0,
    isProcessing,
    enableMultiSelect,
    toggleMultiSelect,
    toggleSelect,
    clearSelection,
    bulkDelete,
    bulkArchive,
    bulkMove,
  };
}
