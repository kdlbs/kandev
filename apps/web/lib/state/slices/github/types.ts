export type PendingPrUrlsState = {
  /**
   * Client-only PR URLs after Create PR succeeds before TaskPR sync (e.g. Azure Repos).
   * Keyed by task id, then repo name (or "" for single-repo).
   */
  byTaskId: Record<string, Record<string, string>>;
};

export type GitHubSliceState = {
  pendingPrUrlByTaskId: PendingPrUrlsState;
};

export type GitHubSliceActions = {
  setPendingPrUrlForTask: (taskId: string, repoKey: string, prUrl: string) => void;
  /** Clear pending URLs once their repos' real TaskPRs land in the TQ cache. */
  reconcilePendingPrUrls: (taskId: string, prs: import("@/lib/types/github").TaskPR[]) => void;
};

export type GitHubSlice = GitHubSliceState & GitHubSliceActions;
