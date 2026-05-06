"use client";

import { useEffect, useState } from "react";
import { getTaskContext, type TaskContextDTO } from "@/lib/api/domains/office-task-context-api";

/**
 * Fetches the office task-handoffs context envelope for `taskId` and
 * keeps it in component state. Returns null while loading or when the
 * backend has no HandoffService configured (the API returns 503/404),
 * letting the caller render the pre-handoffs UI in those cases.
 *
 * Re-fetches on `taskId` change and whenever `revisionKey` changes.
 * Callers pass any value that bumps when the task changes (e.g.
 * `task.updatedAt` from the store, which WS task.updated / task.archived /
 * task.unarchived handlers refresh). This gives reactive panel refresh
 * without subscribing to raw WS messages here and without polling.
 */
export function useTaskContext(
  taskId: string | null,
  revisionKey: string | number = 0,
): TaskContextDTO | null {
  const [data, setData] = useState<TaskContextDTO | null>(null);
  useEffect(() => {
    let cancelled = false;
    if (!taskId) {
      // Defer to a microtask so the setter doesn't run within the
      // effect body (eslint react-hooks/set-state-in-effect).
      Promise.resolve().then(() => {
        if (!cancelled) setData(null);
      });
      return () => {
        cancelled = true;
      };
    }
    getTaskContext(taskId)
      .then((ctx) => {
        if (!cancelled) setData(ctx);
      })
      .catch(() => {
        if (!cancelled) setData(null);
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, revisionKey]);
  return data;
}
