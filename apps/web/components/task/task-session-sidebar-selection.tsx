"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useSidebarMultiSelect } from "@/hooks/use-sidebar-multi-select";
import {
  flattenVisibleTaskIds,
  sortIdsByVisibleOrder,
  type GroupedSidebarList,
} from "@/lib/sidebar/apply-view";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";

type BulkArchiveState = { ids: string[]; executorTypes: Array<string | null | undefined> };

/**
 * Sidebar multi-select wiring: selection state, the visible-order list shift
 * range-select walks, mixed-workflow detection (gates bulk "Move to step"), and
 * the bulk-archive confirm dialog. Returns the props bundle for `TaskSwitcher`
 * plus the dialog state the panel renders via `SidebarBulkArchiveDialog`.
 */
export function useSidebarSelection({
  workspaceId,
  grouped,
  collapsedGroups,
  collapsedSubtaskParents,
  displayTasks,
}: {
  workspaceId: string | null;
  grouped: GroupedSidebarList;
  collapsedGroups: string[];
  collapsedSubtaskParents: string[];
  displayTasks: Array<{ id: string; workflowId?: string; remoteExecutorType?: string | null }>;
}) {
  const multiSelect = useSidebarMultiSelect(workspaceId);
  const {
    selectedIds,
    isSelecting,
    clearSelection,
    selectRange,
    toggleSelect,
    pruneToVisible,
    bulkArchive,
    bulkMove,
  } = multiSelect;
  const [bulkArchiveState, setBulkArchiveState] = useState<BulkArchiveState | null>(null);

  const visibleTaskIds = useMemo(
    () => flattenVisibleTaskIds(grouped, collapsedGroups, collapsedSubtaskParents),
    [grouped, collapsedGroups, collapsedSubtaskParents],
  );

  const isMixedWorkflowSelection = useMemo(() => {
    const wfIds = new Set<string>();
    for (const t of displayTasks) {
      if (selectedIds.has(t.id) && t.workflowId) wfIds.add(t.workflowId);
    }
    return wfIds.size > 1;
  }, [displayTasks, selectedIds]);

  const handleBulkArchive = useCallback(
    (ids: string[]) => {
      const byId = new Map(displayTasks.map((t) => [t.id, t.remoteExecutorType]));
      setBulkArchiveState({ ids, executorTypes: ids.map((id) => byId.get(id) ?? null) });
    },
    [displayTasks],
  );

  const handleBulkArchiveConfirm = useCallback(
    async ({ cascade }: { cascade: boolean }) => {
      if (!bulkArchiveState) return;
      try {
        await bulkArchive(bulkArchiveState.ids, { cascade });
      } catch (error) {
        console.error("Failed to archive tasks:", error);
      } finally {
        setBulkArchiveState(null);
      }
    },
    [bulkArchiveState, bulkArchive],
  );

  // Escape clears an active selection.
  useEffect(() => {
    if (!isSelecting) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") clearSelection();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [isSelecting, clearSelection]);

  // Prune selections that scroll out of view (collapsed group / filter change)
  // so plain clicks on visible rows stop behaving as selection-mode.
  useEffect(() => {
    pruneToVisible(visibleTaskIds);
  }, [visibleTaskIds, pruneToVisible]);

  // Stable ref so TaskSwitcher's React.memo isn't defeated by a fresh closure
  // on every unrelated sidebar re-render.
  const onSelectTaskRange = useCallback(
    (taskId: string) => selectRange(taskId, visibleTaskIds),
    [selectRange, visibleTaskIds],
  );

  // Bulk move in the rendered top-to-bottom order so a backward range selection
  // (anchor after target) doesn't land scrambled at the destination.
  const onBulkMove = useCallback(
    (ids: string[], targetWorkflowId: string, targetStepId: string) =>
      bulkMove(sortIdsByVisibleOrder(ids, visibleTaskIds), targetWorkflowId, targetStepId),
    [bulkMove, visibleTaskIds],
  );

  const switcherProps = {
    selectedTaskIds: selectedIds,
    onToggleSelectTask: toggleSelect,
    onSelectTaskRange,
    onBulkArchive: handleBulkArchive,
    onBulkMove,
    onClearSelection: clearSelection,
    isMixedWorkflowSelection,
  };

  return {
    switcherProps,
    bulkArchiveState,
    setBulkArchiveState,
    handleBulkArchiveConfirm,
    isArchiving: multiSelect.isArchiving,
  };
}

export function SidebarBulkArchiveDialog({
  selection,
}: {
  selection: ReturnType<typeof useSidebarSelection>;
}) {
  if (!selection.bulkArchiveState) return null;
  return (
    <TaskArchiveConfirmDialog
      open
      onOpenChange={(o) => !o && selection.setBulkArchiveState(null)}
      isBulkOperation
      count={selection.bulkArchiveState.ids.length}
      taskIds={selection.bulkArchiveState.ids}
      executorTypes={selection.bulkArchiveState.executorTypes}
      isArchiving={selection.isArchiving}
      onConfirm={selection.handleBulkArchiveConfirm}
      confirmTestId="sidebar-bulk-archive-confirm"
    />
  );
}
