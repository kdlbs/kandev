import type { StateCreator } from "zustand";
import type { GitHubSlice, GitHubSliceState } from "./types";
import type { TaskPR } from "@/lib/types/github";

export const defaultGitHubState: GitHubSliceState = {
  pendingPrUrlByTaskId: { byTaskId: {} },
};

function clearPendingPrUrlForRepo(draft: GitHubSlice, taskId: string, repoKey: string) {
  const pending = draft.pendingPrUrlByTaskId.byTaskId[taskId];
  if (!pending) return;
  delete pending[repoKey];
  if (Object.keys(pending).length === 0) {
    delete draft.pendingPrUrlByTaskId.byTaskId[taskId];
  }
}

/** Clear the client-only pending URL for the repo that just synced (not siblings). */
function clearPendingForTaskPR(
  draft: GitHubSlice,
  taskId: string,
  pr: { repository_id?: string; pr_url?: string },
) {
  clearPendingPrUrlForRepo(draft, taskId, pr.repository_id ?? "");
  clearPendingPrUrlForRepo(draft, taskId, "");
  const pending = draft.pendingPrUrlByTaskId.byTaskId[taskId];
  if (!pending || !pr.pr_url) return;
  for (const key of Object.keys(pending)) {
    if (pending[key] === pr.pr_url) clearPendingPrUrlForRepo(draft, taskId, key);
  }
}

/**
 * Client-only GitHub state.
 *
 * `pendingPrUrlByTaskId` holds optimistic PR URLs surfaced right after Create
 * PR succeeds, before the backend TaskPR sync lands (e.g. Azure Repos). It is
 * set by the UI, never by a fetch or WS event, so it stays in Zustand. All
 * server-owned GitHub state (taskPRs, watches, presets, status, rate limit)
 * lives in the TanStack Query cache.
 */
export const createGitHubSlice: StateCreator<
  GitHubSlice,
  [["zustand/immer", never]],
  [],
  GitHubSlice
> = (set) => ({
  ...defaultGitHubState,
  setPendingPrUrlForTask: (taskId, repoKey, prUrl) =>
    set((draft) => {
      const trimmed = prUrl.trim();
      if (!trimmed) {
        clearPendingPrUrlForRepo(draft, taskId, repoKey);
        return;
      }
      if (!draft.pendingPrUrlByTaskId.byTaskId[taskId]) {
        draft.pendingPrUrlByTaskId.byTaskId[taskId] = {};
      }
      draft.pendingPrUrlByTaskId.byTaskId[taskId][repoKey] = trimmed;
    }),
  reconcilePendingPrUrls: (taskId, prs: TaskPR[]) =>
    set((draft) => {
      if (!draft.pendingPrUrlByTaskId.byTaskId[taskId]) return;
      for (const pr of prs) {
        clearPendingForTaskPR(draft, taskId, pr);
      }
    }),
});
