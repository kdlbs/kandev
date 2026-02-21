import type { StateCreator } from "zustand";
import type { SettingsSlice, SettingsSliceState } from "./types";

export const defaultSettingsState: SettingsSliceState = {
  executors: { items: [] },
  settingsAgents: { items: [] },
  agentDiscovery: { items: [] },
  availableAgents: { items: [], loading: false, loaded: false },
  agentProfiles: { items: [], version: 0 },
  editors: { items: [], loaded: false, loading: false },
  prompts: { items: [], loaded: false, loading: false },
  secrets: { items: [], loaded: false, loading: false },
  sprites: { status: null, instances: [], loaded: false, loading: false },
  notificationProviders: {
    items: [],
    events: [],
    appriseAvailable: false,
    loaded: false,
    loading: false,
  },
  settingsData: { executorsLoaded: false, agentsLoaded: false },
  userSettings: {
    workspaceId: null,
    kanbanViewMode: null,
    workflowId: null,
    repositoryIds: [],
    preferredShell: null,
    shellOptions: [],
    defaultEditorId: null,
    enablePreviewOnClick: false,
    chatSubmitKey: "cmd_enter",
    reviewAutoMarkOnScroll: true,
    lspAutoStartLanguages: [],
    lspAutoInstallLanguages: [],
    lspServerConfigs: {},
    savedLayouts: [],
    loaded: false,
  },
};

type ImmerSet = Parameters<
  StateCreator<SettingsSlice, [["zustand/immer", never]], [], SettingsSlice>
>[0];

function createCoreActions(
  set: ImmerSet,
): Pick<
  SettingsSlice,
  | "setExecutors"
  | "setSettingsAgents"
  | "setAgentDiscovery"
  | "setAvailableAgents"
  | "setAvailableAgentsLoading"
  | "setAgentProfiles"
  | "setEditors"
  | "setEditorsLoading"
  | "setPrompts"
  | "setPromptsLoading"
  | "setSettingsData"
  | "setUserSettings"
  | "bumpAgentProfilesVersion"
> {
  return {
    setExecutors: (executors) =>
      set((draft) => {
        draft.executors.items = executors;
      }),
    setSettingsAgents: (agents) =>
      set((draft) => {
        draft.settingsAgents.items = agents;
      }),
    setAgentDiscovery: (agents) =>
      set((draft) => {
        draft.agentDiscovery.items = agents;
      }),
    setAvailableAgents: (agents) =>
      set((draft) => {
        draft.availableAgents.items = agents;
        draft.availableAgents.loading = false;
        draft.availableAgents.loaded = true;
      }),
    setAvailableAgentsLoading: (loading) =>
      set((draft) => {
        draft.availableAgents.loading = loading;
      }),
    setAgentProfiles: (profiles) =>
      set((draft) => {
        draft.agentProfiles.items = profiles;
      }),
    setEditors: (editors) =>
      set((draft) => {
        draft.editors.items = editors;
        draft.editors.loaded = true;
      }),
    setEditorsLoading: (loading) =>
      set((draft) => {
        draft.editors.loading = loading;
      }),
    setPrompts: (prompts) =>
      set((draft) => {
        draft.prompts.items = prompts;
        draft.prompts.loaded = true;
      }),
    setPromptsLoading: (loading) =>
      set((draft) => {
        draft.prompts.loading = loading;
      }),
    setSettingsData: (next) =>
      set((draft) => {
        draft.settingsData = { ...draft.settingsData, ...next };
      }),
    setUserSettings: (settings) =>
      set((draft) => {
        draft.userSettings = settings;
      }),
    bumpAgentProfilesVersion: () =>
      set((draft) => {
        draft.agentProfiles.version += 1;
      }),
  };
}

function createSecretAndSpriteActions(
  set: ImmerSet,
): Pick<
  SettingsSlice,
  | "setSecrets"
  | "setSecretsLoading"
  | "addSecret"
  | "updateSecret"
  | "removeSecret"
  | "setSpritesStatus"
  | "setSpritesInstances"
  | "setSpritesLoading"
  | "removeSpritesInstance"
  | "setNotificationProviders"
  | "setNotificationProvidersLoading"
> {
  return {
    setSecrets: (items) =>
      set((draft) => {
        draft.secrets.items = items;
        draft.secrets.loaded = true;
      }),
    setSecretsLoading: (loading) =>
      set((draft) => {
        draft.secrets.loading = loading;
      }),
    addSecret: (item) =>
      set((draft) => {
        draft.secrets.items = [...draft.secrets.items.filter((s) => s.id !== item.id), item];
      }),
    updateSecret: (item) =>
      set((draft) => {
        const idx = draft.secrets.items.findIndex((s) => s.id === item.id);
        if (idx >= 0) draft.secrets.items[idx] = { ...draft.secrets.items[idx], ...item };
      }),
    removeSecret: (id) =>
      set((draft) => {
        draft.secrets.items = draft.secrets.items.filter((s) => s.id !== id);
      }),
    setSpritesStatus: (status) =>
      set((draft) => {
        draft.sprites.status = status;
        draft.sprites.loaded = true;
      }),
    setSpritesInstances: (instances) =>
      set((draft) => {
        draft.sprites.instances = instances;
        draft.sprites.loaded = true;
      }),
    setSpritesLoading: (loading) =>
      set((draft) => {
        draft.sprites.loading = loading;
      }),
    removeSpritesInstance: (name) =>
      set((draft) => {
        draft.sprites.instances = draft.sprites.instances.filter((i) => i.name !== name);
        if (draft.sprites.status) {
          draft.sprites.status.instance_count = draft.sprites.instances.length;
        }
      }),
    setNotificationProviders: (state) =>
      set((draft) => {
        draft.notificationProviders.items = state.items;
        draft.notificationProviders.events = state.events;
        draft.notificationProviders.appriseAvailable = state.appriseAvailable;
        draft.notificationProviders.loaded = state.loaded;
        draft.notificationProviders.loading = state.loading;
      }),
    setNotificationProvidersLoading: (loading) =>
      set((draft) => {
        draft.notificationProviders.loading = loading;
      }),
  };
}

export const createSettingsSlice: StateCreator<
  SettingsSlice,
  [["zustand/immer", never]],
  [],
  SettingsSlice
> = (set) => ({
  ...defaultSettingsState,
  ...createCoreActions(set),
  ...createSecretAndSpriteActions(set),
});
