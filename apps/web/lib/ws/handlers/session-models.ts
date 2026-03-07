import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionModelsPayload } from "@/lib/types/backend";

export function registerSessionModelsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.models_updated": (message) => {
      const payload = message.payload as SessionModelsPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      store.getState().setSessionModels(payload.session_id, {
        currentModelId: payload.current_model_id,
        models: (payload.models ?? []).map((m) => ({
          modelId: m.model_id,
          name: m.name,
          description: m.description,
          usageMultiplier: m.usage_multiplier,
          meta: m.meta,
        })),
        configOptions: (payload.config_options ?? []).map((o) => ({
          type: o.type,
          id: o.id,
          name: o.name,
          currentValue: o.current_value,
          category: o.category,
          options: o.options,
        })),
      });
    },
  };
}
