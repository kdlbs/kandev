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
      const acpModels = payload.models ?? [];
      // Resolve currentModelId: prefer the explicit field, fall back to the "model"
      // config option's currentValue (some ACP agents send currentModelId as empty).
      let currentModelId = payload.current_model_id || "";
      if (!currentModelId) {
        const modelOpt = (payload.config_options ?? []).find(
          (o) => o.id === "model" || o.category === "model",
        );
        if (modelOpt?.current_value) {
          currentModelId = modelOpt.current_value;
        }
      }
      console.debug("[session-models] received session.models_updated", {
        sessionId: payload.session_id,
        rawCurrentModelId: payload.current_model_id,
        resolvedCurrentModelId: currentModelId,
        models: acpModels.map((m) => m.model_id),
      });
      store.getState().setSessionModels(payload.session_id, {
        currentModelId,
        models: acpModels.map((m) => ({
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

      // Clear stale activeModel if it uses a profile ID that doesn't exist in ACP models.
      // This happens when a user selected a static model before ACP models arrived.
      if (acpModels.length > 0) {
        const state = store.getState();
        const currentActive = state.activeModel.bySessionId[payload.session_id];
        if (currentActive && !acpModels.some((m) => m.model_id === currentActive)) {
          console.debug("[session-models] clearing stale activeModel", {
            sessionId: payload.session_id,
            staleModel: currentActive,
          });
          state.setActiveModel(payload.session_id, "");
        }
      }
    },
  };
}
