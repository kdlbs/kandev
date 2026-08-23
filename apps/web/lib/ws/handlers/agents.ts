import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import {
  compareTimestamps,
  isStaleAvailableAgentsSnapshot,
  refreshProfileCapabilities,
  refreshSettingsAgentsCapabilities,
  toAgentProfileOption,
} from "@/lib/state/slices/settings/types";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import type { AgentProfile } from "@/lib/types/agent-profile";

function buildProfileEntry(profile: unknown): AgentProfile {
  return normalizeAgentProfile(profile);
}

function getAgentId(raw: unknown): string {
  const obj = (raw ?? {}) as Record<string, unknown>;
  const value = obj.agentId ?? obj.agent_id;
  return typeof value === "string" ? value : "";
}

/**
 * Office-scoped profiles are owned by the office channels, never the kanban
 * settings surface (the HTTP agent list hides them via filterGlobalProfiles).
 * Defense in depth: ignore their events here so a stray broadcast cannot leak
 * them into the global settings store and selectors.
 */
function isOfficeScoped(normalized: { workspaceId?: string }): boolean {
  return Boolean(normalized.workspaceId);
}

/** Finds a profile in the settings agents list by id, across all agents. */
function findExistingProfile(state: AppState, profileId: string): AgentProfile | undefined {
  for (const item of state.settingsAgents.items) {
    const found = item.profiles.find((p) => p.id === profileId);
    if (found) return found;
  }
  return undefined;
}

// Deletion tombstones keyed by profile id -> deletion event timestamp, so a
// delayed create/update event cannot resurrect a profile that was deleted
// after the event was produced. Cleared when a genuinely newer create arrives.
// Tombstones are keyed by UUID and profiles are never recreated under the same
// ID, so the map is bounded by the number of profiles deleted in this session
// (typically single digits to low dozens — no eviction needed).
const deletionTombstones = new Map<string, string>();

/**
 * A profile event is stale when the store already holds a newer revision of
 * the profile or its option (e.g. a delayed duplicate response or a newer
 * WebSocket update arrived first), or when the profile was deleted after the
 * event was produced (tombstone). Events must never regress newer state.
 * Missing event timestamps (never produced by the backend) are treated as
 * not-stale so they cannot trip the guards.
 */
function isStaleProfileEvent(
  state: AppState,
  normalized: { id: string; updatedAt?: string },
  eventTimestamp: string | undefined,
): boolean {
  const tombstone = deletionTombstones.get(normalized.id);
  if (
    tombstone !== undefined &&
    eventTimestamp &&
    compareTimestamps(tombstone, eventTimestamp) >= 0
  ) {
    return true;
  }
  const existingProfile = findExistingProfile(state, normalized.id);
  if (existingProfile && compareTimestamps(existingProfile.updatedAt, normalized.updatedAt) > 0) {
    return true;
  }
  const existingOption = state.agentProfiles.items.find((o) => o.id === normalized.id);
  return Boolean(
    existingOption && compareTimestamps(existingOption.updatedAt, normalized.updatedAt) > 0,
  );
}

/**
 * Applies an agent.profile.created event, skipping office-scoped or stale
 * events (see isStaleProfileEvent) and clearing the deletion tombstone when a
 * genuinely newer create arrives.
 */
function handleProfileCreated(
  state: AppState,
  profile: unknown,
  eventTimestamp: string | undefined,
): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  if (isOfficeScoped(normalized)) return {};
  if (isStaleProfileEvent(state, normalized, eventTimestamp)) return {};
  deletionTombstones.delete(normalized.id); // a genuinely newer create wins
  const agentId = getAgentId(profile);
  const agent = state.settingsAgents.items.find((a) => a.id === agentId);
  const agentStub = {
    id: agentId,
    name: agent?.name ?? "",
    capability_status: agent?.capability_status,
    capability_error: agent?.capability_error,
  };
  const nextProfiles = [
    ...state.agentProfiles.items.filter((p) => p.id !== normalized.id),
    toAgentProfileOption(agentStub, normalized),
  ];
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === agentId
      ? {
          ...item,
          profiles: [
            ...item.profiles.filter((p) => p.id !== normalized.id),
            buildProfileEntry(profile),
          ],
        }
      : item,
  );
  return {
    agentProfiles: { ...state.agentProfiles, items: nextProfiles },
    settingsAgents: { items: nextAgents },
  };
}

/**
 * Applies an agent.profile.updated event, skipping office-scoped or stale
 * events so a delayed update never regresses newer stored state.
 */
