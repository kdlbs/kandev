"use client";

import { useCallback, useEffect, useLayoutEffect, useReducer, useRef, type RefObject } from "react";
import { useTaskActions, type TaskActionOptions } from "@/hooks/use-task-actions";
import { useTaskRemoval, useTaskRemovalSuccessNotifier } from "@/hooks/use-task-removal";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useAppStoreApi } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";
import { sortIdsByDisplayOrder, type DisplayOrderTask } from "@/lib/kanban/task-order";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { getEffectiveView } from "@/lib/kanban/view-registry";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import { taskMatchesPriorityFilter } from "@/lib/kanban/priority-filter-tokens";
import { selectWorkflowSwimlanes, type WorkflowLike } from "@/lib/kanban/workflow-swimlanes";
import type { AppState } from "@/lib/state/store";
import type { TaskPriority } from "@/lib/types/http";

/**
 * Builds a step-id to displayed-index lookup across the effective workflow
 * selection. Explicitly selected hidden workflows remain in that selection;
 * only their hidden steps are removed from the index.
 *
 * @internal Exported for testing.
 */
export function buildPipelineStepIndexOf(
  workflows: WorkflowLike[],
  snapshots: Record<string, { steps: Array<{ id: string; position: number }> }>,
  hiddenWorkflowStepIds: Record<string, string[]>,
  workflowFilter: string | null | undefined = null,
): (stepId: string | undefined) => number {
  const indexByStepId = new Map<string, number>();
  let offset = 0;
  for (const workflow of selectWorkflowSwimlanes(workflowFilter, workflows, snapshots)) {
    const snapshot = snapshots[workflow.id];
    if (!snapshot) continue;
    const hidden = new Set(hiddenWorkflowStepIds[workflow.id] ?? []);
    const displaySteps = sortWorkflowStepsByPosition(snapshot.steps).filter(
      (step) => !hidden.has(step.id),
    );
    displaySteps.forEach((step, index) => indexByStepId.set(step.id, offset + index));
    offset += displaySteps.length;
  }
  return (stepId) => (stepId !== undefined ? (indexByStepId.get(stepId) ?? Infinity) : Infinity);
}

function buildTaskById(state: AppState): Map<string, DisplayOrderTask> {
  const taskById = new Map<string, DisplayOrderTask>();
  for (const snap of Object.values(state.kanbanMulti.snapshots)) {
    for (const task of snap.tasks) taskById.set(task.id, task);
  }
  for (const task of state.kanban.tasks) if (!taskById.has(task.id)) taskById.set(task.id, task);
  return taskById;
}

/** @internal Exported for testing. */
export function filterIdsByPriorityFilter(
  ids: string[],
  taskById: Map<string, DisplayOrderTask>,
  priorityFilterTokens: TaskPriority[],
): string[] {
  if (priorityFilterTokens.length === 0) return ids;
  return ids.filter((id) => {
    const task = taskById.get(id);
    return !task || taskMatchesPriorityFilter(task.priority ?? undefined, priorityFilterTokens);
  });
}

type AppStoreApi = ReturnType<typeof useAppStoreApi>;

function removeTasksFromStoreImpl(store: AppStoreApi, ids: Set<string>) {
  const state = store.getState();
  const currentKanban = state.kanban;
  state.hydrate({
    kanban: {
      ...currentKanban,
      tasks: currentKanban.tasks.filter((task: KanbanState["tasks"][number]) => !ids.has(task.id)),
    },
  });
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
    if (!snapshot.tasks.some((task: KanbanState["tasks"][number]) => ids.has(task.id))) continue;
    state.setWorkflowSnapshot(workflowId, {
      ...snapshot,
      tasks: snapshot.tasks.filter((task: KanbanState["tasks"][number]) => !ids.has(task.id)),
    });
  }
}

function applyMoveInStoreImpl(store: AppStoreApi, succeededIds: Set<string>, targetStepId: string) {
  const state = store.getState();
  const currentKanban = state.kanban;
  state.hydrate({
    kanban: {
      ...currentKanban,
      tasks: currentKanban.tasks.map((task: KanbanState["tasks"][number]) =>
        succeededIds.has(task.id) ? { ...task, workflowStepId: targetStepId } : task,
      ),
    },
  });
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
    if (!snapshot.tasks.some((task: KanbanState["tasks"][number]) => succeededIds.has(task.id))) {
      continue;
    }
    state.setWorkflowSnapshot(workflowId, {
      ...snapshot,
      tasks: snapshot.tasks.map((task: KanbanState["tasks"][number]) =>
        succeededIds.has(task.id) ? { ...task, workflowStepId: targetStepId } : task,
      ),
    });
  }
}

