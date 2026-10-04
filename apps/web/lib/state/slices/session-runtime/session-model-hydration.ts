import type { TaskSession } from "@/lib/types/http";
import type { ConfigOptionEntry, SessionModelEntry, SessionModelsState } from "./types";

/** Returns a record containing only string-valued properties. */
function stringMap(value: unknown): Record<string, string> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] => typeof entry[1] === "string",
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

/** Returns a non-array object as a record. */
function objectMap(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function objectList(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
    : [];
}

function sessionModelHydrationMetadata(session: TaskSession | null) {
  if (!session) return undefined;
  const snapshot = objectMap(session.metadata?.acp_model_state);
  if (!snapshot) return undefined;
  return {
    session,
    snapshot,
    runtime: objectMap(session.metadata?.runtime_config),
    overrides: objectMap(session.metadata?.runtime_config_overrides),
  };
}

function mapSessionModel(model: Record<string, unknown>): SessionModelEntry {
  return {
    modelId: stringValue(model.model_id) ?? "",
    name: stringValue(model.name) ?? "",
    description: stringValue(model.description),
    usageMultiplier: stringValue(model.usage_multiplier),
  };
}

function mapSessionConfigOption(
  option: Record<string, unknown>,
  runtimeOptions: Record<string, string>,
): ConfigOptionEntry {
  const id = stringValue(option.id) ?? "";
  return {
    type: stringValue(option.type) ?? "select",
    id,
    name: stringValue(option.name) ?? "",
    description: stringValue(option.description),
    currentValue: runtimeOptions[id] ?? stringValue(option.current_value) ?? "",
    category: stringValue(option.category),
    options: Array.isArray(option.options)
      ? (option.options as { value: string; name: string; description?: string }[])
      : undefined,
  };
}

/**
 * Reconstructs the model selector state persisted with a session row.
 *
 * ACP model events are normally delivered over the session websocket. A
 * resumed session can be opened before that event is replayed, so the REST
 * session snapshot is the authoritative bootstrap fallback.
 */
export function sessionModelsEntryFromTaskSession(
  session: TaskSession | null,
): SessionModelsState["bySessionId"][string] | undefined {
  const metadata = sessionModelHydrationMetadata(session);
  if (!metadata) return undefined;
  const { snapshot, runtime, overrides } = metadata;
  const runtimeOptions = {
    ...stringMap(runtime?.config_options),
    ...stringMap(overrides?.config_options),
  };
  const currentModelId =
    stringValue(overrides?.model) ??
    stringValue(runtime?.model) ??
    stringValue(snapshot.current_model_id) ??
    "";
  const models = objectList(snapshot.models)
    .map(mapSessionModel)
    .filter((model) => !!model.modelId);
  const configOptions = objectList(snapshot.config_options)
    .map((option) => mapSessionConfigOption(option, runtimeOptions))
    .filter((option) => !!option.id);

  if (!currentModelId && models.length === 0 && configOptions.length === 0) return undefined;

  return {
    currentModelId,
    models,
    configOptions,
    configOptionsSettled: snapshot.config_options_settled === true,
    configBaseline: stringMap(metadata.session.metadata?.acp_config_baseline),
  };
}

/** Builds the partial store state used by SSR and client-side session fetches. */
export function buildSessionModelsState(session: TaskSession | null): {
  sessionModels?: SessionModelsState;
} {
  const entry = sessionModelsEntryFromTaskSession(session);
  if (!session || !entry) return {};
  return { sessionModels: { bySessionId: { [session.id]: entry } } };
}
