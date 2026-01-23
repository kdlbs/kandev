import type { StateCreator } from 'zustand';
import type { SettingsSlice, SettingsSliceState } from './types';

export const defaultSettingsState: SettingsSliceState = {
  executors: { items: [] },
  environments: { items: [] },
  settingsAgents: { items: [] },
  agentDiscovery: { items: [] },
  availableAgents: { items: [], loading: false, loaded: false },
  agentProfiles: { items: [], version: 0 },
  editors: { items: [], loaded: false, loading: false },
  prompts: { items: [], loaded: false, loading: false },
  notificationProviders: {
    items: [],
    events: [],
    appriseAvailable: false,
    loaded: false,
    loading: false,
  },
  settingsData: { executorsLoaded: false, environmentsLoaded: false, agentsLoaded: false },
  userSettings: {
    workspaceId: null,
    boardId: null,
    repositoryIds: [],
    preferredShell: null,
    shellOptions: [],
    defaultEditorId: null,
    enablePreviewOnClick: false,
    loaded: false,
  },
};

export const createSettingsSlice: StateCreator<
  SettingsSlice,
  [['zustand/immer', never]],
  [],
  SettingsSlice
> = (set) => ({
  ...defaultSettingsState,
  setExecutors: (executors) =>
    set((draft) => {
      draft.executors.items = executors;
    }),
  setEnvironments: (environments) =>
    set((draft) => {
      draft.environments.items = environments;
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
});
