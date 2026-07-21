"use client";

import { useCallback } from "react";
import { toast } from "sonner";
import { updateTask } from "@/lib/api";
import { useAppStoreApi } from "@/components/state-provider";

/**
 * useNestTask returns a function that nests a task under a parent (or un-nests
 * it when `parentId` is null). It optimistically patches the workflow snapshot
 * so the sidebar re-renders immediately, then persists via the task API and
 * rolls back on failure. The canonical value is reconciled by the
 * `task.updated` WS handler.
 */
export function useNestTask() {
  const store = useAppStoreApi();

  return useCallback(
    async (taskId: string, workflowId: string, parentId: string | null) => {
      const snapshot = store.getState().kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const original = snapshot.tasks.find((t) => t.id === taskId);
      if (!original) return;

      const nextParent = parentId ?? undefined;
      if ((original.parentTaskId ?? undefined) === nextParent) return; // no-op

      // Optimistic update.
      store.getState().setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t) =>
          t.id === taskId ? { ...t, parentTaskId: nextParent } : t,
        ),
      });

      try {
        // Empty string clears the parent on the backend.
        await updateTask(taskId, { parent_id: parentId ?? "" });
      } catch (error) {
        // Roll back only if the task still holds the optimistic value.
        const cur = store.getState().kanbanMulti.snapshots[workflowId];
        const curTask = cur?.tasks.find((t) => t.id === taskId);
        if (cur && (curTask?.parentTaskId ?? undefined) === nextParent) {
          store.getState().setWorkflowSnapshot(workflowId, {
            ...cur,
            tasks: cur.tasks.map((t) =>
              t.id === taskId ? { ...t, parentTaskId: original.parentTaskId } : t,
            ),
          });
        }
        toast.error(error instanceof Error ? error.message : "Failed to nest task");
      }
    },
    [store],
  );
}