/** @internal Exported for reuse by the sidebar multi-select hook. */
export function useTaskMultiSelectStore() {
  const store = useAppStoreApi();
  const { isMobile } = useResponsiveBreakpoint();

  const removeTasksFromStore = useCallback(
    (ids: Set<string>) => removeTasksFromStoreImpl(store, ids),
    [store],
  );
  const applyMoveInStore = useCallback(
    (ids: Set<string>, targetStepId: string) => applyMoveInStoreImpl(store, ids, targetStepId),
    [store],
  );
  const getWorkflowIdForTask = useCallback(
    (taskId: string): string | null => {
      const snapshots = store.getState().kanbanMulti.snapshots;
      for (const [workflowId, snapshot] of Object.entries(snapshots)) {
        if (snapshot.tasks.some((task: KanbanState["tasks"][number]) => task.id === taskId)) {
          return workflowId;
        }
      }
      return store.getState().kanban.workflowId;
    },
    [store],
  );

  const sortByDisplayOrder = useCallback(
    (ids: string[]): string[] => {
      const state = store.getState();
      if (!state.userSettings) return ids;
      const sortToken = state.userSettings.kanbanSort ?? "created_desc";
      const isPipelineView =
        getEffectiveView(state.userSettings.kanbanViewMode ?? "", isMobile).id === "graph2";
      const stepIndexOf = isPipelineView
        ? buildPipelineStepIndexOf(
            state.workflows.items,
            state.kanbanMulti.snapshots,
            state.userSettings.hiddenWorkflowStepIds ?? {},
            state.workflows.activeId,
          )
        : undefined;
      return sortIdsByDisplayOrder(ids, buildTaskById(state), {
        sortToken,
        isPipelineView,
        stepIndexOf,
      });
    },
    [isMobile, store],
  );
  const eligibleSelectedIds = useCallback(
    (ids: string[]): string[] => {
      const state = store.getState();
      if (!state.userSettings) return ids;
      return filterIdsByPriorityFilter(
        ids,
        buildTaskById(state),
        state.userSettings.kanbanPriorityFilterTokens ?? [],
      );
    },
    [store],
  );

  return {
    removeTasksFromStore,
    applyMoveInStore,
    getWorkflowIdForTask,
    sortByDisplayOrder,
    eligibleSelectedIds,
  };
}

type RunBulkActionOptions = {
  action: "delete" | "archive";
  ids: string[];
  eligibleSelectedIds: (ids: string[]) => string[];
  per: (id: string, opts?: TaskActionOptions) => Promise<void>;
  runTaskRemovalBatch: ReturnType<typeof useTaskRemoval>["runTaskRemovalBatch"];
  removeTasksFromStore: (ids: Set<string>) => void;
  setSelectedIds: (ids: Set<string>) => void;
  setIsMultiSelectEnabled: (v: boolean) => void;
  setBusy: (v: boolean) => void;
  opts?: TaskActionOptions;
};

async function runBulkAction({
  action,
  ids,
  eligibleSelectedIds,
  per,
  runTaskRemovalBatch,
  removeTasksFromStore,
  setSelectedIds,
  setIsMultiSelectEnabled,
  setBusy,
  opts,
}: RunBulkActionOptions): Promise<void> {
  if (ids.length === 0) return;
  const idList = eligibleSelectedIds(ids);
  const hidden = ids.filter((id) => !idList.includes(id));
  if (idList.length === 0) {
    setSelectedIds(new Set(hidden));
    return;
  }
  setBusy(true);
  try {
    const result = await runTaskRemovalBatch(
      action,
      idList.map((id) => ({ taskId: id, mutate: () => per(id, opts) })),
      { cascade: opts?.cascade },
    );
    if (result.skipped) return;
    const succeeded = new Set(result.succeededTaskIds);
    removeTasksFromStore(succeeded);
    const failed = new Set([...result.failedTaskIds, ...hidden]);
    setSelectedIds(failed);
    if (failed.size === 0) setIsMultiSelectEnabled(false);
  } finally {
    setBusy(false);
  }
}

