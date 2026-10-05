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

type ResolvedModelsUpdate = {
  currentModelId: string;
  isEmpty: boolean;
  populated: boolean;
  preserveConfigOptions: boolean;
  preserveModels: boolean;
  existingEntry: SessionModelsState["bySessionId"][string] | undefined;
  existingFallback: string | undefined;
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

// activeModel only ever records a selection the user made in this store, so a
// payload reporting that model is the provider confirming that selection rather
// than stale resume state. The cached session metadata still names the
// pre-switch model until a session update lands, so it cannot arbitrate here.
function payloadConfirmsUserSelection(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
): boolean {
  const selected = state.activeModel?.bySessionId?.[sessionId];
  return !!selected && resolveCurrentModelId(payload) === selected;
}

function shouldHydrateSessionModelsPayload(
  payload: SessionModelsPayload,
  matchesPersisted: boolean,
  confirmsUserSelection: boolean,
  unsettledStartup: boolean,
): boolean {
  // Two-layer defense: the backend gates unsettled startup events, while this
  // barrier protects reconnects where the client session state lags.
  return (
    payload.config_options_settled === true ||
    ((matchesPersisted || confirmsUserSelection) && !unsettledStartup)
  );
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

function shouldPreserveExistingModels(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
): boolean {
  if (payload.models?.length || payload.config_options_settled === true) return false;
  const existing = state.sessionModels.bySessionId[sessionId];
  return !!existing && existing.models.length > 0;
}

function shouldSkipModelsUpdate(resolved: { isEmpty: boolean; populated: boolean }): boolean {
  return resolved.isEmpty && resolved.populated;
}

function resolveModels(
  preserve: boolean,
  existing: SessionModelsState["bySessionId"][string]["models"] | undefined,
  acpModels: SessionModelsPayload["models"],
): SessionModelsState["bySessionId"][string]["models"] {
  if (preserve && existing) return existing;
  return (acpModels ?? []).map((model) => ({
    modelId: model.model_id,
    name: model.name,
    description: model.description,
    usageMultiplier: model.usage_multiplier,
    meta: model.meta,
  }));
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
    preserveModels: boolean;
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
    willSkip: shouldSkipModelsUpdate(resolved),
    preserveConfigOptions: resolved.preserveConfigOptions,
    preserveModels: resolved.preserveModels,
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
  return (payload.config_options ?? []).map((option) => ({
    type: option.type,
    id: option.id,
    name: option.name,
    description: option.description,
    currentValue: resolvedConfigOptionValue(option, pendingRuntime, currentModelId),
    category: option.category,
    options: option.options,
  }));
}

function resolvedConfigOptionValue(
  option: SessionModelConfigOption,
  pendingRuntime: PersistedRuntimeConfig,
  currentModelId: string,
): string {
  if (!isModelConfigOption(option)) {
    return pendingRuntime.configOptions?.[option.id] ?? option.current_value;
  }
  return currentModelId || pendingRuntime.configOptions?.[option.id] || option.current_value;
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

function providerConfigOptionValues(
  options: SessionModelsPayload["config_options"],
): Record<string, string> {
  return Object.fromEntries(
    (options ?? [])
      .filter((option) => !!option.id && typeof option.current_value === "string")
      .map((option) => [option.id, option.current_value]),
  );
}

type ConfirmedConfigProjection = Pick<
  SessionModelsState["bySessionId"][string],
  "confirmedConfigOptions" | "confirmedConfigOptionsExecutionId" | "pendingConfirmedConfigOptions"
>;

function confirmedConfigProjection(
  values?: Record<string, string>,
  executionId?: string,
  pending?: ConfirmedConfigProjection["pendingConfirmedConfigOptions"],
): ConfirmedConfigProjection {
  return {
    confirmedConfigOptions: values,
    confirmedConfigOptionsExecutionId: executionId,
    pendingConfirmedConfigOptions: pending,
  };
}

function existingConfirmedConfigProjection(
  existing: SessionModelsState["bySessionId"][string] | undefined,
): ConfirmedConfigProjection {
  return confirmedConfigProjection(
    existing?.confirmedConfigOptions,
    existing?.confirmedConfigOptionsExecutionId,
    existing?.pendingConfirmedConfigOptions,
  );
}

function pendingConfirmedConfigForPayload(
  existing: SessionModelsState["bySessionId"][string] | undefined,
  payloadExecutionId: string | undefined,
): ConfirmedConfigProjection["pendingConfirmedConfigOptions"] {
  const pending = existing?.pendingConfirmedConfigOptions;
  if (!pending || (payloadExecutionId && pending.executionId !== payloadExecutionId)) {
    return undefined;
  }
  return pending;
}

function staleExecutionConfigProjection(
  payloadExecutionId: string | undefined,
  currentExecutionId: string | undefined,
  existing: SessionModelsState["bySessionId"][string] | undefined,
  pending: ConfirmedConfigProjection["pendingConfirmedConfigOptions"],
): ConfirmedConfigProjection | undefined {
  if (!payloadExecutionId || !currentExecutionId || payloadExecutionId === currentExecutionId) {
    return undefined;
  }
  if (existing?.confirmedConfigOptionsExecutionId === currentExecutionId) {
    return existingConfirmedConfigProjection(existing);
  }
  return confirmedConfigProjection(undefined, undefined, pending);
}

function isStartupUnsettledConfigUpdate(
  payload: SessionModelsPayload,
  startupUnsettled: boolean,
  agentctlStatus: string | undefined,
): boolean {
  return (
    payload.config_options_settled === false ||
    startupUnsettled ||
    (agentctlStatus === "starting" && payload.config_options_settled !== true)
  );
}

function isAuthoritativeConfigUpdate(payload: SessionModelsPayload): boolean {
  return (
    payload.config_options_source === "provider_update" ||
    payload.config_options_source === "provider_response"
  );
}

function providerUpdateMatchesConfirmedExecution(
  providerExecutionId: string | undefined,
  agentctlExecutionId: string | undefined,
  agentctlStatus: string | undefined,
  confirmedExecutionId: string | undefined,
): boolean {
  if (!providerExecutionId || agentctlStatus === "starting") return false;
  return (
    providerExecutionId === confirmedExecutionId || providerExecutionId === agentctlExecutionId
  );
}

function confirmedConfigOptionsForUpdate(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
  startupUnsettled: boolean,
  existing: SessionModelsState["bySessionId"][string] | undefined,
): ConfirmedConfigProjection {
  const agentctl = state.sessionAgentctl?.itemsBySessionId?.[sessionId];
  const payloadExecutionId = payload.agent_execution_id;
  const pending = pendingConfirmedConfigForPayload(existing, payloadExecutionId);
  const stale = staleExecutionConfigProjection(
    payloadExecutionId,
    agentctl?.agentExecutionId,
    existing,
    pending,
  );
  if (stale) return stale;
  if (isStartupUnsettledConfigUpdate(payload, startupUnsettled, agentctl?.status)) {
    return confirmedConfigProjection(undefined, undefined, pending);
  }

  const executionId = payloadExecutionId ?? agentctl?.agentExecutionId;
  if (payload.config_options_settled === true) {
    return confirmedConfigProjection(
      providerConfigOptionValues(payload.config_options),
      executionId,
    );
  }
  if (isAuthoritativeConfigUpdate(payload)) {
    if (
      providerUpdateMatchesConfirmedExecution(
        executionId,
        agentctl?.agentExecutionId,
        agentctl?.status,
        existing?.confirmedConfigOptionsExecutionId,
      )
    ) {
      return confirmedConfigProjection(
        providerConfigOptionValues(payload.config_options),
        executionId,
      );
    }
    return confirmedConfigProjection();
  }

  return existingConfirmedConfigProjection(existing);
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
): ResolvedModelsUpdate {
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
    preserveModels: shouldPreserveExistingModels(state, sessionId, payload),
    existingEntry,
    existingFallback,
  };
}

function resolveModelsUpdateRuntime(
  store: StoreApi<AppState>,
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
) {
  const providerRestored = payload.session_settings_policy === "provider_restored";
  const strictProjection = payload.session_settings_policy === "strict";
  const hydrated = hydratedSessions(store);
  const unsettledStartup = isUnsettledStartupModelsPayload(state, sessionId, payload);
  const persisted = shouldUsePersistedRuntimeConfig(hydrated.has(sessionId), unsettledStartup)
    ? persistedRuntimeConfig(state, sessionId)
    : {};
  const matchesPersisted = payloadMatchesPersistedRuntime(payload, persisted);
  const confirmsUserSelection = payloadConfirmsUserSelection(state, sessionId, payload);
  if (
    providerRestored ||
    shouldHydrateSessionModelsPayload(
      payload,
      matchesPersisted,
      confirmsUserSelection,
      unsettledStartup,
    )
  ) {
    hydrated.add(sessionId);
  }
  const pendingRuntime = resolvePendingRuntime(
    providerRestored,
    hydrated,
    sessionId,
    unsettledStartup,
    persisted,
  );
  const resolved = resolveModelsUpdatedState(state, sessionId, payload, pendingRuntime);
  if (providerRestored) {
    resolved.currentModelId = resolveCurrentModelId(payload);
    resolved.preserveConfigOptions = false;
    resolved.preserveModels = false;
  }
  return { providerRestored, strictProjection, persisted, pendingRuntime, resolved };
}

function resolvePendingRuntime(
  providerRestored: boolean,
  hydrated: Set<string>,
  sessionId: string,
  unsettledStartup: boolean,
  persisted: PersistedRuntimeConfig,
): PersistedRuntimeConfig {
  if (providerRestored) return {};
  return shouldUsePersistedRuntimeConfig(hydrated.has(sessionId), unsettledStartup)
    ? persisted
    : {};
}

function settingsPolicyForProjection(
  providerRestored: boolean,
  strictProjection: boolean,
  existingPolicy: SessionModelsState["bySessionId"][string]["settingsPolicy"],
): SessionModelsState["bySessionId"][string]["settingsPolicy"] {
  if (providerRestored) return "provider_restored";
  if (strictProjection) return undefined;
  return existingPolicy;
}

function handleModelFallback(store: StoreApi<AppState>, payload: unknown): void {
  const fallbackPayload = payload as { session_id?: string; fallback_model?: string } | undefined;
  if (!fallbackPayload?.session_id || !fallbackPayload.fallback_model) return;
  const state = store.getState();
  const existing = state.sessionModels.bySessionId[fallbackPayload.session_id];
  state.setSessionModels(fallbackPayload.session_id, {
    currentModelId: fallbackPayload.fallback_model,
    models: existing?.models ?? [],
    configOptions: existing?.configOptions ?? [],
    confirmedConfigOptions: existing?.confirmedConfigOptions,
    confirmedConfigOptionsExecutionId: existing?.confirmedConfigOptionsExecutionId,
    pendingConfirmedConfigOptions: existing?.pendingConfirmedConfigOptions,
    ...(existing?.configOptionsSettled === undefined
      ? {}
      : { configOptionsSettled: existing.configOptionsSettled }),
    configBaseline: existing?.configBaseline,
    fallbackModel: fallbackPayload.fallback_model,
  });
}

function updateSkippedModelsEntry(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
  startupUnsettled: boolean,
  resolved: ReturnType<typeof resolveModelsUpdateRuntime>["resolved"],
) {
  const existing = resolved.existingEntry;
  const providerUpdate =
    payload.config_options_source === "provider_update" ||
    payload.config_options_source === "provider_response";
  if (
    !existing ||
    (payload.config_options_settled === undefined && !startupUnsettled && !providerUpdate)
  ) {
    return;
  }
  state.setSessionModels(sessionId, {
    ...existing,
    ...confirmedConfigOptionsForUpdate(state, sessionId, payload, startupUnsettled, existing),
  });
}

function applyResolvedModelsUpdate(
  state: AppState,
  sessionId: string,
  payload: SessionModelsPayload,
  startupUnsettled: boolean,
  runtime: ReturnType<typeof resolveModelsUpdateRuntime>,
) {
  const { providerRestored, strictProjection, persisted, pendingRuntime, resolved } = runtime;
  clearStaleContextWindow(state, sessionId, resolved.currentModelId);
  const configOptions = resolveConfigOptions(
    resolved.preserveConfigOptions,
    resolved.existingEntry?.configOptions,
    payload,
    pendingRuntime,
    resolved.currentModelId,
  );
  const configOptionsSettled = providerRestored
    ? payload.config_options_settled === true
    : resolvedConfigOptionsSettled(payload, resolved.existingEntry);
  const confirmedConfigOptions = confirmedConfigOptionsForUpdate(
    state,
    sessionId,
    payload,
    startupUnsettled,
    resolved.existingEntry,
  );
  state.setSessionModels(sessionId, {
    currentModelId: resolved.currentModelId,
    fallbackModel: resolved.existingFallback,
    models: resolveModels(
      resolved.preserveModels,
      resolved.existingEntry?.models,
      payload.models ?? [],
    ),
    configOptions,
    ...confirmedConfigOptions,
    settingsPolicy: settingsPolicyForProjection(
      providerRestored,
      strictProjection,
      resolved.existingEntry?.settingsPolicy,
    ),
    ...(configOptionsSettled === undefined ? {} : { configOptionsSettled }),
    configBaseline:
      payload.config_baseline ??
      persisted.baseline ??
      state.sessionModels.bySessionId[sessionId]?.configBaseline,
  });
  if (providerRestored) state.clearActiveModel(sessionId);
}

function handleSessionModelsUpdated(
  store: StoreApi<AppState>,
  payload: SessionModelsPayload | undefined,
) {
  if (!payload?.session_id) return;
  const sessionId = payload.session_id;
  const state = store.getState();
  const runtime = resolveModelsUpdateRuntime(store, state, sessionId, payload);
  debugModelsUpdate(state, sessionId, payload, runtime.resolved);
  const startupUnsettled = isUnsettledStartupModelsPayload(state, sessionId, payload);
  if (shouldSkipModelsUpdate(runtime.resolved) && !runtime.providerRestored) {
    updateSkippedModelsEntry(state, sessionId, payload, startupUnsettled, runtime.resolved);
    return;
  }
  applyResolvedModelsUpdate(state, sessionId, payload, startupUnsettled, runtime);
}

export function registerSessionModelsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.model_fallback": (message) => handleModelFallback(store, message.payload),
    "session.models_updated": (message) =>
      handleSessionModelsUpdated(store, message.payload as SessionModelsPayload | undefined),
  };
}
