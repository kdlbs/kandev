import type { GitHubStatus, TaskPR, PRWatch, ReviewWatch } from "@/lib/types/github";

export type GitHubStatusState = {
  status: GitHubStatus | null;
  loaded: boolean;
  loading: boolean;
};

export type TaskPRsState = {
  byTaskId: Record<string, TaskPR>;
  loaded: boolean;
  loading: boolean;
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

export type GitHubSliceState = {
  githubStatus: GitHubStatusState;
  taskPRs: TaskPRsState;
  prWatches: PRWatchesState;
  reviewWatches: ReviewWatchesState;
};

export type GitHubSliceActions = {
  setGitHubStatus: (status: GitHubStatus | null) => void;
  setGitHubStatusLoading: (loading: boolean) => void;
  setTaskPRs: (prs: Record<string, TaskPR>) => void;
  setTaskPR: (taskId: string, pr: TaskPR) => void;
  removeTaskPR: (taskId: string) => void;
  setTaskPRsLoading: (loading: boolean) => void;
  setPRWatches: (watches: PRWatch[]) => void;
  setPRWatchesLoading: (loading: boolean) => void;
  removePRWatch: (id: string) => void;
  setReviewWatches: (watches: ReviewWatch[]) => void;
  setReviewWatchesLoading: (loading: boolean) => void;
  addReviewWatch: (watch: ReviewWatch) => void;
  updateReviewWatch: (watch: ReviewWatch) => void;
  removeReviewWatch: (id: string) => void;
};

export type GitHubSlice = GitHubSliceState & GitHubSliceActions;
