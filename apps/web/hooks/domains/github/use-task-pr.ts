"use client";

import { useCallback, useEffect, useMemo, useRef, useSyncExternalStore } from "react";
import { deleteTaskPR, listWorkspaceTaskPRs } from "@/lib/api/domains/github-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getTaskPRsForCurrentWorkspace } from "./use-task-pr-tooltip-hydration";
import { getTaskPRSyncResource, type TaskPRSyncScope } from "./task-pr-sync-resource";
import type { TaskPR } from "@/lib/types/github";

/** Fetch all PR associations for a workspace. */
export function useWorkspacePRs(workspaceId: string | null) {
  const setTaskPRs = useAppStore((state) => state.setTaskPRs);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const fetchedRef = useRef<string | null>(null);
  const requestRef = useRef(0);

  useEffect(() => {
    if (!workspaceId) {
      fetchedRef.current = null;
      return;
    }
    if (fetchedRef.current === workspaceId) return;

    const requestId = ++requestRef.current;
    fetchedRef.current = workspaceId;

    listWorkspaceTaskPRs(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setTaskPRs(response?.task_prs ?? {}, {
          workspaceId,
          workspaceContextGeneration,
        });
      })
      .catch(() => {
        if (requestRef.current === requestId) {
          fetchedRef.current = null; // allow retry on failure
        }
      });
  }, [workspaceContextGeneration, setTaskPRs, workspaceId]);
}

/**
 * Returns the primary PR (first by created_at) for a task. Multi-repo tasks
 * may have additional PRs — use `useTaskPRs` to get the full list.
 */
export function getPrimaryTaskPR(prs: TaskPR[] | undefined): TaskPR | null {
  return prs && prs.length > 0 ? prs[0] : null;
}

/** Fetch a single task's PR associations, with on-demand sync via WS. */
export function useTaskPR(taskId: string | null) {
  const store = useAppStoreApi();
  const prs = useAppStore((state) =>
    taskId ? getTaskPRsForCurrentWorkspace(state, taskId) : null,
  );
  const pr = getPrimaryTaskPR(prs ?? undefined);
  const removeTaskPR = useAppStore((state) => state.removeTaskPR);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const resource = getTaskPRSyncResource(store);
  const scope = useMemo<TaskPRSyncScope | null>(
    () => (taskId ? { taskId, workspaceId, workspaceContextGeneration } : null),
    [taskId, workspaceContextGeneration, workspaceId],
  );
  const subscribe = useCallback(
    (listener: () => void) => (scope ? resource.subscribe(scope, listener) : () => undefined),
    [resource, scope],
  );
  const getSnapshot = useCallback(
    () => (scope ? resource.getSnapshot(scope) : false),
    [resource, scope],
  );
  const resourceLoaded = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const refresh = useCallback(
    () => (scope ? resource.refresh(scope) : Promise.resolve()),
    [resource, scope],
  );

  const unlink = useCallback(
    async (associationId: string) => {
      if (!taskId || !workspaceId) throw new Error("No active workspace is selected.");
      await deleteTaskPR(associationId, workspaceId);
      removeTaskPR(taskId, associationId, { workspaceId, workspaceContextGeneration });
      if (scope) resource.invalidate(scope);
    },
    [removeTaskPR, resource, scope, taskId, workspaceContextGeneration, workspaceId],
  );

  return {
    pr,
    prs: prs ?? [],
    refresh,
    unlink,
    loaded: resourceLoaded || prs !== null,
  } as {
    pr: TaskPR | null;
    prs: TaskPR[];
    refresh: () => Promise<void>;
    unlink: (associationId: string) => Promise<void>;
    loaded: boolean;
  };
}

/** Read the active task's primary PR from the store (no fetching). */
export function useActiveTaskPR(): TaskPR | null {
  return useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return null;
    return getPrimaryTaskPR(getTaskPRsForCurrentWorkspace(s, taskId) ?? undefined);
  });
}
