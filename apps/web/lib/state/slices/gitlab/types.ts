import type {
  TaskMR,
  ReviewWatch,
  IssueWatch,
  MRWatch,
  GitLabStats,
  GitLabActionPresets,
  GitLabStatus,
} from "@/lib/types/gitlab";

export type TaskMRsState = {
  /** Each task may have multiple MRs (one per repository for multi-repo tasks). */
  byTaskId: Record<string, TaskMR[]>;
};

export type LoadableList<T> = {
  items: T[];
  loaded: boolean;
  loading: boolean;
};

export type GitLabReviewWatchesState = LoadableList<ReviewWatch>;
export type GitLabIssueWatchesState = LoadableList<IssueWatch>;
export type GitLabMRWatchesState = LoadableList<MRWatch>;

export type GitLabActionPresetsState = {
  byWorkspaceId: Record<string, GitLabActionPresets>;
  loading: boolean;
};

export type GitLabStatsState = {
  data: GitLabStats | null;
  loading: boolean;
  loadedAt: number | null;
};

export type GitLabStatusState = {
  data: GitLabStatus | null;
  loading: boolean;
  loadedAt: number | null;
};

export type GitLabSliceState = {
  taskMRs: TaskMRsState;
  gitlabReviewWatches: GitLabReviewWatchesState;
  gitlabIssueWatches: GitLabIssueWatchesState;
  gitlabMRWatches: GitLabMRWatchesState;
  gitlabActionPresets: GitLabActionPresetsState;
  gitlabStats: GitLabStatsState;
  gitlabStatus: GitLabStatusState;
};

export type GitLabSliceActions = {
  // TaskMR — keeping legacy names; do not collide with github slice.
  setTaskMRs: (mrs: Record<string, TaskMR[]>) => void;
  setTaskMR: (taskId: string, mr: TaskMR) => void;
  resetTaskMRs: () => void;

  setGitLabReviewWatches: (watches: ReviewWatch[]) => void;
  setGitLabReviewWatchesLoading: (loading: boolean) => void;
  addGitLabReviewWatch: (watch: ReviewWatch) => void;
  updateGitLabReviewWatchInStore: (watch: ReviewWatch) => void;
  removeGitLabReviewWatch: (id: string) => void;

  setGitLabIssueWatches: (watches: IssueWatch[]) => void;
  setGitLabIssueWatchesLoading: (loading: boolean) => void;
  addGitLabIssueWatch: (watch: IssueWatch) => void;
  updateGitLabIssueWatchInStore: (watch: IssueWatch) => void;
  removeGitLabIssueWatch: (id: string) => void;

  setGitLabMRWatches: (watches: MRWatch[]) => void;
  setGitLabMRWatchesLoading: (loading: boolean) => void;
  removeGitLabMRWatch: (id: string) => void;

  setGitLabActionPresets: (workspaceId: string, presets: GitLabActionPresets) => void;
  setGitLabActionPresetsLoading: (loading: boolean) => void;

  setGitLabStats: (stats: GitLabStats | null) => void;
  setGitLabStatsLoading: (loading: boolean) => void;

  setGitLabStatus: (status: GitLabStatus | null) => void;
  setGitLabStatusLoading: (loading: boolean) => void;
};

export type GitLabSlice = GitLabSliceState & GitLabSliceActions;
