import type { StateCreator } from "zustand";
import type { GitLabSlice, GitLabSliceState } from "./types";

export const defaultGitLabState: GitLabSliceState = {
  taskMRs: { byTaskId: {} },
};

type ImmerSet = Parameters<
  StateCreator<GitLabSlice, [["zustand/immer", never]], [], GitLabSlice>
>[0];

export const createGitLabSlice: StateCreator<
  GitLabSlice,
  [["zustand/immer", never]],
  [],
  GitLabSlice
> = (set: ImmerSet) => ({
  ...defaultGitLabState,
  setTaskMRs: (mrs) =>
    set((draft) => {
      draft.taskMRs.byTaskId = mrs;
    }),
  setTaskMR: (taskId, mr) =>
    set((draft) => {
      const existing = draft.taskMRs.byTaskId[taskId] ?? [];
      // Key by (repository_id, project_path, mr_iid) so multi-repo and
      // multiple MRs for the same task coexist. Mirrors the backend's
      // UNIQUE constraint on gitlab_task_mrs.
      const repoKey = mr.repository_id ?? "";
      const idx = existing.findIndex(
        (m) =>
          (m.repository_id ?? "") === repoKey &&
          m.project_path === mr.project_path &&
          m.mr_iid === mr.mr_iid,
      );
      if (idx >= 0) existing[idx] = mr;
      else existing.push(mr);
      draft.taskMRs.byTaskId[taskId] = existing;
    }),
  resetTaskMRs: () =>
    set((draft) => {
      draft.taskMRs.byTaskId = {};
    }),
});
