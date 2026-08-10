import type { StateCreator } from "zustand";
import type { ForgejoSlice, ForgejoSliceState } from "./types";

const initial = <T>(data: T) => ({ data, loading: false, loaded: false, error: null });

export const defaultForgejoState: ForgejoSliceState = {
  forgejoConfig: {},
  forgejoQueue: {},
  forgejoIssueWatches: {},
  forgejoReviewWatches: {},
  forgejoActionPresets: {},
};

type ImmerSet = Parameters<StateCreator<ForgejoSlice, [["zustand/immer", never]], [], ForgejoSlice>>[0];
type ForgejoSliceCreator = StateCreator<ForgejoSlice, [["zustand/immer", never]], [], ForgejoSlice>;

export const createForgejoSlice: ForgejoSliceCreator = (set: ImmerSet) => ({
  ...defaultForgejoState,
  setForgejoConfigState: (workspaceId, data, error = null) => set((state) => {
    state.forgejoConfig[workspaceId] = { data, loading: false, loaded: true, error };
  }),
  setForgejoConfigLoading: (workspaceId, loading) => set((state) => {
    state.forgejoConfig[workspaceId] = { ...(state.forgejoConfig[workspaceId] ?? initial(null)), loading };
  }),
  setForgejoQueueState: (workspaceId, data, error = null) => set((state) => {
    state.forgejoQueue[workspaceId] = { data, loading: false, loaded: true, error };
  }),
  setForgejoQueueLoading: (workspaceId, loading) => set((state) => {
    state.forgejoQueue[workspaceId] = { ...(state.forgejoQueue[workspaceId] ?? initial(null)), loading };
  }),
  setForgejoIssueWatchesState: (workspaceId, data, error = null) => set((state) => {
    state.forgejoIssueWatches[workspaceId] = { data, loading: false, loaded: true, error };
  }),
  setForgejoIssueWatchesLoading: (workspaceId, loading) => set((state) => {
    state.forgejoIssueWatches[workspaceId] = { ...(state.forgejoIssueWatches[workspaceId] ?? initial([])), loading };
  }),
  setForgejoReviewWatchesState: (workspaceId, data, error = null) => set((state) => {
    state.forgejoReviewWatches[workspaceId] = { data, loading: false, loaded: true, error };
  }),
  setForgejoReviewWatchesLoading: (workspaceId, loading) => set((state) => {
    state.forgejoReviewWatches[workspaceId] = { ...(state.forgejoReviewWatches[workspaceId] ?? initial([])), loading };
  }),
  setForgejoActionPresetsState: (workspaceId, data, error = null) => set((state) => {
    state.forgejoActionPresets[workspaceId] = { data, loading: false, loaded: true, error };
  }),
  setForgejoActionPresetsLoading: (workspaceId, loading) => set((state) => {
    state.forgejoActionPresets[workspaceId] = { ...(state.forgejoActionPresets[workspaceId] ?? initial([])), loading };
  }),
  resetForgejoWorkspaceState: (workspaceId) => set((state) => {
    delete state.forgejoConfig[workspaceId];
    delete state.forgejoQueue[workspaceId];
    delete state.forgejoIssueWatches[workspaceId];
    delete state.forgejoReviewWatches[workspaceId];
    delete state.forgejoActionPresets[workspaceId];
  }),
});
