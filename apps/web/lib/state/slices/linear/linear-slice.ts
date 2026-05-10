import type { StateCreator } from "zustand";
import type { LinearSlice, LinearSliceState } from "./types";

export const defaultLinearState: LinearSliceState = {
  linearIssueWatches: { items: [], loaded: false, loading: false },
};

type ImmerSet = Parameters<
  StateCreator<LinearSlice, [["zustand/immer", never]], [], LinearSlice>
>[0];

export const createLinearSlice: StateCreator<
  LinearSlice,
  [["zustand/immer", never]],
  [],
  LinearSlice
> = (set: ImmerSet) => ({
  ...defaultLinearState,
  setLinearIssueWatches: (watches) =>
    set((draft) => {
      draft.linearIssueWatches.items = watches;
      draft.linearIssueWatches.loaded = true;
    }),
  setLinearIssueWatchesLoading: (loading) =>
    set((draft) => {
      draft.linearIssueWatches.loading = loading;
    }),
  addLinearIssueWatch: (watch) =>
    set((draft) => {
      draft.linearIssueWatches.items.push(watch);
    }),
  updateLinearIssueWatch: (watch) =>
    set((draft) => {
      const idx = draft.linearIssueWatches.items.findIndex((w) => w.id === watch.id);
      if (idx >= 0) {
        draft.linearIssueWatches.items[idx] = watch;
      }
    }),
  removeLinearIssueWatch: (id) =>
    set((draft) => {
      draft.linearIssueWatches.items = draft.linearIssueWatches.items.filter((w) => w.id !== id);
    }),
  resetLinearIssueWatches: () =>
    set((draft) => {
      draft.linearIssueWatches.items = [];
      draft.linearIssueWatches.loaded = false;
    }),
});
