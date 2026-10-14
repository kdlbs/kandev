"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSidebarMultiSelect } from "@/hooks/use-sidebar-multi-select";
import {
  computeMixedWorkflowSelection,
  flattenVisibleTaskIds,
  sortIdsByVisibleOrder,
  type GroupedSidebarList,
} from "@/lib/sidebar/apply-view";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import type { TaskActionOptions } from "@/hooks/use-task-actions";

type BulkConfirmState = { ids: string[]; executorTypes: Array<string | null | undefined> };

/**
 * Owns a bulk confirm-dialog's state and runs `run` (archive or delete) on
 * confirm. `run` already surfaces its own failures, so the catch here is
 * defensive; `finally` guarantees the dialog closes.
 * @internal Exported for unit testing.
 */
export function useBulkConfirmDialog(
  displayTasks: Array<{ id: string; remoteExecutorType?: string | null }>,
  run: (ids: string[], opts?: TaskActionOptions) => Promise<void>,
) {
  const [state, setState] = useState<BulkConfirmState | null>(null);

  const open = useCallback(
    (ids: string[]) => {
      const byId = new Map(displayTasks.map((t) => [t.id, t.remoteExecutorType]));
      setState({ ids, executorTypes: ids.map((id) => byId.get(id) ?? null) });
    },
    [displayTasks],
  );

  const confirm = useCallback(
    async ({
      cascade,
      discardWorktreeChanges,
      confirmationId,
    }: TaskActionOptions & { cascade: boolean }) => {
      if (!state) return;
      try {
        await run(state.ids, { cascade, discardWorktreeChanges, confirmationId });
      } catch (error) {
        console.error("Bulk action failed:", error);
      } finally {
        setState(null);
      }
    },
    [state, run],
  );

  return { state, setState, open, confirm };
}

type Multi = ReturnType<typeof useSidebarMultiSelect>;

/**
 * The selection-lifecycle effects (Escape-to-clear, prune-hidden) and the
 * memoized range/move/pin callbacks threaded into `TaskSwitcher`.
 */
