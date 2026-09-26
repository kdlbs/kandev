import type { PluginRecord } from "@/lib/types/plugins";

export type PluginsState = {
  items: PluginRecord[];
  loading: boolean;
  loaded: boolean;
  error: string | null;
};

export type PluginsSliceState = {
  plugins: PluginsState;
};

export type PluginsSliceActions = {
  setPlugins: (plugins: PluginRecord[]) => void;
  setPluginsLoading: (loading: boolean) => void;
  setPluginsError: (error: string | null) => void;
  upsertPlugin: (plugin: PluginRecord) => void;
  updatePluginPublisher: (
    id: string,
    expectedInstallationID: string,
    expectedVersion: string,
    publisherIdentity: PluginRecord["publisher_identity"],
    publisherProvenance: PluginRecord["publisher_provenance"],
  ) => boolean;
  verifyPluginPublisher: (
    id: string,
    expectedInstallationID: string,
    expectedVersion: string,
  ) => Promise<boolean>;
  removePlugin: (id: string) => void;
};

export type PluginsSlice = PluginsSliceState & PluginsSliceActions;