function useBulkOperations({
  workflowId,
  selectedIdsRef,
  setSelectedIds,
  setIsDeleting,
  setIsArchiving,
  setIsMultiSelectEnabled,
  deleteTaskById,
  archiveTaskById,
  runTaskRemovalBatch,
  removeTasksFromStore,
  applyMoveInStore,
  moveTasks,
  sortByDisplayOrder,
  eligibleSelectedIds,
}: {
  workflowId: string | null;
  selectedIdsRef: RefObject<Set<string>>;
  setSelectedIds: (ids: Set<string>) => void;
  setIsDeleting: (v: boolean) => void;
  setIsArchiving: (v: boolean) => void;
  setIsMultiSelectEnabled: (v: boolean) => void;
  deleteTaskById: ReturnType<typeof useTaskActions>["deleteTaskById"];
  archiveTaskById: ReturnType<typeof useTaskActions>["archiveTaskById"];
  runTaskRemovalBatch: ReturnType<typeof useTaskRemoval>["runTaskRemovalBatch"];
  removeTasksFromStore: (ids: Set<string>) => void;
  applyMoveInStore: (ids: Set<string>, stepId: string) => void;
  moveTasks: ReturnType<typeof useTaskWorkflowMove>;
  sortByDisplayOrder: (ids: string[]) => string[];
  eligibleSelectedIds: (ids: string[]) => string[];
}) {
  const runBulk = useCallback(
    (
      action: "delete" | "archive",
      per: (id: string, opts?: TaskActionOptions) => Promise<void>,
      setBusy: (v: boolean) => void,
      opts?: TaskActionOptions,
    ) =>
      runBulkAction({
        action,
        ids: [...(selectedIdsRef.current ?? [])],
        eligibleSelectedIds,
        per,
        runTaskRemovalBatch,
        removeTasksFromStore,
        setSelectedIds,
        setIsMultiSelectEnabled,
        setBusy,
        opts,
      }),
    [
      eligibleSelectedIds,
      removeTasksFromStore,
      runTaskRemovalBatch,
      selectedIdsRef,
      setIsMultiSelectEnabled,
      setSelectedIds,
    ],
  );

  const bulkDelete = useCallback(
    (opts?: TaskActionOptions) => runBulk("delete", deleteTaskById, setIsDeleting, opts),
    [runBulk, deleteTaskById, setIsDeleting],
  );

  const bulkArchive = useCallback(
    (opts?: TaskActionOptions) => runBulk("archive", archiveTaskById, setIsArchiving, opts),
    [runBulk, archiveTaskById, setIsArchiving],
  );

  const bulkMove = useCallback(
    async (targetStepId: string) => {
      const selected = [...(selectedIdsRef.current ?? [])];
      if (selected.length === 0 || !workflowId) return;
      const idList = sortByDisplayOrder(eligibleSelectedIds(selected));
      if (idList.length === 0) return;
      // Routed through the batch endpoint (mirrors use-sidebar-multi-select's
      // bulkMove) rather than fanning out one moveTaskById call per task: a
      // concurrent Promise.allSettled fan-out raced each task's arrival
      // position against the others', so the resulting order depended on
      // response timing rather than the selection (REQ-TASKS-KANBAN-TASK-REORDERING-001.29).
      // The server re-derives submission order from each task's current step.
      try {
        await moveTasks(idList, workflowId, targetStepId, "step");
        applyMoveInStore(new Set(idList), targetStepId);
      } catch {
        // useTaskWorkflowMove already shows the failure toast.
      }
    },
    [
      workflowId,
      moveTasks,
      applyMoveInStore,
      selectedIdsRef,
      sortByDisplayOrder,
      eligibleSelectedIds,
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

  const { deleteTaskById, archiveTaskById } = useTaskActions();
  const store = useAppStoreApi();
  const notifySuccess = useTaskRemovalSuccessNotifier();
  const { runTaskRemovalBatch } = useTaskRemoval({ store, notifySuccess });
  const { removeTasksFromStore, applyMoveInStore, sortByDisplayOrder, eligibleSelectedIds } =
    useTaskMultiSelectStore();
  const moveTasks = useTaskWorkflowMove();

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
    deleteTaskById,
    archiveTaskById,
    runTaskRemovalBatch,
    removeTasksFromStore,
    applyMoveInStore,
    moveTasks,
    sortByDisplayOrder,
    eligibleSelectedIds,
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
