import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerDiffsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "diff.update": (message) => {
      store.setState((state) => ({
        ...state,
        diffs: {
          files: message.payload.files,
        },
      }));
    },
  };
}
