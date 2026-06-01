"use client";

import { createContext, useCallback, useContext } from "react";
import { toast } from "sonner";
import { useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { ListTasksResponse } from "@/lib/api/domains/office-tasks-api";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";

/**
 * Context for the local (page-level) task representation. The TanStack
 * Query cache holds the canonical OfficeTask list but the detail page
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
 * task. Snapshots the local + TQ-cache state, applies the patch
 * immediately, runs the API call, and rolls back + toasts on failure. On
 * success the optimistic patch is left in place; the canonical
 * reconciliation happens via the office TQ bridge (`office.task.updated`
 * patches/invalidates the tasks caches).
 */
export function useOptimisticTaskMutation() {
  const ctx = useTaskOptimisticContext();
  const qc = useQueryClient();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);

  return useCallback(
    async (
      taskId: string,
      patch: Partial<Task>,
      apiCall: () => Promise<unknown>,
    ): Promise<void> => {
      const snapshot = ctx.task;
      const storePatch = toOfficeTaskPatch(patch);

      // Apply optimistic patches: the local detail-page state and the TQ
      // tasks caches (flat + infinite) the list views read from.
      ctx.applyPatch(patch);
      const rollbackCaches = patchTaskCaches(qc, workspaceId, taskId, storePatch);

      try {
        await apiCall();
      } catch (err) {
        // Rollback both layers.
        ctx.restore(snapshot);
        rollbackCaches();
        const message = err instanceof Error ? err.message : "Update failed";
        toast.error(message);
        throw err;
      }
    },
    [ctx, qc, workspaceId],
  );
}

type QueryClient = ReturnType<typeof useQueryClient>;

/**
 * Optimistically patches a task in both the flat (`qk.office.tasks`) and
 * paginated (`qk.office.tasksPaginated`) TQ caches for the workspace, and
 * returns a rollback that restores the pre-patch snapshots. Mirrors the
 * office bridge's `patchTask` updater so the list reflects the change
 * instantly, before the WS round-trip reconciles it.
 */
function patchTaskCaches(
  qc: QueryClient,
  workspaceId: string | null | undefined,
  taskId: string,
  patch: Partial<OfficeTask>,
): () => void {
  if (!workspaceId) return () => {};

  const flatKey = qk.office.tasks(workspaceId);
  const paginatedKey = qk.office.tasksPaginated(workspaceId);

  const flatSnapshots = qc.getQueriesData<OfficeTask[]>({ queryKey: flatKey });
  const paginatedSnapshots = qc.getQueriesData<InfiniteData<ListTasksResponse>>({
    queryKey: paginatedKey,
  });

  qc.setQueriesData<OfficeTask[]>({ queryKey: flatKey }, (prev) =>
    prev?.map((t) => (t.id === taskId ? { ...t, ...patch } : t)),
  );
  qc.setQueriesData<InfiniteData<ListTasksResponse>>({ queryKey: paginatedKey }, (prev) =>
    prev
      ? {
          ...prev,
          pages: prev.pages.map((page) => ({
            ...page,
            tasks: page.tasks.map((t) => (t.id === taskId ? { ...t, ...patch } : t)),
          })),
        }
      : prev,
  );

  return () => {
    for (const [key, data] of flatSnapshots) qc.setQueryData(key, data);
    for (const [key, data] of paginatedSnapshots) qc.setQueryData(key, data);
  };
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
