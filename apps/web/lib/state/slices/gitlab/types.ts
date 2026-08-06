import type {
  TaskMR,
  ReviewWatch,
  IssueWatch,
  MRWatch,
  GitLabStats,
  GitLabActionPresets,
  GitLabStatus,
  TaskMRAutomationOptions,
} from "@/lib/types/gitlab";

export type TaskMRsState = {
  /** Workspace -> task -> linked MRs. */
  byWorkspaceId: Record<string, Record<string, TaskMR[]>>;
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
  workspaceId: string | null;
  data: GitLabStatus | null;
  loading: boolean;
  loadedAt: number | null;
};

export type TaskMRAutomationOptionsState = {
  byTaskId: Record<string, TaskMRAutomationOptions>;
  loading: Record<string, boolean>;
  saving: Record<string, boolean>;
  errors: Record<string, string | null>;
  // Bumped only by an externally-sourced write (the WS push handler), never
  // by the hook's own refresh()/update() calls. A pending refresh or update
  // captures this counter before it starts and checks it hasn't moved before
  // committing its own result — so a WS push that lands mid-request always
  // wins over a slower local response, instead of a stale fetch/optimistic
  // rollback clobbering fresher externally-pushed state.
  externalGeneration: Record<string, number>;
};

export type GitLabSliceState = {
  taskMRs: TaskMRsState;
  gitlabReviewWatches: GitLabReviewWatchesState;
  gitlabIssueWatches: GitLabIssueWatchesState;
  gitlabMRWatches: GitLabMRWatchesState;
  gitlabActionPresets: GitLabActionPresetsState;
  gitlabStats: GitLabStatsState;
  gitlabStatus: GitLabStatusState;
  taskMRAutomation: TaskMRAutomationOptionsState;
};

export type GitLabSliceActions = {
  // TaskMR — keeping legacy names; do not collide with github slice.
  setTaskMRs: (workspaceId: string, mrs: Record<string, TaskMR[]>) => void;
  setTaskMR: (workspaceId: string, taskId: string, mr: TaskMR) => void;
  removeTaskMR: (workspaceId: string, associationId: string) => void;
  resetTaskMRs: (workspaceId?: string) => void;

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

  setGitLabStatus: (workspaceId: string | null, status: GitLabStatus | null) => void;
  setGitLabStatusLoading: (workspaceId: string | null, loading: boolean) => void;

  setTaskMRAutomationOptions: (taskId: string, options: TaskMRAutomationOptions) => void;
  setTaskMRAutomationLoading: (taskId: string, loading: boolean) => void;
  setTaskMRAutomationSaving: (taskId: string, saving: boolean) => void;
  setTaskMRAutomationError: (taskId: string, error: string | null) => void;
  markTaskMRAutomationExternalUpdate: (taskId: string) => void;
};

export type GitLabSlice = GitLabSliceState & GitLabSliceActions;
