"use client";

import { useCallback, useEffect, useReducer, useState, type Dispatch } from "react";
import {
  INITIAL_STATE,
  multiSelectReducer,
  useTaskMultiSelectStore,
} from "./use-task-multi-select";
import { useTaskActions, type TaskActionOptions } from "./use-task-actions";
import { useTaskRemoval, useTaskRemovalSuccessNotifier } from "./use-task-removal";
import { useTaskWorkflowMove } from "./use-task-workflow-move";
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";
import { useTranslation } from "react-i18next";

type BulkOpts = TaskActionOptions;

type BulkRemovalParams = {
  ids: string[];
  action: "archive" | "delete";
  per: (id: string, opts?: BulkOpts) => Promise<void>;
  setBusy: (value: boolean) => void;
  failureKey: string;
  opts?: BulkOpts;
};

/**
 * Bulk archive/delete/move that act on an explicit id list. Archive and delete
 * share one flow: the coordinator moves away from an active task before the
 * mutations complete; keep any failed ids selected for retry and surface the
 * failure via toast.
 */
function useSidebarBulkActions(
  dispatch: Dispatch<{ type: "set_selected"; ids: Set<string> }>,
  clearSelection: () => void,
) {
  const { archiveTaskById, deleteTaskById } = useTaskActions();
  const notifySuccess = useTaskRemovalSuccessNotifier();
  const { runTaskRemovalBatch } = useTaskRemoval({
    store: useAppStoreApi(),
    useLayoutSwitch: true,
    notifySuccess,
  });
  const { getWorkflowIdForTask, removeTasksFromStore } = useTaskMultiSelectStore();
  const moveTasks = useTaskWorkflowMove();
  const { toast } = useToast();
  const { t } = useTranslation();
  const [isArchiving, setIsArchiving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const runBulkRemoval = useCallback(
    async ({ ids, action, per, setBusy, failureKey, opts }: BulkRemovalParams) => {
      if (ids.length === 0) return;
      setBusy(true);
      try {
        const result = await runTaskRemovalBatch(
          action,
          ids.map((id) => ({ taskId: id, mutate: () => per(id, opts) })),
          { cascade: opts?.cascade },
        );
        if (result.skipped) return;
        const failed = result.failedTaskIds;
        if (result.succeededTaskIds.length > 0) {
          removeTasksFromStore(new Set(result.succeededTaskIds));
        }
        if (failed.length > 0) {
          // Keep the failed ids selected so the user can retry, and surface it.
          dispatch({ type: "set_selected", ids: new Set(failed) });
          toast({
            title: t(failureKey, { count: failed.length }),
            variant: "error",
          });
          return;
        }
        clearSelection();
      } finally {
        setBusy(false);
      }
    },
    [runTaskRemovalBatch, removeTasksFromStore, dispatch, toast, clearSelection, t],
  );

  const bulkArchive = useCallback(
    (ids: string[], opts?: BulkOpts) =>
      runBulkRemoval({
        ids,
        action: "archive",
        per: (id) => archiveTaskById(id, opts),
        setBusy: setIsArchiving,
        failureKey: "sidebar:bulkArchiveFailed",
        opts,
      }),
    [runBulkRemoval, archiveTaskById],
  );

  const bulkDelete = useCallback(
    (ids: string[], opts?: BulkOpts) =>
      runBulkRemoval({
        ids,
        action: "delete",
        per: (id) => deleteTaskById(id, opts),
        setBusy: setIsDeleting,
        failureKey: "sidebar:bulkDeleteFailed",
        opts,
      }),
    [runBulkRemoval, deleteTaskById],
  );

  const bulkMove = useCallback(
    async (ids: string[], targetWorkflowId: string, targetStepId: string) => {
      if (ids.length === 0) return;
      try {
        const destination = ids.every((taskId) => getWorkflowIdForTask(taskId) === targetWorkflowId)
          ? "step"
          : "workflow";
        await moveTasks(ids, targetWorkflowId, targetStepId, destination);
        clearSelection();
      } catch {
        // useTaskWorkflowMove already toasts the failure; keep the rows selected
        // for retry and swallow the rejection so it isn't unhandled at the
        // fire-and-forget call site.
      }
    },
    [moveTasks, clearSelection, getWorkflowIdForTask],
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
