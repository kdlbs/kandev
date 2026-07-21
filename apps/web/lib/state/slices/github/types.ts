import type {
  GitHubStatus,
  GitHubRateLimitUpdate,
  TaskPR,
  TaskIssueLink,
  PRWatch,
  ReviewWatch,
  IssueWatch,
  GitHubActionPresets,
  PRFeedback,
  TaskCIAutomationOptions,
} from "@/lib/types/github";

export type GitHubStatusEntry = {
  status: GitHubStatus | null;
  loaded: boolean;
  loading: boolean;
};

export type GitHubStatusState = {
  byWorkspaceId: Record<string, GitHubStatusEntry>;
};

export type TaskPRsState = {
  /** Each task may have multiple PRs (one per repository for multi-repo tasks). */
  byTaskId: Record<string, TaskPR[]>;
};

export type TaskIssuesState = {
  workspaceId: string | null;
  byTaskId: Record<string, TaskIssueLink>;
};

export type PendingPrUrlsState = {
  /**
   * Client-only PR URLs after Create PR succeeds before TaskPR sync (e.g. Azure Repos).
   * Keyed by task id, then repo name (or "" for single-repo).
   */
  byTaskId: Record<string, Record<string, string>>;
};

export type PRWatchesState = {
  items: PRWatch[];
  loaded: boolean;
  loading: boolean;
};

export type ReviewWatchesState = {
  items: ReviewWatch[];
  loaded: boolean;
  loading: boolean;
};

export type IssueWatchesState = {
  items: IssueWatch[];
  loaded: boolean;
  loading: boolean;
};

export type ActionPresetsState = {
  byWorkspaceId: Record<string, GitHubActionPresets>;
  loading: Record<string, boolean>;
};

export type PRFeedbackCacheEntry = {
  feedback: PRFeedback;
  lastUpdatedAt: number;
};

export type PRFeedbackCacheState = {
  /** Keyed by `${owner}/${repo}#${pr_number}` so multi-PR tasks coexist. */
  byKey: Record<string, PRFeedbackCacheEntry>;
};

export type TaskCIAutomationOptionsState = {
  byTaskId: Record<string, TaskCIAutomationOptions>;
  loading: Record<string, boolean>;
  saving: Record<string, boolean>;
  errors: Record<string, string | null>;
};

export type GitHubSliceState = {
  githubStatus: GitHubStatusState;
  taskPRs: TaskPRsState;
  taskIssues: TaskIssuesState;
  pendingPrUrlByTaskId: PendingPrUrlsState;
  prWatches: PRWatchesState;
  reviewWatches: ReviewWatchesState;
  issueWatches: IssueWatchesState;
  actionPresets: ActionPresetsState;
  prFeedbackCache: PRFeedbackCacheState;
  taskCIAutomation: TaskCIAutomationOptionsState;
};

export type GitHubSliceActions = {
  setGitHubStatus: (workspaceId: string, status: GitHubStatus | null) => void;
  setGitHubStatusLoading: (workspaceId: string, loading: boolean) => void;
  resetGitHubStatus: (workspaceId: string) => void;
  setTaskPRs: (prs: Record<string, TaskPR[]>) => void;
  setTaskIssues: (workspaceId: string, issues: Record<string, TaskIssueLink>) => void;
  upsertTaskIssue: (workspaceId: string, issue: TaskIssueLink) => void;
  setTaskPR: (taskId: string, pr: TaskPR) => void;
  setPendingPrUrlForTask: (taskId: string, repoKey: string, prUrl: string) => void;
  setPRWatches: (watches: PRWatch[]) => void;
  setPRWatchesLoading: (loading: boolean) => void;
  removePRWatch: (id: string) => void;
  setReviewWatches: (watches: ReviewWatch[]) => void;
  setReviewWatchesLoading: (loading: boolean) => void;
  addReviewWatch: (watch: ReviewWatch) => void;
  updateReviewWatch: (watch: ReviewWatch) => void;
  removeReviewWatch: (id: string) => void;
  setIssueWatches: (watches: IssueWatch[]) => void;
  setIssueWatchesLoading: (loading: boolean) => void;
  addIssueWatch: (watch: IssueWatch) => void;
  updateIssueWatch: (watch: IssueWatch) => void;
  removeIssueWatch: (id: string) => void;
  setActionPresets: (workspaceId: string, presets: GitHubActionPresets) => void;
  setActionPresetsLoading: (workspaceId: string, loading: boolean) => void;
  applyGitHubRateLimitUpdate: (update: GitHubRateLimitUpdate) => void;
  setPRFeedbackCacheEntry: (key: string, feedback: PRFeedback) => void;
  removePRFeedbackCacheEntry: (key: string) => void;
  setTaskCIAutomationOptions: (taskId: string, options: TaskCIAutomationOptions) => void;
  setTaskCIAutomationLoading: (taskId: string, loading: boolean) => void;
  setTaskCIAutomationSaving: (taskId: string, saving: boolean) => void;
  setTaskCIAutomationError: (taskId: string, error: string | null) => void;
};

export type GitHubSlice = GitHubSliceState & GitHubSliceActions;
