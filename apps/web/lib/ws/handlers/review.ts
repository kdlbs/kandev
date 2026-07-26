import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/**
 * Live native code-review updates. Findings are backend-persisted, so unlike
 * pending inline comments they arrive here rather than living only in this
 * browser's sessionStorage.
 */
export function registerReviewHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.review.run_updated": (message) => {
      const { task_id, run } = message.payload;
      if (!task_id || !run) return;
      store.getState().upsertReviewRun(task_id, run);
    },
    "task.review.findings_published": (message) => {
      const { task_id, findings, superseded_ids } = message.payload;
      if (!task_id || !findings?.length) return;
      store.getState().addReviewFindings(task_id, findings, superseded_ids);
    },
    "task.review.finding_updated": (message) => {
      const { task_id, finding } = message.payload;
      if (!task_id || !finding) return;
      store.getState().updateReviewFinding(task_id, finding);
    },
    "task.review.cleared": (message) => {
      const { task_id } = message.payload;
      if (!task_id) return;
      store.getState().clearTaskReviewState(task_id);
    },
  };
}
