"use client";

import { useCallback, useEffect, useReducer, useState, type Dispatch } from "react";
import {
  INITIAL_STATE,
  multiSelectReducer,
  useTaskMultiSelectStore,
} from "./use-task-multi-select";
import { useTaskActions, useArchiveAndSwitchTask } from "./use-task-actions";
import { useTaskRemoval } from "./use-task-removal";
import { useTaskWorkflowMove } from "./use-task-workflow-move";
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";

type BulkOpts = { cascade?: boolean };

/**
 * Bulk archive/delete/move that act on an explicit id list. Archive and delete
 * share one flow: remove each task, routing the currently-open task through the
 * switch-aware path last so the view doesn't strand on removed content; keep any
 * failed ids selected for retry and surface the failure via toast.
 */
function useSidebarBulkActions(
  dispatch: Dispatch<{ type: "set_selected"; ids: Set<string> }>,
  clearSelection: () => void,
) {
  const store = useAppStoreApi();
  const { archiveTaskById, deleteTaskById } = useTaskActions();
  const archiveAndSwitch = useArchiveAndSwitchTask({ useLayoutSwitch: true });
  const { removeTaskFromBoard } = useTaskRemoval({ store, useLayoutSwitch: true });
  const { removeTasksFromStore } = useTaskMultiSelectStore();
  const moveTasks = useTaskWorkflowMove();
  const { toast } = useToast();
  const [isArchiving, setIsArchiving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const runBulkRemoval = useCallback(
    async (
      ids: string[],
      per: (id: string) => Promise<void>,
      handleActive: (id: string) => Promise<void>,
      setBusy: (v: boolean) => void,
      noun: string,
    ) => {
      if (ids.length === 0) return;
      const activeId = store.getState().tasks.activeTaskId;
      const activeInSet = activeId != null && ids.includes(activeId);
      const restIds = activeInSet ? ids.filter((id) => id !== activeId) : ids;
      setBusy(true);
      try {
        const results = await Promise.allSettled(restIds.map((id) => per(id)));
        const failed = restIds.filter((_, i) => results[i].status === "rejected");
        const succeeded = restIds.filter((_, i) => results[i].status === "fulfilled");
        if (succeeded.length > 0) removeTasksFromStore(new Set(succeeded));
        if (activeInSet) {
          try {
            await handleActive(activeId!);
          } catch {
            failed.push(activeId!);
          }
        }
        if (failed.length > 0) {
          // Keep the failed ids selected so the user can retry, and surface it.
          dispatch({ type: "set_selected", ids: new Set(failed) });
          toast({
            title: `Failed to ${noun} ${failed.length} task${failed.length === 1 ? "" : "s"}`,
            variant: "error",
          });
          return;
        }
        clearSelection();
      } finally {
        setBusy(false);
      }
    },
    [store, removeTasksFromStore, dispatch, toast, clearSelection],
  );

  const bulkArchive = useCallback(
    (ids: string[], opts?: BulkOpts) =>
      runBulkRemoval(
        ids,
        (id) => archiveTaskById(id, opts),
        (id) => archiveAndSwitch(id, opts),
        setIsArchiving,
        "archive",
      ),
    [runBulkRemoval, archiveTaskById, archiveAndSwitch],
  );

  const bulkDelete = useCallback(
    (ids: string[], opts?: BulkOpts) =>
      runBulkRemoval(
        ids,
        (id) => deleteTaskById(id, opts),
        async (id) => {
          const { activeTaskId, activeSessionId } = store.getState().tasks;
          await deleteTaskById(id, opts);
          await removeTaskFromBoard(id, {
            wasActiveTaskId: activeTaskId,
            wasActiveSessionId: activeSessionId,
          });
        },
        setIsDeleting,
        "delete",
      ),
    [runBulkRemoval, deleteTaskById, removeTaskFromBoard, store],
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

  return { bulkArchive, bulkDelete, bulkMove, isArchiving, isDeleting };
}

/**
 * Sidebar task multi-select. Reuses the shared selection reducer (toggle +
 * shift-range with an anchor) from `use-task-multi-select`, and exposes the
 * selection state plus bulk archive/delete/move used by the right-click menu.
 *
 * Unlike the kanban hook this spans all workflows in a workspace, so it resets
 * on workspace change and the caller passes the target workflow for moves.
 */
export function useSidebarMultiSelect(workspaceId: string | null) {
  const [state, dispatch] = useReducer(multiSelectReducer, INITIAL_STATE);
  const { selectedIds } = state;

  // Selection is ephemeral per workspace; drop it when the workspace changes.
  useEffect(() => {
    dispatch({ type: "reset" });
  }, [workspaceId]);

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

  // Drop any selected ids that are no longer visible (collapsed group / filtered
  // out) so `isSelecting` doesn't stay latched on hidden rows. No-ops (and avoids
  // a render loop) when every selected id is still visible.
  const pruneToVisible = useCallback(
    (visibleIds: string[]) => {
      const visible = new Set(visibleIds);
      const next = new Set([...selectedIds].filter((id) => visible.has(id)));
      if (next.size !== selectedIds.size) dispatch({ type: "set_selected", ids: next });
    },
    [selectedIds],
  );

  const bulk = useSidebarBulkActions(dispatch, clearSelection);

  return {
    selectedIds,
    isSelecting: selectedIds.size > 0,
    isArchiving: bulk.isArchiving,
    isDeleting: bulk.isDeleting,
    toggleSelect,
    selectRange,
    clearSelection,
    pruneToVisible,
    bulkArchive: bulk.bulkArchive,
    bulkDelete: bulk.bulkDelete,
    bulkMove: bulk.bulkMove,
  };
}
