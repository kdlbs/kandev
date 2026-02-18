import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import { toAgentProfileOption } from '@/lib/state/slices/settings/types';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildProfileEntry(profile: any) {
  return {
    id: profile.id,
    agent_id: profile.agent_id,
    name: profile.name,
    agent_display_name: profile.agent_display_name,
    model: profile.model,
    auto_approve: profile.auto_approve,
    dangerously_skip_permissions: profile.dangerously_skip_permissions,
    allow_indexing: profile.allow_indexing,
    cli_passthrough: profile.cli_passthrough ?? false,
    plan: profile.plan,
    created_at: profile.created_at ?? '',
    updated_at: profile.updated_at ?? '',
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handleProfileCreated(state: AppState, profile: any): Partial<AppState> {
  const agent = state.settingsAgents.items.find((a) => a.id === profile.agent_id);
  const agentStub = { id: profile.agent_id, name: agent?.name ?? '' };
  const nextProfiles = [
    ...state.agentProfiles.items.filter((p) => p.id !== profile.id),
    toAgentProfileOption(agentStub, profile),
  ];
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === profile.agent_id
      ? { ...item, profiles: [...item.profiles.filter((p) => p.id !== profile.id), buildProfileEntry(profile)] }
      : item
  );
  return { agentProfiles: { ...state.agentProfiles, items: nextProfiles }, settingsAgents: { items: nextAgents } };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handleProfileUpdated(state: AppState, profile: any): Partial<AppState> {
  const agent = state.settingsAgents.items.find((a) => a.id === profile.agent_id);
  const agentStub = { id: profile.agent_id, name: agent?.name ?? '' };
  const nextProfiles = state.agentProfiles.items.map((p) =>
    p.id === profile.id ? toAgentProfileOption(agentStub, profile) : p
  );
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === profile.agent_id
      ? {
          ...item,
          profiles: item.profiles.map((p) =>
            p.id === profile.id
              ? { ...p, name: profile.name, agent_display_name: profile.agent_display_name, model: profile.model, auto_approve: profile.auto_approve, dangerously_skip_permissions: profile.dangerously_skip_permissions, allow_indexing: profile.allow_indexing, plan: profile.plan, updated_at: profile.updated_at ?? p.updated_at }
              : p
          ),
        }
      : item
  );
  return { agentProfiles: { ...state.agentProfiles, items: nextProfiles }, settingsAgents: { items: nextAgents } };
}

export function registerAgentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'agent.available.updated': (message) => {
      store.setState((state) => ({
        ...state,
        availableAgents: { items: message.payload.agents ?? [], loaded: true, loading: false },
      }));
    },
    'agent.updated': (message) => {
      store.setState((state) => ({
        ...state,
        agents: {
          agents: state.agents.agents.some((a) => a.id === message.payload.agentId)
            ? state.agents.agents.map((a) => a.id === message.payload.agentId ? { ...a, status: message.payload.status } : a)
            : [...state.agents.agents, { id: message.payload.agentId, status: message.payload.status }],
        },
      }));
    },
    'agent.profile.created': (message) => {
      store.setState((state) => ({ ...state, ...handleProfileCreated(state, message.payload.profile) }));
    },
    'agent.profile.updated': (message) => {
      store.setState((state) => ({ ...state, ...handleProfileUpdated(state, message.payload.profile) }));
    },
    'agent.profile.deleted': (message) => {
      store.setState((state) => ({
        ...state,
        agentProfiles: { ...state.agentProfiles, items: state.agentProfiles.items.filter((p) => p.id !== message.payload.profile.id) },
        settingsAgents: {
          items: state.settingsAgents.items.map((item) =>
            item.id === message.payload.profile.agent_id
              ? { ...item, profiles: item.profiles.filter((p) => p.id !== message.payload.profile.id) }
              : item
          ),
        },
      }));
    },
  };
}
