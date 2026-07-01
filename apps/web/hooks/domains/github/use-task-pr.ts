"use client";

import { useEffect, useCallback, useRef, useSyncExternalStore } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import {
  taskPrsQueryOptions,
  workspaceTaskPrsQueryOptions,
} from "@/lib/query/query-options/github";
import type { TaskPR } from "@/lib/types/github";

/** Fetch all PR associations for a workspace. */
export function useWorkspacePRs(workspaceId: string | null) {
  const queryClient = useQueryClient();
  const taskIdsRef = useRef<{ workspaceId: string | null; taskIds: Set<string> }>({
    workspaceId: null,
    taskIds: new Set<string>(),
  });
  const query = useQuery({
    ...workspaceTaskPrsQueryOptions(workspaceId ?? ""),
    enabled: Boolean(workspaceId),
  });

  useEffect(() => {
    if (!query.data || !workspaceId) return;
    const prsByTask = query.data.task_prs ?? {};
    const nextTaskIds = new Set(Object.keys(prsByTask));
    const previousTaskIds =
      taskIdsRef.current.workspaceId === workspaceId
        ? taskIdsRef.current.taskIds
        : new Set<string>();
    for (const taskId of previousTaskIds) {
      if (!nextTaskIds.has(taskId)) {
        queryClient.setQueryData(qk.integrations.github.taskPr(taskId), []);
      }
    }
    for (const [taskId, prs] of Object.entries(prsByTask)) {
      queryClient.setQueryData(qk.integrations.github.taskPr(taskId), prs);
    }
    taskIdsRef.current = { workspaceId, taskIds: nextTaskIds };
  }, [query.data, queryClient, workspaceId]);

  return query.data?.task_prs ?? {};
}

const SYNC_RETRY_DELAY = 5_000; // 5 seconds
const SYNC_MAX_RETRIES = 6; // Up to 30 seconds of retries
const EMPTY_TASK_PRS: TaskPR[] = [];

/**
 * Returns the primary PR (first by created_at) for a task. Multi-repo tasks
 * may have additional PRs — use `useTaskPRs` to get the full list.
 */
export function getPrimaryTaskPR(prs: TaskPR[] | undefined): TaskPR | null {
  return prs && prs.length > 0 ? prs[0] : null;
}

/**
 * Normalises the WS sync response into an array of TaskPR rows. Backend
 * returns `{prs: TaskPR[]}` (current shape) for multi-repo support, but we
 * accept the legacy bare-TaskPR shape too in case an older backend is
 * still running. Empty / null / unknown shapes return an empty array.
 */
function normalizeSyncResponse(result: SyncResponse): TaskPR[] {
  if (!result) return [];
  const envelope = result as { prs?: TaskPR[] };
  if (Array.isArray(envelope.prs)) return envelope.prs;
  const single = result as TaskPR;
  if (single.task_id) return [single];
  return [];
}

/**
 * Response shape from the `github.task_pr.sync` WS action. `permanent`
 * is true when every watch on the task points at a repository the
 * backend has classified as unresolvable (missing, deleted, or
 * inaccessible). When set, the 5s retry interval stops — without this
 * the frontend kept hammering a dead repo every 5s for the lifetime of
 * the task. Older backends omit the field, so it's optional.
 */
type SyncResponse = { prs?: TaskPR[]; permanent?: boolean } | TaskPR | null | undefined;

