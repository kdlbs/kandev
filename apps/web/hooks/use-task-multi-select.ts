"use client";

import { useCallback, useEffect, useLayoutEffect, useReducer, useRef, type RefObject } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { WorkflowSnapshotData } from "@/lib/state/slices";
import {
  removeTasksFromWorkflowSnapshotQueries,
  updateWorkflowSnapshotQueries,
} from "@/lib/query/workflow-snapshot-cache";

function applyMoveInQuerySnapshots(
  queryClient: QueryClient,
  succeededIds: Set<string>,
  targetStepId: string,
): void {
  updateWorkflowSnapshotQueries(queryClient, (snapshot) => {
    if (!snapshot.tasks.some((task) => succeededIds.has(task.id))) return snapshot;
    return {
      ...snapshot,
      tasks: snapshot.tasks.map((task) =>
        succeededIds.has(task.id) ? { ...task, workflow_step_id: targetStepId } : task,
      ),
    };
  });
}

function getWorkflowIdForTask(
  snapshots: Record<string, WorkflowSnapshotData>,
  taskId: string,
  fallbackWorkflowId: string | null,
): string | null {
  for (const [workflowId, snapshot] of Object.entries(snapshots)) {
    if (snapshot.tasks.some((task) => task.id === taskId)) return workflowId;
  }
  return fallbackWorkflowId;
}

function useBulkOperations({
  workflowId,
  selectedIdsRef,
  setSelectedIds,
  setIsDeleting,
  setIsArchiving,
  setIsMultiSelectEnabled,
  moveTaskById,
  deleteTaskById,
  archiveTaskById,
  removeTasksFromSnapshots,
  applyMoveInSnapshots,
  resolveWorkflowIdForTask,
}: {
  workflowId: string | null;
  selectedIdsRef: RefObject<Set<string>>;
  setSelectedIds: (ids: Set<string>) => void;
  setIsDeleting: (v: boolean) => void;
  setIsArchiving: (v: boolean) => void;
  setIsMultiSelectEnabled: (v: boolean) => void;
  moveTaskById: ReturnType<typeof useTaskActions>["moveTaskById"];
  deleteTaskById: ReturnType<typeof useTaskActions>["deleteTaskById"];
  archiveTaskById: ReturnType<typeof useTaskActions>["archiveTaskById"];
  removeTasksFromSnapshots: (ids: Set<string>) => void;
  applyMoveInSnapshots: (ids: Set<string>, stepId: string) => void;
  resolveWorkflowIdForTask: (id: string) => string | null;
}) {
  const runBulk = useCallback(
    async (
      per: (id: string, opts?: { cascade?: boolean }) => Promise<void>,
      setBusy: (v: boolean) => void,
      opts?: { cascade?: boolean },
    ) => {
      const ids = selectedIdsRef.current;
      if (!ids || ids.size === 0) return;
      setBusy(true);
      try {
        const idList = [...ids];
        const results = await Promise.allSettled(idList.map((id) => per(id, opts)));
        const succeeded = new Set(idList.filter((_, i) => results[i].status === "fulfilled"));
        removeTasksFromSnapshots(succeeded);
        const failed = new Set(idList.filter((_, i) => results[i].status === "rejected"));
        setSelectedIds(failed);
        if (failed.size === 0) setIsMultiSelectEnabled(false);
      } finally {
        setBusy(false);
      }
    },
    [removeTasksFromSnapshots, selectedIdsRef, setIsMultiSelectEnabled, setSelectedIds],
  );

  const bulkDelete = useCallback(
    (opts?: { cascade?: boolean }) => runBulk(deleteTaskById, setIsDeleting, opts),
    [runBulk, deleteTaskById, setIsDeleting],
  );

  const bulkArchive = useCallback(
    (opts?: { cascade?: boolean }) => runBulk(archiveTaskById, setIsArchiving, opts),
    [runBulk, archiveTaskById, setIsArchiving],
  );

  const bulkMove = useCallback(
    async (targetStepId: string) => {
      const idList = [...(selectedIdsRef.current ?? [])];
      if (idList.length === 0) return;
      const results = await Promise.allSettled(
        idList.map((id, i) => {
          const wfId = resolveWorkflowIdForTask(id) ?? workflowId;
          if (!wfId) return Promise.reject(new Error("no workflow"));
          return moveTaskById(id, {
            workflow_id: wfId,
            workflow_step_id: targetStepId,
            position: i,
          });
        }),
      );
      const succeeded = new Set(idList.filter((_, i) => results[i].status === "fulfilled"));
      applyMoveInSnapshots(succeeded, targetStepId);
    },
    [workflowId, moveTaskById, applyMoveInSnapshots, resolveWorkflowIdForTask, selectedIdsRef],
  );

  return { bulkDelete, bulkArchive, bulkMove };
}

