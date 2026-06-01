import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/**
 * Session-runtime agent status handler. The settings-domain agent handlers
 * (available agents, install jobs, agent profiles) moved to the TanStack Query
 * bridge (`lib/query/bridge/settings.ts`); only `agent.updated` — which writes
 * the session-runtime `agents` slice — remains here.
 */
export function registerAgentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
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
  };
}
