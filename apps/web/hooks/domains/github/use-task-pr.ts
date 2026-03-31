"use client";

import { useEffect, useCallback, useRef } from "react";
import { listWorkspaceTaskPRs } from "@/lib/api/domains/github-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

/** Fetch all PR associations for a workspace. */
export function useWorkspacePRs(workspaceId: string | null) {
  const setTaskPRs = useAppStore((state) => state.setTaskPRs);
  const setTaskPRsLoading = useAppStore((state) => state.setTaskPRsLoading);
  const fetchedRef = useRef("");

  useEffect(() => {
    if (!workspaceId || fetchedRef.current === workspaceId) return;
    fetchedRef.current = workspaceId;

    setTaskPRsLoading(true);
    listWorkspaceTaskPRs(workspaceId, { cache: "no-store" })
      .then((response) => {
        setTaskPRs(response?.task_prs ?? {});
      })
      .catch(() => {})
      .finally(() => {
        setTaskPRsLoading(false);
      });
  }, [workspaceId, setTaskPRs, setTaskPRsLoading]);
}

const SYNC_RETRY_DELAY = 5_000; // 5 seconds
const SYNC_MAX_RETRIES = 6; // Up to 30 seconds of retries

/** Fetch a single task's PR association, with on-demand sync via WS. */
export function useTaskPR(taskId: string | null) {
  const pr = useAppStore((state) => (taskId ? (state.taskPRs.byTaskId[taskId] ?? null) : null));
  const setTaskPR = useAppStore((state) => state.setTaskPR);
  const retryRef = useRef(0);

  const refresh = useCallback(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;

    client
      .request<TaskPR | null>("github.task_pr.sync", { task_id: taskId })
      .then((result) => {
        if (result?.task_id) {
          setTaskPR(taskId, result);
          retryRef.current = 0;
        }
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

  return { pr, refresh } as { pr: TaskPR | null; refresh: () => void };
}

/** Read the active task's PR from the store (no fetching). */
export function useActiveTaskPR(): TaskPR | null {
  return useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    return taskId ? (s.taskPRs.byTaskId[taskId] ?? null) : null;
  });
}
