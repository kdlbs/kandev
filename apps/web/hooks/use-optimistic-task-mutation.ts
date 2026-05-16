"use client";

import { createContext, useCallback, useContext } from "react";
import { toast } from "sonner";
import { useAppStoreApi } from "@/components/state-provider";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";

/**
 * Context for the local (page-level) task representation. The office store
 * holds the canonical OfficeTask but the detail page maintains a richer
 * Task object with extra fields (reviewers, approvers, blockedBy, etc.).
 *
 * Pickers live inside <TaskOptimisticProvider> on the detail page; the
 * provider exposes a way to patch / restore the local task state so
 * optimistic updates flow into the visible UI without prop drilling.
 */
export type TaskOptimisticContextValue = {
  task: Task;
  applyPatch: (patch: Partial<Task>) => void;
  restore: (snapshot: Task) => void;
};

const TaskOptimisticContext = createContext<TaskOptimisticContextValue | null>(null);

export const TaskOptimisticContextProvider = TaskOptimisticContext.Provider;

export function useTaskOptimisticContext(): TaskOptimisticContextValue {
  const ctx = useContext(TaskOptimisticContext);
  if (!ctx) {
    throw new Error("useTaskOptimisticContext must be used within <TaskOptimisticContextProvider>");
  }
  return ctx;
}

/**
 * Returns a function that performs an optimistic mutation on the current
 * task. Snapshots the local + store state, applies the patch immediately,
 * runs the API call, and rolls back + toasts on failure. On success the
 * optimistic patch is left in place; the canonical reconciliation happens
 * via the `office.task.updated` WS handler (re-fetches the task DTO).
 */
export function useOptimisticTaskMutation() {
  const ctx = useTaskOptimisticContext();
  const storeApi = useAppStoreApi();

  return useCallback(
    async (
      taskId: string,
      patch: Partial<Task>,
      apiCall: () => Promise<unknown>,
    ): Promise<void> => {
      const snapshot = ctx.task;
      const storePatch = toOfficeTaskPatch(patch);
      const storeSnapshot = storeApi.getState().office.tasks.items.find((t) => t.id === taskId);

      // Apply optimistic patches (local + store).
      ctx.applyPatch(patch);
      if (storeSnapshot) {
        storeApi.getState().patchTaskInStore(taskId, storePatch);
      }

      try {
        await apiCall();
      } catch (err) {
        // Rollback both layers.
        ctx.restore(snapshot);
        if (storeSnapshot) {
          storeApi.getState().patchTaskInStore(taskId, storeSnapshot);
        }
        const message = err instanceof Error ? err.message : "Update failed";
        toast.error(message);
        throw err;
      }
    },
    [ctx, storeApi],
  );
}

/**
 * Maps a Task patch to the subset of fields that exist on OfficeTask, so we
 * can keep both the local and store representations in sync.
 */
function toOfficeTaskPatch(patch: Partial<Task>): Partial<OfficeTask> {
  const out: Partial<OfficeTask> = {};
  if (patch.status !== undefined) out.status = patch.status;
  if (patch.priority !== undefined) out.priority = patch.priority;
  if (patch.assigneeAgentProfileId !== undefined) {
    out.assigneeAgentProfileId = patch.assigneeAgentProfileId;
  }
  if (patch.projectId !== undefined) out.projectId = patch.projectId;
  if (patch.parentId !== undefined) out.parentId = patch.parentId;
  if (patch.labels !== undefined) out.labels = patch.labels;
  if (patch.blockedBy !== undefined) out.blockedBy = patch.blockedBy;
  return out;
}
