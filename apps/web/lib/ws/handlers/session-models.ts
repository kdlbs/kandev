import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { SessionModelsPayload } from "@/lib/types/backend";
import type {
  ConfigOptionEntry,
  SessionModelsState,
} from "@/lib/state/slices/session-runtime/types";
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

function isModelConfigOption(option: Pick<SessionModelConfigOption, "id" | "category">): boolean {
  return option.id === "model" || option.category === "model";
}

function isUnsettledStartupModelsPayload(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
): boolean {
  return (
    state.taskSessions.items[sessionId]?.state === "STARTING" &&
    payload.config_options_settled !== true
  );
}

function shouldHydrateSessionModelsPayload(
  payload: SessionModelsPayload,
  matchesPersisted: boolean,
  unsettledStartup: boolean,
): boolean {
  // Two-layer defense: the backend gates unsettled startup events, while this
  // barrier protects reconnects where the client session state lags.
  return payload.config_options_settled === true || (matchesPersisted && !unsettledStartup);
}

function shouldUsePersistedRuntimeConfig(hydrated: boolean, unsettledStartup: boolean): boolean {
  // A store can outlive the backend session after a restart. Keep persisted
  // runtime values authoritative until the resumed startup payload settles.
  return !hydrated || unsettledStartup;
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

// resolveConfigOptions keeps the previously settled config options when the
// incoming update is a preserved echo, otherwise rebuilds from the payload.
function resolveConfigOptions(
  preserve: boolean,
  existing: ConfigOptionEntry[] | undefined,
  payload: SessionModelsPayload,
  pendingRuntime: PersistedRuntimeConfig,
  currentModelId: string,
): ConfigOptionEntry[] {
  if (preserve && existing) {
    return existing.map((option) =>
      isModelConfigOption(option) && currentModelId
        ? { ...option, currentValue: currentModelId }
        : option,
    );
  }
  return (payload.config_options ?? []).map((o) => ({
    type: o.type,
    id: o.id,
    name: o.name,
    description: o.description,
    currentValue: isModelConfigOption(o)
      ? currentModelId || pendingRuntime.configOptions?.[o.id] || o.current_value
      : (pendingRuntime.configOptions?.[o.id] ?? o.current_value),
    category: o.category,
    options: o.options,
  }));
}

function clearStaleContextWindow(state: AppState, sessionId: string, currentModelId: string) {
  const previousModelId = state.sessionModels.bySessionId[sessionId]?.currentModelId ?? "";
  if (previousModelId && currentModelId && previousModelId !== currentModelId) {
    state.clearContextWindow(sessionId);
  }
}

function resolvedConfigOptionsSettled(
  payload: SessionModelsPayload,
  existing: SessionModelsState["bySessionId"][string] | undefined,
): boolean | undefined {
  return payload.config_options_settled ?? existing?.configOptionsSettled;
}

// resolveModelsUpdatedState computes the convergence target for a
// models_updated event: which model becomes current, whether the update is
// an empty relaunch echo, and whether the previously settled config options
// must be preserved. The fallback note makes the payload's reported model
// win over the persisted runtime model (which can still name the gone start
// model) so the picker converges on the fallback as the live model instead
// of re-selecting the unavailable configured model.
function resolveModelsUpdatedState(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
  pendingRuntime: PersistedRuntimeConfig,
): {
  currentModelId: string;
  isEmpty: boolean;
  populated: boolean;
  preserveConfigOptions: boolean;
  existingEntry: SessionModelsState["bySessionId"][string] | undefined;
  existingFallback: string | undefined;
} {
  const payloadCurrentModelId = resolveCurrentModelId(payload);
  const existingEntry = state.sessionModels.bySessionId[sessionId];
  const existingFallback = existingEntry?.fallbackModel;
  return {
    currentModelId:
      existingFallback && payloadCurrentModelId
        ? payloadCurrentModelId
        : pendingRuntime.model || payloadCurrentModelId,
    isEmpty: isEmptyModelsUpdate(
      payloadCurrentModelId,
      payload.models ?? [],
      payload.config_options,
    ),
    populated: hasPopulatedModels(state, sessionId),
    preserveConfigOptions: shouldPreserveExistingConfigOptions(state, sessionId, payload),
    existingEntry,
    existingFallback,
  };
}

export function registerSessionModelsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.model_fallback": (message) => {
      const payload = message.payload as
        | { session_id?: string; fallback_model?: string }
        | undefined;
      if (!payload?.session_id || !payload.fallback_model) {
        return;
      }
      const state = store.getState();
      const sessionId = payload.session_id;
      const existing = state.sessionModels.bySessionId[sessionId];
      // Merge the explicit "using fallback model" signal into the session's
      // model entry so the picker can show why the start model was replaced.
      // The fallback model also becomes the current model: the persisted
      // runtime model may still name the unavailable start model, and the
      // picker must not keep showing that as the live model.
      store.getState().setSessionModels(sessionId, {
        currentModelId: payload.fallback_model,
        models: existing?.models ?? [],
        configOptions: existing?.configOptions ?? [],
        ...(existing?.configOptionsSettled === undefined
          ? {}
          : { configOptionsSettled: existing.configOptionsSettled }),
        configBaseline: existing?.configBaseline,
        fallbackModel: payload.fallback_model,
      });
    },

    "session.models_updated": (message) => {
      const payload = message.payload as SessionModelsPayload | undefined;
      if (!payload?.session_id) {
        return;
      }
      const acpModels = payload.models ?? [];
      const sessionId = payload.session_id;
      const state = store.getState();
      const hydrated = hydratedSessions(store);
      const unsettledStartup = isUnsettledStartupModelsPayload(state, sessionId, payload);
      const usePersistedRuntime = shouldUsePersistedRuntimeConfig(
        hydrated.has(sessionId),
        unsettledStartup,
      );
      const persisted = usePersistedRuntime ? persistedRuntimeConfig(state, sessionId) : {};
      const matchesPersisted = payloadMatchesPersistedRuntime(payload, persisted);
      if (shouldHydrateSessionModelsPayload(payload, matchesPersisted, unsettledStartup)) {
        hydrated.add(sessionId);
      }
      const pendingRuntime = shouldUsePersistedRuntimeConfig(
        hydrated.has(sessionId),
        unsettledStartup,
      )
        ? persisted
        : {};
      const resolved = resolveModelsUpdatedState(state, sessionId, payload, pendingRuntime);
      debugModelsUpdate(state, sessionId, payload, resolved);
      if (resolved.isEmpty && resolved.populated) {
        return;
      }
      clearStaleContextWindow(state, sessionId, resolved.currentModelId);

      const configOptions = resolveConfigOptions(
        resolved.preserveConfigOptions,
        resolved.existingEntry?.configOptions,
        payload,
        pendingRuntime,
        resolved.currentModelId,
      );
      const configOptionsSettled = resolvedConfigOptionsSettled(payload, resolved.existingEntry);

      state.setSessionModels(sessionId, {
        currentModelId: resolved.currentModelId,
        fallbackModel: resolved.existingFallback,
        models: acpModels.map((m) => ({
          modelId: m.model_id,
          name: m.name,
          description: m.description,
          usageMultiplier: m.usage_multiplier,
          meta: m.meta,
        })),
        configOptions,
        ...(configOptionsSettled === undefined ? {} : { configOptionsSettled }),
        configBaseline:
          payload.config_baseline ??
          persisted.baseline ??
          state.sessionModels.bySessionId[sessionId]?.configBaseline,
      });
    },
  };
}
