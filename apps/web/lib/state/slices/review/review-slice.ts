import type { StateCreator } from "zustand";
import type { TaskReviewFinding, TaskReviewRun } from "@/lib/types/review";
import type { ReviewSlice, ReviewSliceState } from "./types";

export const defaultReviewState: ReviewSliceState = {
  taskReview: {
    runsByTaskId: {},
    findingsByTaskId: {},
    loadedTaskIds: {},
  },
};

/** How many runs a task keeps client-side; matches the backend read cap. */
const RUN_HISTORY_LIMIT = 20;

type ImmerSet = Parameters<
  StateCreator<ReviewSlice, [["zustand/immer", never]], [], ReviewSlice>
>[0];

/** Newest run first, so the toolbar always reflects the latest pass. */
function sortRunsNewestFirst(runs: TaskReviewRun[]): TaskReviewRun[] {
  return [...runs].sort((a, b) => b.created_at.localeCompare(a.created_at));
}

/**
 * Merges findings by id. Publishing the same finding again must replace it, not
 * append a duplicate — the backend supersedes repeated anchors, and the client
 * has to agree or the panel would show the issue twice.
 */
function mergeFindings(
  existing: TaskReviewFinding[],
  incoming: TaskReviewFinding[],
): TaskReviewFinding[] {
  if (incoming.length === 0) return existing;
  const byId = new Map(existing.map((f) => [f.id, f]));
  for (const finding of incoming) byId.set(finding.id, finding);
  return Array.from(byId.values());
}

/**
 * Typed as a plain factory over `set` rather than a full `StateCreator` because
 * that is all this slice uses. Declaring the unused `get`/`api` params only to
 * satisfy the broader signature makes every call site pass superfluous
 * arguments (CodeQL js/superfluous-trailing-arguments).
 */
export const createReviewSlice = (set: ImmerSet): ReviewSlice => ({
  ...defaultReviewState,

  setTaskReview: (taskId, snapshot) =>
    set((draft) => {
      draft.taskReview.runsByTaskId[taskId] = sortRunsNewestFirst(snapshot.runs).slice(
        0,
        RUN_HISTORY_LIMIT,
      );
      draft.taskReview.findingsByTaskId[taskId] = snapshot.findings;
      draft.taskReview.loadedTaskIds[taskId] = true;
    }),

  upsertReviewRun: (taskId, run) =>
    set((draft) => {
      const existing = draft.taskReview.runsByTaskId[taskId] ?? [];
      const without = existing.filter((r) => r.id !== run.id);
      draft.taskReview.runsByTaskId[taskId] = sortRunsNewestFirst([run, ...without]).slice(
        0,
        RUN_HISTORY_LIMIT,
      );
    }),

  addReviewFindings: (taskId, findings, supersededIds) =>
    set((draft) => {
      // The backend deletes a superseded finding and inserts a replacement with a
      // new id, so merging by id alone would leave both visible at the same
      // anchor until a reload. Drop the superseded ids first.
      const existing = draft.taskReview.findingsByTaskId[taskId] ?? [];
      const kept =
        supersededIds && supersededIds.length > 0
          ? existing.filter((f) => !supersededIds.includes(f.id))
          : existing;
      draft.taskReview.findingsByTaskId[taskId] = mergeFindings(kept, findings);
    }),

  updateReviewFinding: (taskId, finding) =>
    set((draft) => {
      const existing = draft.taskReview.findingsByTaskId[taskId] ?? [];
      const index = existing.findIndex((f) => f.id === finding.id);
      if (index === -1) {
        // A status change can arrive for a finding this client never saw (another
        // browser, or an event that beat the backfill). Keep it rather than
        // dropping the update.
        draft.taskReview.findingsByTaskId[taskId] = [...existing, finding];
        return;
      }
      existing[index] = finding;
    }),

  clearTaskReviewState: (taskId) =>
    set((draft) => {
      delete draft.taskReview.runsByTaskId[taskId];
      delete draft.taskReview.findingsByTaskId[taskId];
      delete draft.taskReview.loadedTaskIds[taskId];
    }),
});
