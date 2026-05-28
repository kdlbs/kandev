"use client";

import { useEffect, useCallback, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import type { TaskPR } from "@/lib/types/github";

const SYNC_RETRY_DELAY = 5_000; // 5 seconds
const SYNC_MAX_RETRIES = 6; // Up to 30 seconds of retries

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

/**
 * Fetch all PR associations for a workspace and prime both the TQ cache and
 * the Zustand `taskPRs.byTaskId` mirror — `usePRKeyToTasks` (the /github
 * page's PR-row task indicator) still reads from Zustand, and the
 * github bridge only writes to TQ for streaming WS events, not the initial
 * fetch.
 */
export function useWorkspacePRs(workspaceId: string | null) {
  const { data } = useQuery(githubQueryOptions.workspacePRs(workspaceId ?? ""));
  const setTaskPR = useAppStore((s) => s.setTaskPR);
  useEffect(() => {
    if (!data?.task_prs) return;
    for (const [taskId, prs] of Object.entries(data.task_prs)) {
      if (!Array.isArray(prs)) continue;
      for (const pr of prs) {
        if (pr.task_id) setTaskPR(taskId, pr);
      }
    }
  }, [data, setTaskPR]);
}

/**
 * Fetch a single task's PR associations, with on-demand sync via WS.
 *
 * Reads from the Zustand slice (updated by both the old WS handler and the
 * new bridge) so cross-domain components stay consistent during the
 * transitional Wave 2 period.
 */
export function useTaskPR(taskId: string | null) {
  // Read from the Zustand slice — the WS handler and the TQ bridge both write
  // here, so we always have the freshest value from either path.
  const prs = useAppStore((state) => (taskId ? (state.taskPRs.byTaskId[taskId] ?? null) : null));
  const pr = getPrimaryTaskPR(prs ?? undefined);
  const setTaskPR = useAppStore((state) => state.setTaskPR);
  const retryRef = useRef(0);

  const refresh = useCallback(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;

    client
      .request<{ prs?: TaskPR[] } | TaskPR | null>("github.task_pr.sync", {
        task_id: taskId,
      })
      .then((result) => {
        const list = normalizeSyncResponse(result);
        if (list.length === 0) return;
        for (const taskPR of list) {
          if (taskPR.task_id) setTaskPR(taskId, taskPR);
        }
        retryRef.current = 0;
      })
      .catch(() => {
        // Ignore — sync may fail if no watch exists
      });
  }, [taskId, setTaskPR]);

  // Reset retry count when taskId changes.
  useEffect(() => {
    retryRef.current = 0;
  }, [taskId]);

  // Sync once when the task becomes active.
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

  return {
    pr,
    prs: prs ?? [],
    refresh,
  } as { pr: TaskPR | null; prs: TaskPR[]; refresh: () => void };
}

/** Read the active task's primary PR from the store (no fetching). */
export function useActiveTaskPR(): TaskPR | null {
  return useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return null;
    return getPrimaryTaskPR(s.taskPRs.byTaskId[taskId]);
  });
}