type MultiSelectState = {
  selectedIds: Set<string>;
  isMultiSelectEnabled: boolean;
  isDeleting: boolean;
  isArchiving: boolean;
};

type MultiSelectAction =
  | { type: "reset" }
  | { type: "toggle_select"; taskId: string }
  | { type: "set_selected"; ids: Set<string> }
  | { type: "set_enabled"; value: boolean }
  | { type: "set_deleting"; value: boolean }
  | { type: "set_archiving"; value: boolean };

/** @internal Exported for testing. */
export const INITIAL_STATE: MultiSelectState = {
  selectedIds: new Set(),
  isMultiSelectEnabled: false,
  isDeleting: false,
  isArchiving: false,
};

/** @internal Exported for testing. */
export function multiSelectReducer(
  state: MultiSelectState,
  action: MultiSelectAction,
): MultiSelectState {
  switch (action.type) {
    case "reset":
      return INITIAL_STATE;
    case "toggle_select": {
      const next = new Set(state.selectedIds);
      if (next.has(action.taskId)) next.delete(action.taskId);
      else next.add(action.taskId);
      return { ...state, selectedIds: next };
    }
    case "set_selected":
      return { ...state, selectedIds: action.ids };
    case "set_enabled":
      return { ...state, isMultiSelectEnabled: action.value };
    case "set_deleting":
      return { ...state, isDeleting: action.value };
    case "set_archiving":
      return { ...state, isArchiving: action.value };
  }
}

export function useTaskMultiSelect(
  workflowId: string | null,
  snapshots: Record<string, WorkflowSnapshotData> = {},
) {
  const queryClient = useQueryClient();
  const [state, dispatch] = useReducer(multiSelectReducer, INITIAL_STATE);
  const { selectedIds, isMultiSelectEnabled, isDeleting, isArchiving } = state;
  const selectedIdsRef = useRef(selectedIds);
  useLayoutEffect(() => {
    selectedIdsRef.current = selectedIds;
  });
  const isProcessing = isDeleting || isArchiving;

  const setSelectedIds = useCallback(
    (ids: Set<string>) => dispatch({ type: "set_selected", ids }),
    [],
  );
  const setIsMultiSelectEnabled = useCallback(
    (value: boolean) => dispatch({ type: "set_enabled", value }),
    [],
  );
  const setIsDeleting = useCallback(
    (value: boolean) => dispatch({ type: "set_deleting", value }),
    [],
  );
  const setIsArchiving = useCallback(
    (value: boolean) => dispatch({ type: "set_archiving", value }),
    [],
  );

  useEffect(() => {
    dispatch({ type: "reset" });
  }, [workflowId]);

  const { moveTaskById, deleteTaskById, archiveTaskById } = useTaskActions();
  const removeTasksFromSnapshots = useCallback(
    (ids: Set<string>) => removeTasksFromWorkflowSnapshotQueries(queryClient, ids),
    [queryClient],
  );
  const applyMoveInSnapshots = useCallback(
    (ids: Set<string>, stepId: string) => applyMoveInQuerySnapshots(queryClient, ids, stepId),
    [queryClient],
  );
  const resolveWorkflowIdForTask = useCallback(
    (taskId: string) => getWorkflowIdForTask(snapshots, taskId, workflowId),
    [snapshots, workflowId],
  );

  const toggleSelect = useCallback(
    (taskId: string) => dispatch({ type: "toggle_select", taskId }),
    [],
  );

  const enableMultiSelect = useCallback(
    () => setIsMultiSelectEnabled(true),
    [setIsMultiSelectEnabled],
  );

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
    setIsMultiSelectEnabled(false);
  }, [setSelectedIds, setIsMultiSelectEnabled]);

  const toggleMultiSelect = useCallback(() => {
    if (isMultiSelectEnabled || selectedIds.size > 0) {
      setSelectedIds(new Set());
      setIsMultiSelectEnabled(false);
    } else {
      setIsMultiSelectEnabled(true);
    }
  }, [isMultiSelectEnabled, selectedIds, setSelectedIds, setIsMultiSelectEnabled]);

  const { bulkDelete, bulkArchive, bulkMove } = useBulkOperations({
    workflowId,
    selectedIdsRef,
    setSelectedIds,
    setIsDeleting,
    setIsArchiving,
    setIsMultiSelectEnabled,
    moveTaskById,
    deleteTaskById,
    archiveTaskById,
    removeTasksFromSnapshots,
    applyMoveInSnapshots,
    resolveWorkflowIdForTask,
  });

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
