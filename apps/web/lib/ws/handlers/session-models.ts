import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionModelsPayload } from "@/lib/types/backend";

type SessionModelConfigOption = SessionModelsPayload["config_options"][number];

function resolveCurrentModelId(payload: SessionModelsPayload): string {
  if (payload.current_model_id) {
    return payload.current_model_id;
  }
  const modelOpt = (payload.config_options ?? []).find(isModelConfigOption);
  return modelOpt?.current_value ?? "";
}

function isModelConfigOption(option: SessionModelConfigOption): boolean {
  return option.id === "model" || option.category === "model";
}

function clearStaleContextWindow(state: AppState, sessionId: string, currentModelId: string) {
  const previousModelId = state.sessionModels.bySessionId[sessionId]?.currentModelId ?? "";
  if (previousModelId && currentModelId && previousModelId !== currentModelId) {
    state.clearContextWindow(sessionId);
  }
}

function clearStaleActiveModel(
  state: AppState,
  sessionId: string,
  acpModels: SessionModelsPayload["models"],
) {
  if (!acpModels?.length) {
    return;
  }
  const currentActive = state.activeModel.bySessionId[sessionId];
  if (currentActive && !acpModels.some((m) => m.model_id === currentActive)) {
    state.setActiveModel(sessionId, "");
  }
}

export function registerSessionModelsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.models_updated": (message) => {
      const payload = message.payload as SessionModelsPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      const acpModels = payload.models ?? [];
      const sessionId = payload.session_id;
      const currentModelId = resolveCurrentModelId(payload);
      const state = store.getState();
      clearStaleContextWindow(state, sessionId, currentModelId);

      state.setSessionModels(sessionId, {
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

      clearStaleActiveModel(state, sessionId, acpModels);
    },
  };
}
