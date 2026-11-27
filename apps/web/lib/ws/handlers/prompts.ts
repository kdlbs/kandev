import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { refreshPrompts } from "@/lib/state/prompts-loader";

export function registerPromptsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "prompts.changed": () => {
      void refreshPrompts(store);
    },
  };
}
