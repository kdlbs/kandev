"use client";

import { useCallback, useEffect, useLayoutEffect, useReducer, useRef, type RefObject } from "react";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useAppStoreApi } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";

/** @internal Exported for reuse by the sidebar multi-select hook. */
export function useTaskMultiSelectStore() {
  const store = useAppStoreApi();

  const removeTasksFromStore = useCallback(
    (ids: Set<string>) => {
      const state = store.getState();
      // Remove from single-workflow view
      const currentKanban = state.kanban;
      state.hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.filter((t: KanbanState["tasks"][number]) => !ids.has(t.id)),
        },
      });
      // Remove from multi-workflow snapshots
      for (const [wfId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
        const affected = snapshot.tasks.some((t: KanbanState["tasks"][number]) => ids.has(t.id));
        if (affected) {
          state.setWorkflowSnapshot(wfId, {
            ...snapshot,
            tasks: snapshot.tasks.filter((t: KanbanState["tasks"][number]) => !ids.has(t.id)),
          });
        }
      }
    },
    [store],
  );

  const applyMoveInStore = useCallback(
    (succeededIds: Set<string>, targetStepId: string) => {
      const state = store.getState();
      // Update single-workflow view
      const currentKanban = state.kanban;
      state.hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.map((t: KanbanState["tasks"][number]) =>
            succeededIds.has(t.id) ? { ...t, workflowStepId: targetStepId } : t,
          ),
        },
      });
      // Update multi-workflow snapshots
      for (const [wfId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
        const affected = snapshot.tasks.filter((t: KanbanState["tasks"][number]) =>
          succeededIds.has(t.id),
        );
        if (affected.length > 0) {
          state.setWorkflowSnapshot(wfId, {
            ...snapshot,
            tasks: snapshot.tasks.map((t: KanbanState["tasks"][number]) =>
              succeededIds.has(t.id) ? { ...t, workflowStepId: targetStepId } : t,
            ),
          });
        }
      }
    },
    [store],
  );

  const getWorkflowIdForTask = useCallback(
    (taskId: string): string | null => {
      const snapshots = store.getState().kanbanMulti.snapshots;
      for (const [wfId, snapshot] of Object.entries(snapshots)) {
        if (snapshot.tasks.some((t: KanbanState["tasks"][number]) => t.id === taskId)) {
          return wfId;
        }
      }
      return store.getState().kanban.workflowId;
    },
    [store],
  );

  return { removeTasksFromStore, applyMoveInStore, getWorkflowIdForTask };
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
  removeTasksFromStore,
  applyMoveInStore,
  getWorkflowIdForTask,
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
  removeTasksFromStore: (ids: Set<string>) => void;
  applyMoveInStore: (ids: Set<string>, stepId: string) => void;
  getWorkflowIdForTask: (id: string) => string | null;
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
        removeTasksFromStore(succeeded);
        const failed = new Set(idList.filter((_, i) => results[i].status === "rejected"));
        setSelectedIds(failed);
        if (failed.size === 0) setIsMultiSelectEnabled(false);
      } finally {
        setBusy(false);
      }
    },
    [removeTasksFromStore, selectedIdsRef, setIsMultiSelectEnabled, setSelectedIds],
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
          const wfId = getWorkflowIdForTask(id) ?? workflowId;
          if (!wfId) return Promise.reject(new Error("no workflow"));
          return moveTaskById(id, {
            workflow_id: wfId,
            workflow_step_id: targetStepId,
            position: i,
          });
        }),
      );
      const succeeded = new Set(idList.filter((_, i) => results[i].status === "fulfilled"));
      applyMoveInStore(succeeded, targetStepId);
    },
    [workflowId, moveTaskById, applyMoveInStore, getWorkflowIdForTask, selectedIdsRef],
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
 * Union-select every id from the anchor to `taskId` (inclusive) within
 * `orderedIds`. When there is no valid anchor in `orderedIds` (first shift
 * click, or anchor lives in a different column), fall back to selecting just
 * `taskId` and making it the new anchor.
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
      if (next.has(action.taskId)) next.delete(action.taskId);
      else next.add(action.taskId);
      return { ...state, selectedIds: next, anchorId: action.taskId };
    }
    case "select_range":
      return applyRangeSelect(state, action.taskId, action.orderedIds);
    case "set_selected":
      return {
        ...state,
        selectedIds: action.ids,
        anchorId: action.ids.size ? state.anchorId : null,
      };
    case "set_enabled":
      return { ...state, isMultiSelectEnabled: action.value };
    case "set_deleting":
      return { ...state, isDeleting: action.value };
    case "set_archiving":
      return { ...state, isArchiving: action.value };
  }
}

export function useTaskMultiSelect(workflowId: string | null) {
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
  const { removeTasksFromStore, applyMoveInStore, getWorkflowIdForTask } =
    useTaskMultiSelectStore();

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
    removeTasksFromStore,
    applyMoveInStore,
    getWorkflowIdForTask,
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
