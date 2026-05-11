import type { StateCreator } from "zustand";
import type { SettingsSlice, SettingsSliceState } from "./types";

export const defaultSettingsState: SettingsSliceState = {
  executors: { items: [] },
  settingsAgents: { items: [] },
  agentDiscovery: { items: [], loading: false, loaded: false },
  availableAgents: { items: [], tools: [], loading: false, loaded: false },
  agentProfiles: { items: [], version: 0 },
  installJobs: { byAgent: {} },
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
    terminalLinkBehavior: "new_tab",
    chatSubmitKey: "cmd_enter",
    reviewAutoMarkOnScroll: true,
    showReleaseNotification: true,
    releaseNotesLastSeenVersion: null,
    lspAutoStartLanguages: [],
    lspAutoInstallLanguages: [],
    lspServerConfigs: {},
    savedLayouts: [],
    sidebarViews: [],
    defaultUtilityAgentId: null,
    keyboardShortcuts: {},
    terminalFontFamily: null,
    terminalFontSize: null,
    loaded: false,
  },
};

type ImmerSet = Parameters<
  StateCreator<SettingsSlice, [["zustand/immer", never]], [], SettingsSlice>
>[0];

// installJobStartedAtMs parses started_at to epoch ms so we compare on time,
// not lexicographically. RFC3339 strings with variable fractional seconds
// would otherwise misorder ("2026-05-11T10:00:00Z" sorts after
// "2026-05-11T10:00:00.1Z" despite being older).
function installJobStartedAtMs(job: { started_at: string }): number {
  return Date.parse(job.started_at);
}

function createInstallJobActions(
  set: ImmerSet,
): Pick<
  SettingsSlice,
  "setInstallJobs" | "upsertInstallJob" | "appendInstallOutput" | "clearInstallJob"
> {
  return {
    setInstallJobs: (jobs) =>
      set((draft) => {
        const byAgent: Record<string, (typeof jobs)[number]> = {};
        for (const job of jobs) {
          // If two jobs target the same agent (a current run + a stale
          // finished snapshot in retention), prefer the newest start.
          const existing = byAgent[job.agent_name];
          if (!existing || installJobStartedAtMs(job) > installJobStartedAtMs(existing)) {
            byAgent[job.agent_name] = job;
          }
        }
        draft.installJobs.byAgent = byAgent;
      }),
    upsertInstallJob: (job) =>
      set((draft) => {
        const current = draft.installJobs.byAgent[job.agent_name];
        // Drop stale events from a previous job_id (e.g. after retry).
        if (
          current &&
          current.job_id !== job.job_id &&
          installJobStartedAtMs(current) > installJobStartedAtMs(job)
        ) {
          return;
        }
        draft.installJobs.byAgent[job.agent_name] = job;
      }),
    appendInstallOutput: (agentName, chunk) =>
      set((draft) => {
        const current = draft.installJobs.byAgent[agentName];
        if (!current) return;
        const next = (current.output ?? "") + chunk;
        // Cap at 64KB; drop oldest chars on overflow so the live tail stays current.
        const max = 64 * 1024;
        current.output = next.length > max ? next.slice(next.length - max) : next;
      }),
    clearInstallJob: (agentName) =>
      set((draft) => {
        delete draft.installJobs.byAgent[agentName];
      }),
  };
}

function createCoreActions(
  set: ImmerSet,
): Pick<
  SettingsSlice,
  | "setExecutors"
  | "setSettingsAgents"
  | "setAgentDiscovery"
  | "setAgentDiscoveryLoading"
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
        draft.agentDiscovery.loading = false;
        draft.agentDiscovery.loaded = true;
      }),
    setAgentDiscoveryLoading: (loading) =>
      set((draft) => {
        draft.agentDiscovery.loading = loading;
      }),
    setAvailableAgents: (agents, tools) =>
      set((draft) => {
        draft.availableAgents.items = agents;
        if (tools) draft.availableAgents.tools = tools;
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
  ...createInstallJobActions(set),
  ...createSecretAndSpriteActions(set),
});
