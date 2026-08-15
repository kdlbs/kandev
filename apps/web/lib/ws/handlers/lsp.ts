import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "./types";

export function registerLspHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.lsp.changed": (message) => {
      store.getState().mergeTaskLspLanguage(message.payload);
    },
  };
}
