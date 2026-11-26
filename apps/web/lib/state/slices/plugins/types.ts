import type { ThemeInventory, ThemeDefinition } from "@/lib/theme/theme-types";
import type { PluginRecord } from "@/lib/types/plugins";

export type PluginsState = {
  items: PluginRecord[];
  loading: boolean;
  loaded: boolean;
  error: string | null;
};

export type PluginsSliceState = {
  plugins: PluginsState;
  themeInventory: ThemeInventory | null;
  themeDefinitions: Record<string, ThemeDefinition>;
  themeError: string | null;
};

export type PluginsSliceActions = {
  setThemeInventory: (inventory: ThemeInventory) => void;
  setThemeDefinition: (digest: string, definition: ThemeDefinition) => void;
  setThemeError: (error: string | null) => void;
  resetThemes: () => void;
  setPlugins: (plugins: PluginRecord[]) => void;
  setPluginsLoading: (loading: boolean) => void;
  setPluginsError: (error: string | null) => void;
  upsertPlugin: (plugin: PluginRecord) => void;
  removePlugin: (id: string) => void;
};

export type PluginsSlice = PluginsSliceState & PluginsSliceActions;
