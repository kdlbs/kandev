"use client";

import { useCallback, useEffect, useReducer, useState } from "react";
import {
  INITIAL_STATE,
  multiSelectReducer,
  useTaskMultiSelectStore,
} from "./use-task-multi-select";
import { useTaskActions, useArchiveAndSwitchTask } from "./use-task-actions";
import { useTaskWorkflowMove } from "./use-task-workflow-move";
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";

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

  const store = useAppStoreApi();
  const { archiveTaskById } = useTaskActions();
  const archiveAndSwitch = useArchiveAndSwitchTask({ useLayoutSwitch: true });
  const { removeTasksFromStore } = useTaskMultiSelectStore();
  const moveTasks = useTaskWorkflowMove();
  const { toast } = useToast();

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
      // If the open task is in the set, archive it via the switch-aware path last
      // so the URL/layout moves off it instead of showing stale content.
      const activeId = store.getState().tasks.activeTaskId;
      const activeInSet = activeId != null && ids.includes(activeId);
      const restIds = activeInSet ? ids.filter((id) => id !== activeId) : ids;
      setIsArchiving(true);
      try {
        const results = await Promise.allSettled(restIds.map((id) => archiveTaskById(id, opts)));
        const failed = restIds.filter((_, i) => results[i].status === "rejected");
        const succeeded = restIds.filter((_, i) => results[i].status === "fulfilled");
        if (succeeded.length > 0) removeTasksFromStore(new Set(succeeded));
        if (activeInSet) {
          try {
            await archiveAndSwitch(activeId!, opts);
          } catch {
            failed.push(activeId!);
          }
        }
        if (failed.length > 0) {
          // Mirror the kanban hook: keep the failed ids selected so the user can
          // retry, and surface the failure instead of silently clearing.
          dispatch({ type: "set_selected", ids: new Set(failed) });
          toast({
            title: `Failed to archive ${failed.length} task${failed.length === 1 ? "" : "s"}`,
            variant: "error",
          });
          return;
        }
        clearSelection();
      } finally {
        setIsArchiving(false);
      }
    },
    [store, archiveTaskById, archiveAndSwitch, removeTasksFromStore, clearSelection, toast],
  );

  const bulkMove = useCallback(
    async (ids: string[], targetWorkflowId: string, targetStepId: string) => {
      if (ids.length === 0) return;
      try {
        await moveTasks(ids, targetWorkflowId, targetStepId);
        clearSelection();
      } catch {
        // useTaskWorkflowMove already toasts the failure; keep the rows selected
        // for retry and swallow the rejection so it isn't unhandled at the
        // fire-and-forget call site.
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
