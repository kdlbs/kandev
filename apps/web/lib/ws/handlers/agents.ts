import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
export function registerAgentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'agent.available.updated': (message) => {
      store.setState((state) => {
        const availableAgents = message.payload.agents ?? [];
        return {
          ...state,
          availableAgents: { items: availableAgents, loaded: true, loading: false },
        };
      });
    },
    'agent.updated': (message) => {
      store.setState((state) => ({
        ...state,
        agents: {
          agents: state.agents.agents.some((agent) => agent.id === message.payload.agentId)
            ? state.agents.agents.map((agent) =>
                agent.id === message.payload.agentId
                  ? { ...agent, status: message.payload.status }
                  : agent
              )
            : [...state.agents.agents, { id: message.payload.agentId, status: message.payload.status }],
        },
      }));
    },
    'agent.profile.created': (message) => {
      store.setState((state) => {
        const label = `${message.payload.profile.agent_display_name} • ${message.payload.profile.name}`;
        const agentName = state.settingsAgents.items.find((a) => a.id === message.payload.profile.agent_id)?.name ?? '';
        const nextProfiles = [
          ...state.agentProfiles.items.filter((profile) => profile.id !== message.payload.profile.id),
          { id: message.payload.profile.id, label, agent_id: message.payload.profile.agent_id, agent_name: agentName },
        ];
        const nextAgents = state.settingsAgents.items.map((item) =>
          item.id === message.payload.profile.agent_id
            ? {
                ...item,
                profiles: [
                  ...item.profiles.filter((profile) => profile.id !== message.payload.profile.id),
                  {
                    id: message.payload.profile.id,
                    agent_id: message.payload.profile.agent_id,
                    name: message.payload.profile.name,
                    agent_display_name: message.payload.profile.agent_display_name,
                    model: message.payload.profile.model,
                    auto_approve: message.payload.profile.auto_approve,
                    dangerously_skip_permissions: message.payload.profile.dangerously_skip_permissions,
                    allow_indexing: message.payload.profile.allow_indexing,
                    cli_passthrough: message.payload.profile.cli_passthrough ?? false,
                    plan: message.payload.profile.plan,
                    created_at: message.payload.profile.created_at ?? '',
                    updated_at: message.payload.profile.updated_at ?? '',
                  },
                ],
              }
            : item
        );
        return {
          ...state,
          agentProfiles: { ...state.agentProfiles, items: nextProfiles },
          settingsAgents: { items: nextAgents },
        };
      });
    },
    'agent.profile.updated': (message) => {
      store.setState((state) => {
        const label = `${message.payload.profile.agent_display_name} • ${message.payload.profile.name}`;
        const agentName = state.settingsAgents.items.find((a) => a.id === message.payload.profile.agent_id)?.name ?? '';
        const nextProfiles = state.agentProfiles.items.map((profile) =>
          profile.id === message.payload.profile.id
            ? { ...profile, label, agent_id: message.payload.profile.agent_id, agent_name: agentName }
            : profile
        );
        const nextAgents = state.settingsAgents.items.map((item) =>
          item.id === message.payload.profile.agent_id
            ? {
                ...item,
                profiles: item.profiles.map((profile) =>
                  profile.id === message.payload.profile.id
                    ? {
                        ...profile,
                        name: message.payload.profile.name,
                        agent_display_name: message.payload.profile.agent_display_name,
                        model: message.payload.profile.model,
                        auto_approve: message.payload.profile.auto_approve,
                        dangerously_skip_permissions: message.payload.profile.dangerously_skip_permissions,
                        allow_indexing: message.payload.profile.allow_indexing,
                        plan: message.payload.profile.plan,
                        updated_at: message.payload.profile.updated_at ?? profile.updated_at,
                      }
                    : profile
                ),
              }
            : item
        );
        return {
          ...state,
          agentProfiles: { ...state.agentProfiles, items: nextProfiles },
          settingsAgents: { items: nextAgents },
        };
      });
    },
    'agent.profile.deleted': (message) => {
      store.setState((state) => {
        const nextProfiles = state.agentProfiles.items.filter(
          (profile) => profile.id !== message.payload.profile.id
        );
        const nextAgents = state.settingsAgents.items.map((item) =>
          item.id === message.payload.profile.agent_id
            ? {
                ...item,
                profiles: item.profiles.filter((profile) => profile.id !== message.payload.profile.id),
              }
            : item
        );
        return {
          ...state,
          agentProfiles: { ...state.agentProfiles, items: nextProfiles },
          settingsAgents: { items: nextAgents },
        };
      });
    },
  };
}
