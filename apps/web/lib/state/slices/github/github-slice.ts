import type { StateCreator } from "zustand";
import type { GitHubSlice, GitHubSliceState } from "./types";

export const defaultGitHubState: GitHubSliceState = {
  pendingPrUrlByTaskId: { byTaskId: {} },
  prFeedbackCache: { byKey: {} },
};

const PR_FEEDBACK_CACHE_LIMIT = 20;

type ImmerSet = Parameters<
  StateCreator<GitHubSlice, [["zustand/immer", never]], [], GitHubSlice>
>[0];

function clearPendingPrUrlForRepo(draft: GitHubSlice, taskId: string, repoKey: string) {
  const pending = draft.pendingPrUrlByTaskId.byTaskId[taskId];
  if (!pending) return;
  delete pending[repoKey];
  if (Object.keys(pending).length === 0) {
    delete draft.pendingPrUrlByTaskId.byTaskId[taskId];
  }
}

/** Clear client-only pending URLs for the repo that just synced (not sibling repos). */
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

function createPendingPrUrlActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setPendingPrUrlForTask" | "clearPendingPrUrlForTaskPR"> {
  return {
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
    clearPendingPrUrlForTaskPR: (taskId, pr) =>
      set((draft) => {
        clearPendingForTaskPR(draft, taskId, pr);
      }),
  };
}

function createPRFeedbackCacheActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setPRFeedbackCacheEntry" | "removePRFeedbackCacheEntry"> {
  return {
    setPRFeedbackCacheEntry: (key, feedback) =>
      set((draft) => {
        draft.prFeedbackCache.byKey[key] = { feedback, lastUpdatedAt: Date.now() };
        // Bound cache size: drop the oldest entries when over the limit so a
        // user opening many PRs doesn't grow the slice unboundedly.
        const entries = Object.entries(draft.prFeedbackCache.byKey);
        if (entries.length > PR_FEEDBACK_CACHE_LIMIT) {
          entries.sort((a, b) => a[1].lastUpdatedAt - b[1].lastUpdatedAt);
          const drop = entries.length - PR_FEEDBACK_CACHE_LIMIT;
          for (let i = 0; i < drop; i++) {
            delete draft.prFeedbackCache.byKey[entries[i][0]];
          }
        }
      }),
    removePRFeedbackCacheEntry: (key) =>
      set((draft) => {
        delete draft.prFeedbackCache.byKey[key];
      }),
  };
}

export const createGitHubSlice: StateCreator<
  GitHubSlice,
  [["zustand/immer", never]],
  [],
  GitHubSlice
> = (set) => ({
  ...defaultGitHubState,
  ...createPendingPrUrlActions(set),
  ...createPRFeedbackCacheActions(set),
});