function handleProfileUpdated(
  state: AppState,
  profile: unknown,
  eventTimestamp: string | undefined,
): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  if (isOfficeScoped(normalized)) return {};
  if (isStaleProfileEvent(state, normalized, eventTimestamp)) return {};
  const agentId = getAgentId(profile);
  const agent = state.settingsAgents.items.find((a) => a.id === agentId);
  const agentStub = {
    id: agentId,
    name: agent?.name ?? "",
    capability_status: agent?.capability_status,
    capability_error: agent?.capability_error,
  };
  const nextProfiles = state.agentProfiles.items.map((p) =>
    p.id === normalized.id ? toAgentProfileOption(agentStub, normalized) : p,
  );
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === agentId
      ? {
          ...item,
          profiles: item.profiles.map((p) => (p.id === normalized.id ? normalized : p)),
        }
      : item,
  );
  return {
    agentProfiles: { ...state.agentProfiles, items: nextProfiles },
    settingsAgents: { items: nextAgents },
  };
}

/**
 * Applies an agent.profile.deleted event: removes the profile from both
 * slices and records a deletion tombstone (keyed by the event timestamp) so a
 * delayed create/update cannot resurrect it. Office-scoped and stale deletes
 * (a delete whose snapshot is older than the stored revision — e.g. a delayed
 * event after a newer update already applied) are ignored so the handler
 * never regresses newer state.
 */
function handleProfileDeleted(
  state: AppState,
  profile: unknown,
  eventTimestamp: string | undefined,
): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  if (isOfficeScoped(normalized)) return {};
  if (isStaleProfileEvent(state, normalized, eventTimestamp)) return {};
  if (eventTimestamp) deletionTombstones.set(normalized.id, eventTimestamp);
  const agentId = getAgentId(profile);
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === agentId
      ? {
          ...item,
          profiles: item.profiles.filter((p) => p.id !== normalized.id),
        }
      : item,
  );
  return {
    agentProfiles: {
      ...state.agentProfiles,
      items: state.agentProfiles.items.filter((p) => p.id !== normalized.id),
    },
    settingsAgents: { items: nextAgents },
  };
}

export function registerAgentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "agent.available.updated": (message) => {
      const agents = message.payload.agents ?? [];
      store.setState((state) => {
        if (isStaleAvailableAgentsSnapshot(state.availableAgents.items, agents)) return state;
        return {
          ...state,
          availableAgents: {
            items: agents,
            tools: message.payload.tools ?? state.availableAgents.tools,
            loaded: true,
            loading: false,
          },
          agentProfiles: {
            ...state.agentProfiles,
            items: refreshProfileCapabilities(state.agentProfiles.items, agents),
          },
          settingsAgents: {
            ...state.settingsAgents,
            items: refreshSettingsAgentsCapabilities(state.settingsAgents.items, agents),
          },
        };
      });
    },
    "agent.install.started": (message) => {
      // Payload is the full job snapshot (queued → running transitions both emit this).
      store.getState().upsertInstallJob(message.payload);
    },
    "agent.install.output": (message) => {
      const { agent_name, chunk } = message.payload as {
        agent_name: string;
        chunk: string;
      };
      store.getState().appendInstallOutput(agent_name, chunk);
    },
    "agent.install.finished": (message) => {
      store.getState().upsertInstallJob(message.payload);
    },
    "agent.update.started": (message) => {
      store.getState().upsertAgentUpdateJob(message.payload);
    },
    "agent.update.output": (message) => {
      const { agent_name, job_id, chunk } = message.payload;
      store.getState().appendAgentUpdateOutput(agent_name, job_id, chunk);
    },
    "agent.update.finished": (message) => {
      store.getState().upsertAgentUpdateJob(message.payload);
    },
    "agent.updated": (message) => {
      store.setState((state) => ({
        ...state,
        agents: {
          agents: state.agents.agents.some((a) => a.id === message.payload.agentId)
            ? state.agents.agents.map((a) =>
                a.id === message.payload.agentId ? { ...a, status: message.payload.status } : a,
              )
            : [
                ...state.agents.agents,
                { id: message.payload.agentId, status: message.payload.status },
              ],
        },
      }));
    },
    // Full settings record for one agent (e.g. a custom TUI agent's MCP
    // strategy changed). Distinct from "agent.updated", which is a runtime
    // status ping keyed by {agentId, status} and feeds state.agents.
    "agent.settings.updated": (message) => {
      const incoming = message.payload?.agent;
      if (!incoming?.id) return;
      store.setState((state) => {
        const exists = state.settingsAgents.items.some((a) => a.id === incoming.id);
        if (!exists) return state;
        return {
          ...state,
          settingsAgents: {
            items: state.settingsAgents.items.map((a) =>
              // Preserve the locally normalized profiles: this event carries
              // agent-level settings, and the profile list has its own events.
              a.id === incoming.id ? { ...a, ...incoming, profiles: a.profiles } : a,
            ),
          },
        };
      });
    },
    "agent.profile.created": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileCreated(state, message.payload.profile, message.timestamp),
      }));
    },
    "agent.profile.updated": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileUpdated(state, message.payload.profile, message.timestamp),
      }));
    },
    "agent.profile.deleted": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileDeleted(state, message.payload.profile, message.timestamp),
      }));
    },
  };
}
