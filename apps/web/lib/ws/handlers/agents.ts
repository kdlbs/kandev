import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
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

function handleProfileCreated(state: AppState, profile: unknown): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  const agentId = getAgentId(profile);
  const agent = state.settingsAgents.items.find((a) => a.id === agentId);
  const agentStub = { id: agentId, name: agent?.name ?? "" };
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

function handleProfileUpdated(state: AppState, profile: unknown): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  const agentId = getAgentId(profile);
  const agent = state.settingsAgents.items.find((a) => a.id === agentId);
  const agentStub = { id: agentId, name: agent?.name ?? "" };
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

export function registerAgentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "agent.available.updated": (message) => {
      store.setState((state) => ({
        ...state,
        availableAgents: {
          items: message.payload.agents ?? [],
          tools: message.payload.tools ?? state.availableAgents.tools,
          loaded: true,
          loading: false,
        },
      }));
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
    "agent.profile.created": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileCreated(state, message.payload.profile),
      }));
    },
    "agent.profile.updated": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileUpdated(state, message.payload.profile),
      }));
    },
    "agent.profile.deleted": (message) => {
      const profile = message.payload.profile as Record<string, unknown>;
      const profileId = profile.id as string;
      const agentId = getAgentId(profile);
      store.setState((state) => ({
        ...state,
        agentProfiles: {
          ...state.agentProfiles,
          items: state.agentProfiles.items.filter((p) => p.id !== profileId),
        },
        settingsAgents: {
          items: state.settingsAgents.items.map((item) =>
            item.id === agentId
              ? {
                  ...item,
                  profiles: item.profiles.filter((p) => p.id !== profileId),
                }
              : item,
          ),
        },
      }));
    },
  };
}
