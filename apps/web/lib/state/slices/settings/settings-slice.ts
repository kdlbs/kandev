import type { StateCreator } from "zustand";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import {
  refreshProfileCapabilities,
  refreshSettingsAgentsCapabilities,
  isStaleAvailableAgentsSnapshot,
  type SettingsSlice,
  type SettingsSliceState,
} from "./types";
import {
  mergeAgentProfileRecentUseState,
  type AgentProfileRecentUseState,
} from "@/lib/agent-profile-recent-use";

export const defaultSettingsState: SettingsSliceState = {
  executors: { items: [] },
  settingsAgents: { items: [] },
  agentDiscovery: { items: [], loading: false, loaded: false },
  availableAgents: { items: [], tools: [], loading: false, loaded: false },
  agentProfiles: { items: [], version: 0 },
  installJobs: { byAgent: {} },
  updateJobs: { byAgent: {} },
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
  sleepInhibition: { response: null, loaded: false, loading: false, error: false },
  userSettings: createDefaultUserSettings(),
  agentProfileRecentUse: { records: {}, loaded: false },
};

type ImmerSet = Parameters<
  StateCreator<SettingsSlice, [["zustand/immer", never]], [], SettingsSlice>
>[0];

const JOB_OUTPUT_MAX = 64 * 1024;

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
        current.output =
          next.length > JOB_OUTPUT_MAX ? next.slice(next.length - JOB_OUTPUT_MAX) : next;
      }),
    clearInstallJob: (agentName) =>
      set((draft) => {
        delete draft.installJobs.byAgent[agentName];
      }),
  };
}

function updateStatusRank(status: string): number {
  switch (status) {
    case "queued":
      return 0;
    case "resolving":
      return 1;
    case "updating":
      return 2;
    case "refreshing":
      return 3;
    case "succeeded":
    case "failed":
      return 4;
    default:
      return -1;
  }
}

function mergeJobOutput(current: string | undefined, incoming: string | undefined) {
  if (!current) return incoming;
  if (!incoming || current === incoming || current.startsWith(incoming)) return current;
  if (incoming.startsWith(current)) return incoming;
  return incoming.length >= current.length ? incoming : current;
}

function createAgentUpdateJobActions(
  set: ImmerSet,
): Pick<
  SettingsSlice,
  "setAgentUpdateJobs" | "upsertAgentUpdateJob" | "appendAgentUpdateOutput" | "clearAgentUpdateJob"
> {
  return {
    setAgentUpdateJobs: (jobs) =>
      set((draft) => {
        const byAgent: Record<string, (typeof jobs)[number]> = {};
        for (const job of jobs) {
          const current = byAgent[job.agent_name];
          if (!current || installJobStartedAtMs(job) > installJobStartedAtMs(current)) {
            byAgent[job.agent_name] = job;
          }
        }
        draft.updateJobs.byAgent = byAgent;
      }),
    upsertAgentUpdateJob: (job) =>
      set((draft) => {
        const current = draft.updateJobs.byAgent[job.agent_name];
        if (
          current &&
          current.job_id !== job.job_id &&
          installJobStartedAtMs(current) > installJobStartedAtMs(job)
        ) {
          return;
        }
        if (!current || current.job_id !== job.job_id) {
          draft.updateJobs.byAgent[job.agent_name] = job;
          return;
        }
        const incomingIsNewer = updateStatusRank(job.status) >= updateStatusRank(current.status);
        const merged = incomingIsNewer ? { ...current, ...job } : { ...job, ...current };
        merged.output = mergeJobOutput(current.output, job.output);
        draft.updateJobs.byAgent[job.agent_name] = merged;
      }),
    appendAgentUpdateOutput: (agentName, jobId, chunk) =>
      set((draft) => {
        const current = draft.updateJobs.byAgent[agentName];
        if (!current || current.job_id !== jobId) return;
        const next = (current.output ?? "") + chunk;
        current.output =
          next.length > JOB_OUTPUT_MAX ? next.slice(next.length - JOB_OUTPUT_MAX) : next;
      }),
    clearAgentUpdateJob: (agentName) =>
      set((draft) => {
        delete draft.updateJobs.byAgent[agentName];
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
        if (isStaleAvailableAgentsSnapshot(draft.availableAgents.items, agents)) {
          // The request completed, even though its data is stale. Keep the
          // newer snapshot but do not leave the polling indicator stuck.
          draft.availableAgents.loading = false;
          draft.availableAgents.loaded = true;
          return;
        }
        draft.availableAgents.items = agents;
        if (tools) draft.availableAgents.tools = tools;
        draft.availableAgents.loading = false;
        draft.availableAgents.loaded = true;
        // The revalidation poll in use-available-agents.ts is the only place
        // a probing/not_configured status ever settles outside a WS push
        // (agents.ts's "agent.available.updated" handler covers that path);
        // without applying the same refresh here, a settled status never
        // reaches agentProfiles/settingsAgents on a cold launch.
        draft.agentProfiles.items = refreshProfileCapabilities(draft.agentProfiles.items, agents);
        draft.settingsAgents.items = refreshSettingsAgentsCapabilities(
          draft.settingsAgents.items,
          agents,
        );
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
        const order = compareUserSettingsRevisions(settings.revision, draft.userSettings.revision);
        if (order !== null && order < 0) return;
        draft.userSettings = settings;
      }),
    bumpAgentProfilesVersion: () =>
      set((draft) => {
        draft.agentProfiles.version += 1;
      }),
  };
}

function createAgentProfileRecentUseActions(
  set: ImmerSet,
): Pick<SettingsSlice, "setAgentProfileRecentUse" | "applyAgentProfileRecentUse"> {
  return {
    setAgentProfileRecentUse: (state) =>
      set((draft) => {
        draft.agentProfileRecentUse = mergeAgentProfileRecentUseState(
          draft.agentProfileRecentUse as unknown as AgentProfileRecentUseState,
          state,
        ) as unknown as typeof draft.agentProfileRecentUse;
      }),
    applyAgentProfileRecentUse: (context, record) =>
      set((draft) => {
        const current = draft.agentProfileRecentUse.records[context];
        if (!current || record.revision >= current.revision) {
          draft.agentProfileRecentUse.records[context] = {
            profileIds: [...record.profileIds],
            revision: record.revision,
            updatedAt: record.updatedAt,
          };
        }
        draft.agentProfileRecentUse.loaded = true;
      }),
  };
}

function createSleepInhibitionActions(
  set: ImmerSet,
): Pick<
  SettingsSlice,
  "setSleepInhibition" | "setSleepInhibitionLoading" | "setSleepInhibitionError"
> {
  return {
    setSleepInhibition: (response) =>
      set((draft) => {
        draft.sleepInhibition.response = response;
        draft.sleepInhibition.loaded = true;
        draft.sleepInhibition.loading = false;
        draft.sleepInhibition.error = false;
      }),
    setSleepInhibitionLoading: (loading) =>
      set((draft) => {
        draft.sleepInhibition.loading = loading;
      }),
    setSleepInhibitionError: (error) =>
      set((draft) => {
        draft.sleepInhibition.error = error;
        if (error) draft.sleepInhibition.loaded = true;
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
  ...createAgentProfileRecentUseActions(set),
  ...createSleepInhibitionActions(set),
  ...createInstallJobActions(set),
  ...createAgentUpdateJobActions(set),
  ...createSecretAndSpriteActions(set),
});
