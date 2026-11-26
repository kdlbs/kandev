import type { SliceCreator } from "@/lib/state/slice-creator";

import type { PluginsSlice, PluginsSliceState } from "./types";

export const defaultPluginsState: PluginsSliceState = {
  themeInventory: null,
  themeDefinitions: {},
  themeError: null,
  plugins: { items: [], loading: false, loaded: false, error: null },
};

type ImmerSet = Parameters<SliceCreator<PluginsSlice>>[0];

export const createPluginsSlice: SliceCreator<PluginsSlice> = (set: ImmerSet) => ({
  ...defaultPluginsState,
  setThemeInventory: (inventory) =>
    set((draft) => {
      const current = draft.themeInventory;
      if (current?.boot_id === inventory.boot_id && current.revision > inventory.revision) return;
      draft.themeInventory = inventory;
      draft.themeError = null;
    }),
  setThemeDefinition: (digest, definition) =>
    set((draft) => {
      const entries = Object.entries(draft.themeDefinitions)
        .filter(([key]) => key !== digest)
        .slice(-3);
      draft.themeDefinitions = { ...Object.fromEntries(entries), [digest]: definition };
    }),
  setThemeError: (error) =>
    set((draft) => {
      draft.themeError = error;
    }),
  resetThemes: () =>
    set((draft) => {
      draft.themeInventory = null;
      draft.themeDefinitions = {};
      draft.themeError = null;
    }),
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
