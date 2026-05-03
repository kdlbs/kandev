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
function normalizeSyncResponse(result: { prs?: TaskPR[] } | TaskPR | null | undefined): TaskPR[] {
  if (!result) return [];
  const envelope = result as { prs?: TaskPR[] };
  if (Array.isArray(envelope.prs)) return envelope.prs;
  const single = result as TaskPR;
  if (single.task_id) return [single];
  return [];
}

/** Fetch a single task's PR associations, with on-demand sync via WS. */
export function useTaskPR(taskId: string | null) {
  const prs = useAppStore((state) => (taskId ? (state.taskPRs.byTaskId[taskId] ?? null) : null));
  const pr = getPrimaryTaskPR(prs ?? undefined);
  const setTaskPR = useAppStore((state) => state.setTaskPR);
  const retryRef = useRef(0);

  const refresh = useCallback(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;

    // Backend returns `{prs: TaskPR[]}` — multi-repo tasks have one row per
    // repo. We push each into the store so the per-repo PR icon stays in
    // sync. Empty array means no watches yet (the freshness retry below
    // handles the polling cadence). Legacy single-PR shape (`TaskPR` only)
    // is detected via the absence of `.prs`.
    client
      .request<{ prs?: TaskPR[] } | TaskPR | null>("github.task_pr.sync", { task_id: taskId })
      .then((result) => {
        const list = normalizeSyncResponse(result);
        if (list.length === 0) return;
        for (const pr of list) {
          if (pr.task_id) setTaskPR(taskId, pr);
        }
        retryRef.current = 0;
      })
      .catch(() => {
        // Ignore - sync may fail if no watch exists
      });
  }, [taskId, setTaskPR]);

  // Reset retry count when taskId changes
  useEffect(() => {
    retryRef.current = 0;
  }, [taskId]);

  // Sync once when the task becomes active (freshness check).
  // Intentionally excludes `pr` so WS-driven store updates don't re-trigger.
  useEffect(() => {
    if (!taskId) return;
    refresh();
  }, [taskId, refresh]);

  // Retry polling when no PR is in the store yet.
  useEffect(() => {
    if (!taskId || pr) return;

    const interval = setInterval(() => {
      if (retryRef.current >= SYNC_MAX_RETRIES) {
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
