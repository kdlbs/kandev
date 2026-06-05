"use client";

import { useEffect, useCallback, useRef } from "react";
import { listWorkspaceTaskPRs } from "@/lib/api/domains/github-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

/** Fetch all PR associations for a workspace. */
export function useWorkspacePRs(workspaceId: string | null) {
  const setTaskPRs = useAppStore((state) => state.setTaskPRs);
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
        setTaskPRs(response?.task_prs ?? {});
      })
      .catch(() => {
        if (requestRef.current === requestId) {
          fetchedRef.current = null; // allow retry on failure
        }
      });
  }, [workspaceId, setTaskPRs]);
}

const SYNC_RETRY_DELAY = 5_000; // 5 seconds
const SYNC_MAX_RETRIES = 6; // Up to 30 seconds of retries

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
  const prs = useAppStore((state) => (taskId ? (state.taskPRs.byTaskId[taskId] ?? null) : null));
  const pr = getPrimaryTaskPR(prs ?? undefined);
  const setTaskPR = useAppStore((state) => state.setTaskPR);
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
        if (list.length === 0) return;
        for (const pr of list) {
          if (pr.task_id) setTaskPR(requestedTaskId, pr);
        }
        retryRef.current = 0;
      })
      .catch(() => {
        // Ignore - sync may fail if no watch exists
      });
  }, [taskId, setTaskPR]);

  // Reset retry/permanent state when taskId changes. Bumping requestRef
  // here invalidates any still-in-flight .then() closure from the prior
  // taskId so it can't write to the new task's refs.
  useEffect(() => {
    retryRef.current = 0;
    permanentRef.current = false;
    requestRef.current++;
  }, [taskId]);

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

/** Read the active task's primary PR from the store (no fetching). */
export function useActiveTaskPR(): TaskPR | null {
  return useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return null;
    return getPrimaryTaskPR(s.taskPRs.byTaskId[taskId]);
  });
}