/** Fetch a single task's PR associations, with on-demand sync via WS. */
export function useTaskPR(taskId: string | null) {
  const queryClient = useQueryClient();
  const taskPrsQuery = useQuery(taskPrsQueryOptions(taskId ?? ""));
  const prs = Array.isArray(taskPrsQuery.data) ? taskPrsQuery.data : [];
  const pr = getPrimaryTaskPR(prs ?? undefined);
  const clearPendingPrUrlForTaskPR = useAppStore((state) => state.clearPendingPrUrlForTaskPR);
  const retryRef = useRef(0);
  const permanentRef = useRef(false);
  // Monotonic counter incremented before each WS request, snapshotted in
  // the .then() closure. Mirrors useWorkspacePRs above. Without this, a
  // stale response from a previous taskId can land after the user
  // navigates to a new task and flip permanentRef.current = true for the
  // active task, killing its retry loop. The reset effect below clears
  // retry/permanent state on taskId change, but a still-in-flight WS
  // call from the previous task can race that reset.
  const requestRef = useRef(0);

  const refresh = useCallback(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;

    // Backend returns `{prs: TaskPR[], permanent?: boolean}` — multi-repo
    // tasks have one row per repo. We push each into the store so the
    // per-repo PR icon stays in sync. Empty array means no watches yet
    // (the freshness retry below handles the polling cadence). Legacy
    // single-PR shape (`TaskPR` only) is detected via the absence of
    // `.prs`. When `permanent` is true (every watch's repo is dead),
    // exhaust the retry counter so the 5s interval below clears itself.
    const requestId = ++requestRef.current;
    const requestedTaskId = taskId;
    client
      .request<SyncResponse>("github.task_pr.sync", { task_id: requestedTaskId })
      .then((result) => {
        // Drop responses that aren't the latest in-flight request for
        // this hook instance — they'd otherwise corrupt permanentRef /
        // retryRef for whatever task the user is now viewing. The
        // taskId-change effect below bumps requestRef.current too, so
        // requestId alone covers both stale-by-sequence and
        // stale-by-task-change.
        if (requestRef.current !== requestId) return;
        const envelope = (result ?? {}) as { permanent?: boolean };
        if (envelope.permanent) {
          permanentRef.current = true;
          retryRef.current = SYNC_MAX_RETRIES;
        }
        const list = normalizeSyncResponse(result);
        if (list.length === 0) {
          queryClient.setQueryData(qk.integrations.github.taskPr(requestedTaskId), EMPTY_TASK_PRS);
          return;
        }
        queryClient.setQueryData(qk.integrations.github.taskPr(requestedTaskId), list);
        retryRef.current = 0;
      })
      .catch(() => {
        // Ignore - sync may fail if no watch exists
      });
  }, [queryClient, taskId]);

  // Reset retry/permanent state when taskId changes. Bumping requestRef
  // here invalidates any still-in-flight .then() closure from the prior
  // taskId so it can't write to the new task's refs.
  useEffect(() => {
    retryRef.current = 0;
    permanentRef.current = false;
    requestRef.current++;
  }, [taskId]);

  useEffect(() => {
    if (!taskId || prs.length === 0) return;
    for (const taskPR of prs) {
      clearPendingPrUrlForTaskPR(taskId, taskPR);
    }
  }, [clearPendingPrUrlForTaskPR, prs, taskId]);

  // Sync once when the task becomes active (freshness check).
  // Intentionally excludes `pr` so WS-driven store updates don't re-trigger.
  useEffect(() => {
    if (!taskId) return;
    refresh();
  }, [taskId, refresh]);

  // Retry polling when no PR is in the store yet. permanentRef short-circuits
  // the interval entirely so a task whose repos are all dead doesn't tie up
  // the backend's gh throttle on every 5s tick.
  useEffect(() => {
    if (!taskId || pr || permanentRef.current) return;

    const interval = setInterval(() => {
      if (retryRef.current >= SYNC_MAX_RETRIES || permanentRef.current) {
        clearInterval(interval);
        return;
      }
      retryRef.current++;
      refresh();
    }, SYNC_RETRY_DELAY);

    return () => clearInterval(interval);
  }, [taskId, pr, refresh]);

  return { pr, prs: prs ?? [], refresh } as {
    pr: TaskPR | null;
    prs: TaskPR[];
    refresh: () => void;
  };
}

function useCachedTaskPRs(taskId: string | null): TaskPR[] {
  const queryClient = useQueryClient();
  const subscribe = useCallback(
    (onStoreChange: () => void) => queryClient.getQueryCache().subscribe(onStoreChange),
    [queryClient],
  );
  const getSnapshot = useCallback(() => {
    if (!taskId) return EMPTY_TASK_PRS;
    const prs = queryClient.getQueryData(qk.integrations.github.taskPr(taskId));
    return Array.isArray(prs) ? prs : EMPTY_TASK_PRS;
  }, [queryClient, taskId]);

  return useSyncExternalStore(subscribe, getSnapshot, () => EMPTY_TASK_PRS);
}

/** Read the active task's primary PR from the cache (no fetching or sync). */
export function useActiveTaskPR(): TaskPR | null {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const prs = useCachedTaskPRs(activeTaskId);
  if (!activeTaskId) return null;
  return getPrimaryTaskPR(prs);
}
