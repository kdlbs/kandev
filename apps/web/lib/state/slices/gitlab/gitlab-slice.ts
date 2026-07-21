import type { StateCreator } from "zustand";
import type { GitLabSlice, GitLabSliceState } from "./types";
import { normalizeGitLabOrigin } from "@/lib/gitlab-identity";

export const defaultGitLabState: GitLabSliceState = {
  taskMRs: { byWorkspaceId: {} },
  gitlabReviewWatches: { items: [], loaded: false, loading: false },
  gitlabIssueWatches: { items: [], loaded: false, loading: false },
  gitlabMRWatches: { items: [], loaded: false, loading: false },
  gitlabActionPresets: { byWorkspaceId: {}, loading: false },
  gitlabStats: { data: null, loading: false, loadedAt: null },
  gitlabStatus: { workspaceId: null, data: null, loading: false, loadedAt: null },
};

type ImmerSet = Parameters<
  StateCreator<GitLabSlice, [["zustand/immer", never]], [], GitLabSlice>
>[0];

type GitLabSliceCreator = StateCreator<GitLabSlice, [["zustand/immer", never]], [], GitLabSlice>;

export const createGitLabSlice: GitLabSliceCreator = (set: ImmerSet) => ({
  ...defaultGitLabState,
  ...taskMRActions(set),
  ...reviewWatchActions(set),
  ...issueWatchActions(set),
  ...mrWatchActions(set),
  ...presetActions(set),
  ...statsActions(set),
  ...statusActions(set),
});

function taskMRActions(set: ImmerSet) {
  return {
    setTaskMRs: (workspaceId: string, mrs: GitLabSliceState["taskMRs"]["byWorkspaceId"][string]) =>
      set((draft) => {
        draft.taskMRs.byWorkspaceId[workspaceId] = mrs;
      }),
    setTaskMR: (
      workspaceId: string,
      taskId: string,
      mr: GitLabSliceState["taskMRs"]["byWorkspaceId"][string][string][number],
    ) =>
      set((draft) => {
        const workspaceMRs = draft.taskMRs.byWorkspaceId[workspaceId] ?? {};
        const existing = workspaceMRs[taskId] ?? [];
        const repoKey = mr.repository_id ?? "";
        const idx = existing.findIndex(
          (m) =>
            (m.repository_id ?? "") === repoKey &&
            normalizeGitLabOrigin(m.host) === normalizeGitLabOrigin(mr.host) &&
            m.project_path === mr.project_path &&
            m.mr_iid === mr.mr_iid,
        );
        if (idx >= 0) existing[idx] = mr;
        else existing.push(mr);
        workspaceMRs[taskId] = existing;
        draft.taskMRs.byWorkspaceId[workspaceId] = workspaceMRs;
      }),
    removeTaskMR: (workspaceId: string, associationId: string) =>
      set((draft) => {
        const workspaceMRs = draft.taskMRs.byWorkspaceId[workspaceId];
        if (!workspaceMRs) return;
        for (const taskId of Object.keys(workspaceMRs)) {
          workspaceMRs[taskId] = (workspaceMRs[taskId] ?? []).filter(
            (mr) => mr.id !== associationId,
          );
        }
      }),
    resetTaskMRs: (workspaceId?: string) =>
      set((draft) => {
        if (workspaceId) {
          delete draft.taskMRs.byWorkspaceId[workspaceId];
          return;
        }
        draft.taskMRs.byWorkspaceId = {};
      }),
  };
}

function reviewWatchActions(set: ImmerSet) {
  return {
    setGitLabReviewWatches: (watches: GitLabSliceState["gitlabReviewWatches"]["items"]) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = watches;
        draft.gitlabReviewWatches.loaded = true;
      }),
    setGitLabReviewWatchesLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabReviewWatches.loading = loading;
      }),
    addGitLabReviewWatch: (watch: GitLabSliceState["gitlabReviewWatches"]["items"][number]) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = [...draft.gitlabReviewWatches.items, watch];
      }),
    updateGitLabReviewWatchInStore: (
      watch: GitLabSliceState["gitlabReviewWatches"]["items"][number],
    ) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = draft.gitlabReviewWatches.items.map((w) =>
          w.id === watch.id ? watch : w,
        );
      }),
    removeGitLabReviewWatch: (id: string) =>
      set((draft) => {
        draft.gitlabReviewWatches.items = draft.gitlabReviewWatches.items.filter(
          (w) => w.id !== id,
        );
      }),
  };
}

function issueWatchActions(set: ImmerSet) {
  return {
    setGitLabIssueWatches: (watches: GitLabSliceState["gitlabIssueWatches"]["items"]) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = watches;
        draft.gitlabIssueWatches.loaded = true;
      }),
    setGitLabIssueWatchesLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabIssueWatches.loading = loading;
      }),
    addGitLabIssueWatch: (watch: GitLabSliceState["gitlabIssueWatches"]["items"][number]) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = [...draft.gitlabIssueWatches.items, watch];
      }),
    updateGitLabIssueWatchInStore: (
      watch: GitLabSliceState["gitlabIssueWatches"]["items"][number],
    ) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = draft.gitlabIssueWatches.items.map((w) =>
          w.id === watch.id ? watch : w,
        );
      }),
    removeGitLabIssueWatch: (id: string) =>
      set((draft) => {
        draft.gitlabIssueWatches.items = draft.gitlabIssueWatches.items.filter((w) => w.id !== id);
      }),
  };
}

function mrWatchActions(set: ImmerSet) {
  return {
    setGitLabMRWatches: (watches: GitLabSliceState["gitlabMRWatches"]["items"]) =>
      set((draft) => {
        draft.gitlabMRWatches.items = watches;
        draft.gitlabMRWatches.loaded = true;
      }),
    setGitLabMRWatchesLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabMRWatches.loading = loading;
      }),
    removeGitLabMRWatch: (id: string) =>
      set((draft) => {
        draft.gitlabMRWatches.items = draft.gitlabMRWatches.items.filter((w) => w.id !== id);
      }),
  };
}

function presetActions(set: ImmerSet) {
  return {
    setGitLabActionPresets: (
      workspaceId: string,
      presets: GitLabSliceState["gitlabActionPresets"]["byWorkspaceId"][string],
    ) =>
      set((draft) => {
        draft.gitlabActionPresets.byWorkspaceId[workspaceId] = presets;
      }),
    setGitLabActionPresetsLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabActionPresets.loading = loading;
      }),
  };
}

function statsActions(set: ImmerSet) {
  return {
    setGitLabStats: (stats: GitLabSliceState["gitlabStats"]["data"]) =>
      set((draft) => {
        draft.gitlabStats.data = stats;
        draft.gitlabStats.loadedAt = Date.now();
      }),
    setGitLabStatsLoading: (loading: boolean) =>
      set((draft) => {
        draft.gitlabStats.loading = loading;
      }),
  };
}

function statusActions(set: ImmerSet) {
  return {
    setGitLabStatus: (
      workspaceId: string | null,
      status: GitLabSliceState["gitlabStatus"]["data"],
    ) =>
      set((draft) => {
        draft.gitlabStatus.workspaceId = workspaceId;
        draft.gitlabStatus.data = status;
        draft.gitlabStatus.loadedAt = Date.now();
      }),
    setGitLabStatusLoading: (workspaceId: string | null, loading: boolean) =>
      set((draft) => {
        draft.gitlabStatus.workspaceId = workspaceId;
        draft.gitlabStatus.loading = loading;
      }),
  };
}
