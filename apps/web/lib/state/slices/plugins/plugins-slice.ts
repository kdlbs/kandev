import type { SliceCreator } from "@/lib/state/slice-creator";

import type { PluginsSlice, PluginsSliceState } from "./types";

export const defaultPluginsState: PluginsSliceState = {
  plugins: { items: [], loading: false, loaded: false, error: null },
};

type ImmerSet = Parameters<SliceCreator<PluginsSlice>>[0];

export const createPluginsSlice: SliceCreator<PluginsSlice> = (set: ImmerSet) => ({
  ...defaultPluginsState,
  setPlugins: (plugins) =>
    set((draft) => {
      draft.plugins.items = plugins;
      draft.plugins.loaded = true;
      draft.plugins.error = null;
    }),
  setPluginsLoading: (loading) =>
    set((draft) => {
      draft.plugins.loading = loading;
    }),
  setPluginsError: (error) =>
    set((draft) => {
      draft.plugins.error = error;
    }),
  upsertPlugin: (plugin) =>
    set((draft) => {
      const idx = draft.plugins.items.findIndex((p) => p.id === plugin.id);
      if (idx >= 0) {
        draft.plugins.items[idx] = plugin;
      } else {
        draft.plugins.items.push(plugin);
      }
    }),
  removePlugin: (id) =>
    set((draft) => {
      draft.plugins.items = draft.plugins.items.filter((p) => p.id !== id);
    }),
});
