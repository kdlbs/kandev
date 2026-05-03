import type {
  GitHubStatus,
  TaskPR,
  PRWatch,
  ReviewWatch,
  IssueWatch,
  GitHubActionPresets,
} from "@/lib/types/github";

export type GitHubStatusState = {
  status: GitHubStatus | null;
  loaded: boolean;
  loading: boolean;
};

export type TaskPRsState = {
  /** Each task may have multiple PRs (one per repository for multi-repo tasks). */
  byTaskId: Record<string, TaskPR[]>;
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

export type GitHubSliceState = {
  githubStatus: GitHubStatusState;
  taskPRs: TaskPRsState;
  prWatches: PRWatchesState;
  reviewWatches: ReviewWatchesState;
  issueWatches: IssueWatchesState;
  actionPresets: ActionPresetsState;
};

export type GitHubSliceActions = {
  setGitHubStatus: (status: GitHubStatus | null) => void;
  setGitHubStatusLoading: (loading: boolean) => void;
  setTaskPRs: (prs: Record<string, TaskPR[]>) => void;
  setTaskPR: (taskId: string, pr: TaskPR) => void;
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
};

export type GitHubSlice = GitHubSliceState & GitHubSliceActions;
