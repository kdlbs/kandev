import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionPromptUsagePayload } from "@/lib/types/session-runtime-payloads";

export function registerPromptUsageHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.prompt_usage": (message) => {
      const payload = message.payload as SessionPromptUsagePayload | undefined;
      if (!payload?.session_id || !payload?.usage) {
        return;
      }
      store.getState().setPromptUsage(payload.session_id, {
        inputTokens: payload.usage.input_tokens,
        outputTokens: payload.usage.output_tokens,
        cachedReadTokens: payload.usage.cached_read_tokens,
        cachedWriteTokens: payload.usage.cached_write_tokens,
        totalTokens: payload.usage.total_tokens,
      });
    },
  };
}
