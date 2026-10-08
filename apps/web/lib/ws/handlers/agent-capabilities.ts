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
      syncPromptSuggestionSource(store, payload);
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

/** Mirrors the backend's prompt_suggestion_source metadata so the composer
 *  stops (or starts) using the utility fallback without a reload. */
function syncPromptSuggestionSource(store: StoreApi<AppState>, payload: AgentCapabilitiesPayload) {
  const session = store.getState().taskSessions.items[payload.session_id];
  if (!session) return;
  const current = session.metadata?.prompt_suggestion_source;
  const native = Boolean(payload.supports_prompt_suggestions);
  if (!native && current !== "native") return;
  const next = native ? "native" : "none";
  if (current === next) return;
  store.getState().setTaskSession({
    ...session,
    metadata: { ...(session.metadata ?? {}), prompt_suggestion_source: next },
  });
}
