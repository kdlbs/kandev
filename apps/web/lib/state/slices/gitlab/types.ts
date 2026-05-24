import type { TaskMR } from "@/lib/types/gitlab";

export type TaskMRsState = {
  /** Each task may have multiple MRs (one per repository for multi-repo tasks). */
  byTaskId: Record<string, TaskMR[]>;
};

export type GitLabSliceState = {
  taskMRs: TaskMRsState;
};

export type GitLabSliceActions = {
  setTaskMRs: (mrs: Record<string, TaskMR[]>) => void;
  /**
   * Upsert a single MR for a task — keyed by (repository_id, project_path,
   * mr_iid) so multi-repo + multi-MR scenarios coexist without overwriting.
   */
  setTaskMR: (taskId: string, mr: TaskMR) => void;
  /** Clear all task MR state. Used on workspace switch / sign-out. */
  resetTaskMRs: () => void;
};

export type GitLabSlice = GitLabSliceState & GitLabSliceActions;
