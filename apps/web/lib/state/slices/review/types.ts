import type { TaskReviewFinding, TaskReviewRun } from "@/lib/types/review";

export type ReviewSliceState = {
  taskReview: {
    /** Bounded run history per task, newest first. */
    runsByTaskId: Record<string, TaskReviewRun[]>;
    /** Every finding for the task, regardless of status. */
    findingsByTaskId: Record<string, TaskReviewFinding[]>;
    /** Tasks whose review has been backfilled, so mount does not refetch. */
    loadedTaskIds: Record<string, boolean>;
  };
};

export type ReviewSliceActions = {
  /** Replaces a task's review state from a backfill snapshot. */
  setTaskReview: (
    taskId: string,
    snapshot: { runs: TaskReviewRun[]; findings: TaskReviewFinding[] },
  ) => void;
  /** Inserts or replaces a run by id, keeping newest-first order. */
  upsertReviewRun: (taskId: string, run: TaskReviewRun) => void;
  /**
   * Adds findings, replacing any with the same id so a re-publish never
   * duplicates. `supersededIds` are findings the backend deleted in favour of
   * these; they are removed so a re-review does not show both at one anchor.
   */
  addReviewFindings: (
    taskId: string,
    findings: TaskReviewFinding[],
    supersededIds?: string[],
  ) => void;
  /** Replaces a single finding, typically after a status change. */
  updateReviewFinding: (taskId: string, finding: TaskReviewFinding) => void;
  /** Drops all review state for a task. */
  clearTaskReviewState: (taskId: string) => void;
};

export type ReviewSlice = ReviewSliceState & ReviewSliceActions;
