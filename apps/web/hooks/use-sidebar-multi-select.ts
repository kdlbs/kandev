"use client";

import { useCallback, useEffect, useReducer, useState } from "react";
import {
  INITIAL_STATE,
  multiSelectReducer,
  useTaskMultiSelectStore,
} from "./use-task-multi-select";
import { useTaskActions } from "./use-task-actions";
import { useTaskWorkflowMove } from "./use-task-workflow-move";

/**
 * Sidebar task multi-select. Reuses the shared selection reducer (toggle +
 * shift-range with an anchor) from `use-task-multi-select`, but exposes only the
 * pieces the sidebar needs: selection state plus bulk archive/move that act on
 * an explicit id list (driven by the right-click context menu, not a toolbar).
 *
 * Unlike the kanban hook this spans all workflows in a workspace, so it resets
 * on workspace change and the caller passes the target workflow for moves.
 */
export function useSidebarMultiSelect(workspaceId: string | null) {
  const [state, dispatch] = useReducer(multiSelectReducer, INITIAL_STATE);
  const { selectedIds } = state;
  const [isArchiving, setIsArchiving] = useState(false);

  // Selection is ephemeral per workspace; drop it when the workspace changes.
  useEffect(() => {
    dispatch({ type: "reset" });
  }, [workspaceId]);

  const { archiveTaskById } = useTaskActions();
  const { removeTasksFromStore } = useTaskMultiSelectStore();
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

  const clearSelection = useCallback(() => dispatch({ type: "set_selected", ids: new Set() }), []);

  const bulkArchive = useCallback(
    async (ids: string[], opts?: { cascade?: boolean }) => {
      if (ids.length === 0) return;
      setIsArchiving(true);
      try {
        const results = await Promise.allSettled(ids.map((id) => archiveTaskById(id, opts)));
        const succeeded = new Set(ids.filter((_, i) => results[i].status === "fulfilled"));
        removeTasksFromStore(succeeded);
      } finally {
        setIsArchiving(false);
        clearSelection();
      }
    },
    [archiveTaskById, removeTasksFromStore, clearSelection],
  );

  const bulkMove = useCallback(
    async (ids: string[], targetWorkflowId: string, targetStepId: string) => {
      if (ids.length === 0) return;
      try {
        await moveTasks(ids, targetWorkflowId, targetStepId);
      } finally {
        clearSelection();
      }
    },
    [moveTasks, clearSelection],
  );

  return {
    selectedIds,
    isSelecting: selectedIds.size > 0,
    isArchiving,
    toggleSelect,
    selectRange,
    clearSelection,
    bulkArchive,
    bulkMove,
  };
}