export function useSelectionHandlers(args: {
  multiSelect: Multi;
  pinTasks: (ids: string[]) => void;
  unpinTasks: (ids: string[]) => void;
  pinnedTaskIds: string[];
  visibleTaskIds: string[];
  movableSelectedIds: Set<string>;
  workflowIdByTaskId?: ReadonlyMap<string, string>;
  selectionScopeKey?: string;
}) {
  const {
    multiSelect,
    pinTasks,
    unpinTasks,
    pinnedTaskIds,
    visibleTaskIds,
    movableSelectedIds,
    workflowIdByTaskId,
    selectionScopeKey,
  } = args;
  const { isSelecting, clearSelection, selectRange, bulkMove } = multiSelect;

  // Escape clears an active selection.
  useEffect(() => {
    if (!isSelecting) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") clearSelection();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [isSelecting, clearSelection]);

  const selectionScopeRef = useRef(selectionScopeKey);
  useEffect(() => {
    if (selectionScopeRef.current === selectionScopeKey) return;
    selectionScopeRef.current = selectionScopeKey;
    clearSelection();
  }, [clearSelection, selectionScopeKey]);

  // Stable ref so TaskSwitcher's React.memo isn't defeated by a fresh closure.
  const onSelectTaskRange = useCallback(
    (taskId: string) => selectRange(taskId, visibleTaskIds),
    [selectRange, visibleTaskIds],
  );

  // Bulk move in rendered order so a backward range selection isn't scrambled at
  // the destination; drop workflow-less rows that can't be moved.
  const onBulkMove = useCallback(
    (ids: string[], targetWorkflowId: string, targetStepId: string) =>
      bulkMove(
        sortIdsByVisibleOrder(
          ids.filter((id) => movableSelectedIds.has(id)),
          visibleTaskIds,
        ),
        targetWorkflowId,
        targetStepId,
        workflowIdByTaskId,
      ),
    [bulkMove, visibleTaskIds, movableSelectedIds, workflowIdByTaskId],
  );

  const onBulkPin = useCallback(
    (ids: string[]) => {
      const orderedIds = sortIdsByVisibleOrder(ids, visibleTaskIds);
      const pinnedSet = new Set(pinnedTaskIds);
      const allPinned = orderedIds.length > 0 && orderedIds.every((id) => pinnedSet.has(id));
      // Pin in visible order so pinned rows keep the order the user saw.
      if (allPinned) unpinTasks(orderedIds);
      else pinTasks(orderedIds);
      clearSelection();
    },
    [pinTasks, unpinTasks, pinnedTaskIds, clearSelection, visibleTaskIds],
  );

  return { onSelectTaskRange, onBulkMove, onBulkPin };
}

/**
 * Sidebar multi-select wiring: selection state, the visible-order list shift
 * range-select walks, mixed-workflow detection (gates bulk "Move to step"), and
 * the bulk archive/delete confirm dialogs. Returns the props bundle for
 * `TaskSwitcher` plus the dialog state the panel renders via `SidebarBulkDialogs`.
 */
export function useSidebarSelection({
  workspaceId,
  grouped,
  collapsedGroups,
  collapsedSubtaskParents,
  displayTasks,
  selectionScopeKey,
}: {
  workspaceId: string | null;
  grouped: GroupedSidebarList;
  collapsedGroups: string[];
  collapsedSubtaskParents: string[];
  displayTasks: Array<{ id: string; workflowId?: string; remoteExecutorType?: string | null }>;
  selectionScopeKey?: string;
}) {
  const multiSelect = useSidebarMultiSelect(workspaceId);
  const { selectedIds, clearSelection, toggleSelect } = multiSelect;
  const pinTasks = useAppStore((s) => s.pinTasks);
  const unpinTasks = useAppStore((s) => s.unpinTasks);
  const pinnedTaskIds = useAppStore((s) => s.sidebarTaskPrefs.pinnedTaskIds);
  const selectedTaskDetailsRef = useRef(new Map<string, (typeof displayTasks)[number]>());
  const currentTasksById = useMemo(
    () => new Map(displayTasks.map((task) => [task.id, task])),
    [displayTasks],
  );
  const selectedTaskDetails = useMemo(() => {
    const cache = selectedTaskDetailsRef.current;
    for (const id of selectedIds) {
      const task = currentTasksById.get(id);
      if (task) cache.set(id, task);
    }
    for (const id of cache.keys()) {
      if (!selectedIds.has(id)) cache.delete(id);
    }
    return [...selectedIds].map((id) => cache.get(id)).filter((task) => task !== undefined);
  }, [currentTasksById, selectedIds]);
  const selectedWorkflowByTaskId = useMemo(
    () =>
      new Map(
        selectedTaskDetails
          .filter(
            (task): task is (typeof displayTasks)[number] & { workflowId: string } =>
              !!task.workflowId,
          )
          .map((task) => [task.id, task.workflowId]),
      ),
    [selectedTaskDetails],
  );
  const archiveDialog = useBulkConfirmDialog(selectedTaskDetails, multiSelect.bulkArchive);
  const deleteDialog = useBulkConfirmDialog(selectedTaskDetails, multiSelect.bulkDelete);

  const visibleTaskIds = useMemo(
    () => flattenVisibleTaskIds(grouped, collapsedGroups, collapsedSubtaskParents),
    [grouped, collapsedGroups, collapsedSubtaskParents],
  );

  // A workflow-less selected row (e.g. the archived placeholder) can't be moved,
  // so treat its presence as a mixed selection (disables "Move to step") and
  // filter such ids out of the actual move.
  const { isMixedWorkflowSelection, movableSelectedIds } = useMemo(
    () => computeMixedWorkflowSelection(selectedTaskDetails, selectedIds),
    [selectedTaskDetails, selectedIds],
  );

  const { onSelectTaskRange, onBulkMove, onBulkPin } = useSelectionHandlers({
    multiSelect,
    pinTasks,
    unpinTasks,
    pinnedTaskIds,
    visibleTaskIds,
    movableSelectedIds,
    workflowIdByTaskId: selectedWorkflowByTaskId,
    selectionScopeKey,
  });

  const switcherProps = {
    selectedTaskIds: selectedIds,
    onToggleSelectTask: toggleSelect,
    onSelectTaskRange,
    onBulkArchive: archiveDialog.open,
    onBulkDelete: deleteDialog.open,
    onBulkPin,
    onBulkMove,
    onClearSelection: clearSelection,
    isMixedWorkflowSelection,
  };

  return {
    switcherProps,
    archiveDialog,
    deleteDialog,
    isArchiving: multiSelect.isArchiving,
    isDeleting: multiSelect.isDeleting,
  };
}

export function SidebarBulkDialogs({
  selection,
}: {
  selection: ReturnType<typeof useSidebarSelection>;
}) {
  const { archiveDialog, deleteDialog } = selection;
  return (
    <>
      {archiveDialog.state && (
        <TaskArchiveConfirmDialog
          open
          onOpenChange={(o) => !o && archiveDialog.setState(null)}
          isBulkOperation
          count={archiveDialog.state.ids.length}
          taskIds={archiveDialog.state.ids}
          executorTypes={archiveDialog.state.executorTypes}
          isArchiving={selection.isArchiving}
          onConfirm={archiveDialog.confirm}
          confirmTestId="sidebar-bulk-archive-confirm"
        />
      )}
      {deleteDialog.state && (
        <TaskDeleteConfirmDialog
          open
          onOpenChange={(o) => !o && deleteDialog.setState(null)}
          isBulkOperation
          count={deleteDialog.state.ids.length}
          taskIds={deleteDialog.state.ids}
          executorTypes={deleteDialog.state.executorTypes}
          isDeleting={selection.isDeleting}
          onConfirm={deleteDialog.confirm}
          confirmTestId="sidebar-bulk-delete-confirm"
        />
      )}
    </>
  );
}
