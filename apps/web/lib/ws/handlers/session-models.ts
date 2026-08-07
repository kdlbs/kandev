import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionModelsPayload } from "@/lib/types/backend";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debug = createDebugLogger("model-selector:ws");

type SessionModelConfigOption = SessionModelsPayload["config_options"][number];

type PersistedRuntimeConfig = {
  model?: string;
  configOptions?: Record<string, string>;
  baseline?: Record<string, string>;
};

const hydratedRuntimeByStore = new WeakMap<StoreApi<AppState>, Set<string>>();

function hydratedSessions(store: StoreApi<AppState>): Set<string> {
  let sessions = hydratedRuntimeByStore.get(store);
  if (!sessions) {
    sessions = new Set();
    hydratedRuntimeByStore.set(store, sessions);
  }
  return sessions;
}

function stringRecord(value: unknown): Record<string, string> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] => typeof entry[1] === "string",
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function persistedRuntimeConfig(state: AppState, sessionId: string): PersistedRuntimeConfig {
  const metadata = state.taskSessions.items[sessionId]?.metadata;
  if (!metadata) return {};
  const runtime =
    metadata.runtime_config && typeof metadata.runtime_config === "object"
      ? (metadata.runtime_config as Record<string, unknown>)
      : undefined;
  const overrides =
    metadata.runtime_config_overrides && typeof metadata.runtime_config_overrides === "object"
      ? (metadata.runtime_config_overrides as Record<string, unknown>)
      : undefined;
  const runtimeOptions = stringRecord(runtime?.config_options);
  const overrideOptions = stringRecord(overrides?.config_options);
  return {
    model: stringValue(overrides?.model) ?? stringValue(runtime?.model),
    configOptions:
      runtimeOptions || overrideOptions ? { ...runtimeOptions, ...overrideOptions } : undefined,
    baseline: stringRecord(metadata.acp_config_baseline),
  };
}

function payloadMatchesPersistedRuntime(
  payload: SessionModelsPayload,
  persisted: PersistedRuntimeConfig,
): boolean {
  if (persisted.model && resolveCurrentModelId(payload) !== persisted.model) return false;
  const values = persisted.configOptions;
  if (!values) return true;
  const payloadValues = new Map(
    (payload.config_options ?? []).map((option) => [option.id, option.current_value]),
  );
  return Object.entries(values).every(([id, value]) => payloadValues.get(id) === value);
}

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

// During an agentctl relaunch (ready -> starting -> ready) the backend can emit
// a transient session.models_updated with no models, no current model, and no
// config options before the fresh agent reports its real capabilities. Since
// setSessionModels is a blind replace, that empty event would wipe the populated
// entry and unmount the model selector until a good event arrives. Skip the
// overwrite when the incoming payload is fully empty but a populated entry
// already exists — the same stale-empty guard used by setSessionCommits.
function isEmptyModelsUpdate(
  currentModelId: string,
  acpModels: SessionModelsPayload["models"],
  configOptions: SessionModelsPayload["config_options"],
): boolean {
  return !currentModelId && !acpModels?.length && !configOptions?.length;
}

function hasPopulatedModels(state: AppState, sessionId: string): boolean {
  const existing = state.sessionModels.bySessionId[sessionId];
  return (
    !!existing &&
    (!!existing.currentModelId || existing.models.length > 0 || existing.configOptions.length > 0)
  );
}

// During an agentctl relaunch the backend can emit a transient
// session.models_updated that still carries models + a current model (so it is
// not "fully empty") but reports an empty config_options array before the fresh
// agent finishes reporting its dynamic config. Blindly writing that array wipes
// the previously-hydrated config options, which flips the model selector's
// configHydrated gate to false and unmounts it. Preserve the existing populated
// config options (and their baseline) when the payload drops them; a real config
// change always re-sends the non-empty array.
function shouldPreserveExistingConfigOptions(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
): boolean {
  if (payload.config_options?.length) return false;
  const existing = state.sessionModels.bySessionId[sessionId];
  return !!existing && existing.configOptions.length > 0;
}

function debugModelsUpdate(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
  resolved: {
    currentModelId: string;
    isEmpty: boolean;
    populated: boolean;
    preserveConfigOptions: boolean;
  },
) {
  if (!isDebug()) return;
  const existing = state.sessionModels.bySessionId[sessionId];
  debug("models_updated", {
    sessionId,
    payloadCurrentModelId: payload.current_model_id ?? "",
    resolvedCurrentModelId: resolved.currentModelId,
    payloadModelsLen: payload.models?.length ?? 0,
    payloadConfigOptionIds: (payload.config_options ?? []).map((o) => o.id),
    isEmpty: resolved.isEmpty,
    existingPopulated: resolved.populated,
    existingCurrentModelId: existing?.currentModelId ?? "",
    existingModelsLen: existing?.models.length ?? 0,
    existingConfigOptionIds: (existing?.configOptions ?? []).map((o) => o.id),
    willSkip: resolved.isEmpty && resolved.populated,
    preserveConfigOptions: resolved.preserveConfigOptions,
  });
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
      const state = store.getState();
      const hydrated = hydratedSessions(store);
      const persisted = hydrated.has(sessionId) ? {} : persistedRuntimeConfig(state, sessionId);
      const matchesPersisted = payloadMatchesPersistedRuntime(payload, persisted);
      if (matchesPersisted) hydrated.add(sessionId);
      const pendingRuntime = matchesPersisted ? {} : persisted;
      const payloadCurrentModelId = resolveCurrentModelId(payload);
      const currentModelId = pendingRuntime.model || payloadCurrentModelId;
      // Classify emptiness from the incoming payload alone; persisted metadata
      // must not make a genuinely empty relaunch event look populated.
      const isEmpty = isEmptyModelsUpdate(payloadCurrentModelId, acpModels, payload.config_options);
      const populated = hasPopulatedModels(state, sessionId);
      const preserveConfigOptions = shouldPreserveExistingConfigOptions(state, sessionId, payload);
      debugModelsUpdate(state, sessionId, payload, {
        currentModelId,
        isEmpty,
        populated,
        preserveConfigOptions,
      });
      if (isEmpty && populated) {
        return;
      }
      clearStaleContextWindow(state, sessionId, currentModelId);

      const existingEntry = state.sessionModels.bySessionId[sessionId];
      const configOptions = preserveConfigOptions
        ? existingEntry.configOptions
        : (payload.config_options ?? []).map((o) => ({
            type: o.type,
            id: o.id,
            name: o.name,
            description: o.description,
            currentValue: pendingRuntime.configOptions?.[o.id] ?? o.current_value,
            category: o.category,
            options: o.options,
          }));

      state.setSessionModels(sessionId, {
        currentModelId,
        models: acpModels.map((m) => ({
          modelId: m.model_id,
          name: m.name,
          description: m.description,
          usageMultiplier: m.usage_multiplier,
          meta: m.meta,
        })),
        configOptions,
        configBaseline:
          payload.config_baseline ??
          persisted.baseline ??
          state.sessionModels.bySessionId[sessionId]?.configBaseline,
      });

      clearStaleActiveModel(state, sessionId, acpModels);
    },
  };
}
