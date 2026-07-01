import type { PRFeedback } from "@/lib/types/github";

export type PendingPrUrlsState = {
  /**
   * Client-only PR URLs after Create PR succeeds before TaskPR sync (e.g. Azure Repos).
   * Keyed by task id, then repo name (or "" for single-repo).
   */
  byTaskId: Record<string, Record<string, string>>;
};

export type PRFeedbackCacheEntry = {
  feedback: PRFeedback;
  lastUpdatedAt: number;
};

export type PRFeedbackCacheState = {
  /** Keyed by `${owner}/${repo}#${pr_number}` so multi-PR tasks coexist. */
  byKey: Record<string, PRFeedbackCacheEntry>;
};

export type GitHubSliceState = {
  pendingPrUrlByTaskId: PendingPrUrlsState;
  prFeedbackCache: PRFeedbackCacheState;
};

export type GitHubSliceActions = {
  setPendingPrUrlForTask: (taskId: string, repoKey: string, prUrl: string) => void;
  clearPendingPrUrlForTaskPR: (
    taskId: string,
    pr: { repository_id?: string; pr_url?: string },
  ) => void;
  setPRFeedbackCacheEntry: (key: string, feedback: PRFeedback) => void;
  removePRFeedbackCacheEntry: (key: string) => void;
};

export type GitHubSlice = GitHubSliceState & GitHubSliceActions;
