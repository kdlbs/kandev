"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useReducer,
  useRef,
  type Dispatch,
  type RefObject,
} from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { WorkflowSnapshotData } from "@/lib/state/slices";
import { sortIdsByCreatedDesc } from "@/lib/kanban/task-order";
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

function sortByDisplayOrder(
  snapshots: Record<string, WorkflowSnapshotData>,
  ids: string[],
): string[] {
  const taskById = new Map<string, { createdAt?: string }>();
  for (const snapshot of Object.values(snapshots)) {
    for (const task of snapshot.tasks) taskById.set(task.id, task);
  }
  return sortIdsByCreatedDesc(ids, taskById);
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
  sortSelectedIdsByDisplayOrder,
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
  sortSelectedIdsByDisplayOrder: (ids: string[]) => string[];
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
      // Move in board order so a backward range selection isn't reordered when
      // sequential positions are assigned below.
      const idList = sortSelectedIdsByDisplayOrder([...(selectedIdsRef.current ?? [])]);
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
    [
      workflowId,
      moveTaskById,
      applyMoveInSnapshots,
      resolveWorkflowIdForTask,
      selectedIdsRef,
      sortSelectedIdsByDisplayOrder,
    ],
  );

  return { bulkDelete, bulkArchive, bulkMove };
}

type MultiSelectState = {
  selectedIds: Set<string>;
  isMultiSelectEnabled: boolean;
  isDeleting: boolean;
  isArchiving: boolean;
  /**
   * The task that anchors a shift-click range selection — the last task the
   * user toggled/range-selected. `null` when there is no active anchor.
   */
  anchorId: string | null;
};

type MultiSelectAction =
  | { type: "reset" }
  | { type: "toggle_select"; taskId: string }
  | { type: "select_range"; taskId: string; orderedIds: string[] }
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
  anchorId: null,
};

/**
 * Pick a valid range anchor after the selection set is replaced wholesale: keep
 * the existing anchor if it survived, otherwise fall back to any remaining id
 * (or null when the selection is now empty).
 */
function realignAnchor(state: MultiSelectState, ids: Set<string>): string | null {
  if (ids.size === 0) return null;
  if (state.anchorId && ids.has(state.anchorId)) return state.anchorId;
  return ids.values().next().value ?? null;
}

/**
 * Union-select every id from the anchor to `taskId` (inclusive) within
 * `orderedIds`. When there is no valid anchor in `orderedIds` (first shift
 * click, or anchor lives in a different column), fall back to union-selecting
 * just `taskId` — the previous selection is preserved — and make it the new
 * anchor.
 */
function applyRangeSelect(
  state: MultiSelectState,
  taskId: string,
  orderedIds: string[],
): MultiSelectState {
  const anchor = state.anchorId;
  const anchorIdx = anchor ? orderedIds.indexOf(anchor) : -1;
  const targetIdx = orderedIds.indexOf(taskId);
  if (anchorIdx === -1 || targetIdx === -1) {
    const next = new Set(state.selectedIds);
    next.add(taskId);
    return { ...state, selectedIds: next, anchorId: taskId };
  }
  const [lo, hi] = anchorIdx < targetIdx ? [anchorIdx, targetIdx] : [targetIdx, anchorIdx];
  const next = new Set(state.selectedIds);
  for (let i = lo; i <= hi; i++) next.add(orderedIds[i]);
  return { ...state, selectedIds: next };
}

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
      const added = !next.has(action.taskId);
      if (added) next.add(action.taskId);
      else next.delete(action.taskId);
      // Adding anchors to the toggled task; removing realigns to a surviving id
      // (or clears the anchor when the selection is now empty) so a later
      // Shift+click can't range from a stale anchor.
      return {
        ...state,
        selectedIds: next,
        anchorId: added ? action.taskId : realignAnchor(state, next),
      };
    }
    case "select_range":
      return applyRangeSelect(state, action.taskId, action.orderedIds);
    case "set_selected":
      // Keep the range anchor pointing at a still-selected task. After a partial
      // bulk failure (selection replaced with the failed ids) the old anchor may
      // be gone, so realign to a remaining id rather than stranding the next
      // Shift+click on an invalid anchor.
      return { ...state, selectedIds: action.ids, anchorId: realignAnchor(state, action.ids) };
    case "set_enabled":
      return { ...state, isMultiSelectEnabled: action.value };
    case "set_deleting":
      return { ...state, isDeleting: action.value };
    case "set_archiving":
      return { ...state, isArchiving: action.value };
  }
}

function useMultiSelectDispatchers(dispatch: Dispatch<MultiSelectAction>) {
  const setSelectedIds = useCallback(
    (ids: Set<string>) => dispatch({ type: "set_selected", ids }),
    [dispatch],
  );
  const setIsMultiSelectEnabled = useCallback(
    (value: boolean) => dispatch({ type: "set_enabled", value }),
    [dispatch],
  );
  const setIsDeleting = useCallback(
    (value: boolean) => dispatch({ type: "set_deleting", value }),
    [dispatch],
  );
  const setIsArchiving = useCallback(
    (value: boolean) => dispatch({ type: "set_archiving", value }),
    [dispatch],
  );
  return { setSelectedIds, setIsMultiSelectEnabled, setIsDeleting, setIsArchiving };
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
  const { setSelectedIds, setIsMultiSelectEnabled, setIsDeleting, setIsArchiving } =
    useMultiSelectDispatchers(dispatch);

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
  const sortSelectedIdsByDisplayOrder = useCallback(
    (ids: string[]) => sortByDisplayOrder(snapshots, ids),
    [snapshots],
  );

  const toggleSelect = useCallback(
    (taskId: string) => dispatch({ type: "toggle_select", taskId }),
    [],
  );

  const selectRange = useCallback(
    (taskId: string, orderedIds: string[]) =>
      dispatch({ type: "select_range", taskId, orderedIds }),
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
    sortSelectedIdsByDisplayOrder,
  });

  return {
    selectedIds,
    isMultiSelectMode: isMultiSelectEnabled || selectedIds.size > 0,
    isProcessing,
    enableMultiSelect,
    toggleMultiSelect,
    toggleSelect,
    selectRange,
    clearSelection,
    bulkDelete,
    bulkArchive,
    bulkMove,
  };
}
