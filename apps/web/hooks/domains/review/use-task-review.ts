"use client";

import { useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getTaskReview } from "@/lib/api/domains/review-api";
import { latestRun, openFindingCount } from "@/lib/review/findings";
import type { TaskReviewFinding, TaskReviewRun } from "@/lib/types/review";

const EMPTY_RUNS: TaskReviewRun[] = [];
const EMPTY_FINDINGS: TaskReviewFinding[] = [];

/**
 * Reads a task's review state, backfilling it once on mount.
 *
 * Live `task.review.*` events can fire before the page's WS subscription is
 * established, so the store is seeded from a one-shot read the same way the
 * walkthrough store is.
 */
export function useTaskReview(taskId: string | null | undefined) {
  const storeApi = useAppStoreApi();
  const runs = useAppStore((state) =>
    taskId ? (state.taskReview.runsByTaskId[taskId] ?? EMPTY_RUNS) : EMPTY_RUNS,
  );
  const findings = useAppStore((state) =>
    taskId ? (state.taskReview.findingsByTaskId[taskId] ?? EMPTY_FINDINGS) : EMPTY_FINDINGS,
  );
  const loaded = useAppStore((state) =>
    taskId ? Boolean(state.taskReview.loadedTaskIds[taskId]) : false,
  );

  useEffect(() => {
    if (!taskId || loaded) return;
    let cancelled = false;
    getTaskReview(taskId)
      .then((snapshot) => {
        if (cancelled) return;
        storeApi.getState().setTaskReview(taskId, snapshot);
      })
      .catch(() => {
        // A failed backfill is not worth a toast: live events still populate the
        // panel, and the run control surfaces any real failure itself.
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, loaded, storeApi]);

  return useMemo(
    () => ({
      runs,
      findings,
      activeRun: latestRun(runs),
      openCount: openFindingCount(findings),
    }),
    [runs, findings],
  );
}
