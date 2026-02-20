import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SecretListItem } from "@/lib/types/http-secrets";

export function registerSecretsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "secrets.created": (message) => {
      const item = message.payload as SecretListItem;
      store.getState().addSecret(item);
    },
    "secrets.updated": (message) => {
      const item = message.payload as SecretListItem;
      store.getState().updateSecret(item);
    },
    "secrets.deleted": (message) => {
      const { id } = message.payload as { id: string };
      store.getState().removeSecret(id);
    },
  };
}
