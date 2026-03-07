import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { AgentCapabilitiesPayload } from "@/lib/types/backend";

export function registerAgentCapabilitiesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.agent_capabilities": (message) => {
      const payload = message.payload as AgentCapabilitiesPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      store.getState().setAgentCapabilities(payload.session_id, {
        supportsImage: payload.supports_image,
        supportsAudio: payload.supports_audio,
        supportsEmbeddedContext: payload.supports_embedded_context,
        authMethods: (payload.auth_methods ?? []).map((m) => ({
          id: m.id,
          name: m.name,
          description: m.description,
          terminalAuth: m.terminal_auth
            ? {
                command: m.terminal_auth.command,
                args: m.terminal_auth.args,
                label: m.terminal_auth.label,
              }
            : undefined,
          meta: m.meta,
        })),
      });
    },
  };
}
