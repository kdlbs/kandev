import type { StateCreator } from "zustand";
import type { GitHubSlice, GitHubSliceState } from "./types";

export const defaultGitHubState: GitHubSliceState = {
  githubStatus: { status: null, loaded: false, loading: false },
  taskPRs: { byTaskId: {}, loaded: false, loading: false },
  prWatches: { items: [], loaded: false, loading: false },
  reviewWatches: { items: [], loaded: false, loading: false },
};

type ImmerSet = Parameters<
  StateCreator<GitHubSlice, [["zustand/immer", never]], [], GitHubSlice>
>[0];

function createGitHubStatusActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setGitHubStatus" | "setGitHubStatusLoading"> {
  return {
    setGitHubStatus: (status) =>
      set((draft) => {
        draft.githubStatus.status = status;
        draft.githubStatus.loaded = true;
      }),
    setGitHubStatusLoading: (loading) =>
      set((draft) => {
        draft.githubStatus.loading = loading;
      }),
  };
}

function createTaskPRActions(
  set: ImmerSet,
): Pick<GitHubSlice, "setTaskPRs" | "setTaskPR" | "removeTaskPR" | "setTaskPRsLoading"> {
  return {
    setTaskPRs: (prs) =>
      set((draft) => {
        draft.taskPRs.byTaskId = prs;
        draft.taskPRs.loaded = true;
      }),
    setTaskPR: (taskId, pr) =>
      set((draft) => {
        draft.taskPRs.byTaskId[taskId] = pr;
      }),
    removeTaskPR: (taskId) =>
      set((draft) => {
        delete draft.taskPRs.byTaskId[taskId];
      }),
    setTaskPRsLoading: (loading) =>
      set((draft) => {
        draft.taskPRs.loading = loading;
      }),
  };
}

function createWatchActions(
  set: ImmerSet,
): Pick<
  GitHubSlice,
  | "setPRWatches"
  | "setPRWatchesLoading"
  | "removePRWatch"
  | "setReviewWatches"
  | "setReviewWatchesLoading"
  | "addReviewWatch"
  | "updateReviewWatch"
  | "removeReviewWatch"
> {
  return {
    setPRWatches: (watches) =>
      set((draft) => {
        draft.prWatches.items = watches;
        draft.prWatches.loaded = true;
      }),
    setPRWatchesLoading: (loading) =>
      set((draft) => {
        draft.prWatches.loading = loading;
      }),
    removePRWatch: (id) =>
      set((draft) => {
        draft.prWatches.items = draft.prWatches.items.filter((w) => w.id !== id);
      }),
    setReviewWatches: (watches) =>
      set((draft) => {
        draft.reviewWatches.items = watches;
        draft.reviewWatches.loaded = true;
      }),
    setReviewWatchesLoading: (loading) =>
      set((draft) => {
        draft.reviewWatches.loading = loading;
      }),
    addReviewWatch: (watch) =>
      set((draft) => {
        draft.reviewWatches.items = [
          ...draft.reviewWatches.items.filter((w) => w.id !== watch.id),
          watch,
        ];
        draft.reviewWatches.loaded = true;
      }),
    updateReviewWatch: (watch) =>
      set((draft) => {
        const idx = draft.reviewWatches.items.findIndex((w) => w.id === watch.id);
        if (idx >= 0) {
          draft.reviewWatches.items[idx] = watch;
        }
      }),
    removeReviewWatch: (id) =>
      set((draft) => {
        draft.reviewWatches.items = draft.reviewWatches.items.filter((w) => w.id !== id);
      }),
  };
}

export const createGitHubSlice: StateCreator<
  GitHubSlice,
  [["zustand/immer", never]],
  [],
  GitHubSlice
> = (set) => ({
  ...defaultGitHubState,
  ...createGitHubStatusActions(set),
  ...createTaskPRActions(set),
  ...createWatchActions(set),
});
