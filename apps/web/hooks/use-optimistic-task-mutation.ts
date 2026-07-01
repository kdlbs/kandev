"use client";

import { createContext, useCallback, useContext } from "react";
import { toast } from "sonner";
import type { Task } from "@/app/office/tasks/[id]/types";

/**
 * Context for the local (page-level) task representation. The detail page
 * maintains a richer Task object with extra fields (reviewers, approvers,
 * blockedBy, etc.).
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
 * task. Snapshots local state, applies the patch immediately, runs the API
 * call, and rolls back + toasts on failure. On success the optimistic patch
 * is left in place; canonical reconciliation happens via Query invalidation.
 */
export function useOptimisticTaskMutation() {
  const ctx = useTaskOptimisticContext();

  return useCallback(
    async (
      taskId: string,
      patch: Partial<Task>,
      apiCall: () => Promise<unknown>,
    ): Promise<void> => {
      const snapshot = ctx.task;

      // Keep taskId in the signature for callers that already bind the task being mutated.
      void taskId;
      ctx.applyPatch(patch);

      try {
        await apiCall();
      } catch (err) {
        ctx.restore(snapshot);
        const message = err instanceof Error ? err.message : "Update failed";
        toast.error(message);
        throw err;
      }
    },
    [ctx],
  );
}
