"use client";

import { useEffect, useCallback, useRef } from "react";
import { listTaskPRs, getTaskPR } from "@/lib/api/domains/github-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";

/** Fetch and cache PR associations for a batch of task IDs. */
export function useTaskPRs(taskIds: string[]) {
  const byTaskId = useAppStore((state) => state.taskPRs.byTaskId);
  const loading = useAppStore((state) => state.taskPRs.loading);
  const setTaskPRs = useAppStore((state) => state.setTaskPRs);
  const setTaskPRsLoading = useAppStore((state) => state.setTaskPRsLoading);

  useEffect(() => {
    if (taskIds.length === 0 || loading) return;
    setTaskPRsLoading(true);
    listTaskPRs(taskIds, { cache: "no-store" })
      .then((response) => {
        setTaskPRs(response?.task_prs ?? {});
      })
      .catch(() => {
        // Keep existing data on error
      })
      .finally(() => {
        setTaskPRsLoading(false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskIds.join(",")]);

  return { byTaskId, loading };
}

/** Fetch a single task's PR association. */
export function useTaskPR(taskId: string | null) {
  const pr = useAppStore((state) => (taskId ? (state.taskPRs.byTaskId[taskId] ?? null) : null));
  const setTaskPR = useAppStore((state) => state.setTaskPR);

  const refresh = useCallback(() => {
    if (!taskId) return;
    getTaskPR(taskId, { cache: "no-store" })
      .then((response) => {
        if (response) setTaskPR(taskId, response);
      })
      .catch(() => {
        // Ignore - PR may not exist for this task
      });
  }, [taskId, setTaskPR]);

  useEffect(() => {
    if (!taskId || pr) return;
    refresh();
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

const PR_DETECTION_INTERVAL = 30_000; // 30 seconds

/**
 * Periodically checks for a PR on the session's branch when no PR is
 * associated with the task yet. Triggers a backend check that ensures
 * a PR watch exists and tries to find the PR immediately.
 */
export function useTaskPRDetection(
  taskId: string | null,
  sessionId: string | null,
  branch: string | null,
) {
  const pr = useAppStore((s) => (taskId ? (s.taskPRs.byTaskId[taskId] ?? null) : null));
  const setTaskPR = useAppStore((s) => s.setTaskPR);
  const checkingRef = useRef(false);

  useEffect(() => {
    if (!taskId || !sessionId || !branch || pr) return;

    const check = () => {
      if (checkingRef.current) return;
      checkingRef.current = true;

      const client = getWebSocketClient();
      if (!client) {
        checkingRef.current = false;
        return;
      }

      client
        .request<{ found?: boolean }>("github.check_session_pr", {
          task_id: taskId,
          session_id: sessionId,
        })
        .then((resp) => {
          if (resp?.found) {
            // PR was found and associated — fetch the full TaskPR record
            getTaskPR(taskId, { cache: "no-store" })
              .then((taskPr) => {
                if (taskPr) setTaskPR(taskId, taskPr);
              })
              .catch(() => {});
          }
        })
        .catch(() => {})
        .finally(() => {
          checkingRef.current = false;
        });
    };

    // Check immediately, then on interval
    check();
    const interval = setInterval(check, PR_DETECTION_INTERVAL);
    return () => clearInterval(interval);
  }, [taskId, sessionId, branch, pr, setTaskPR]);
}
