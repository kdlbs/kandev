import type { TaskSession } from "@/lib/types/http";

/** Returns the value narrowed to its string-valued entries, or undefined when none qualify. */
function stringMap(value: unknown): Record<string, string> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] => typeof entry[1] === "string",
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

/** Returns the value as a plain record when it is a non-array object; otherwise undefined. */
function objectMap(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

/** Returns the value when it is a string; otherwise undefined. */
function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

/** Filters an array down to its plain-object items, or returns [] for non-arrays. */
function objectList(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
    : [];
}

/** Maps a raw ACP model record to the display shape, defaulting missing strings to "". */
function mapSessionModel(model: Record<string, unknown>) {
  return {
    modelId: stringValue(model.model_id) ?? "",
    name: stringValue(model.name) ?? "",
    description: stringValue(model.description),
    usageMultiplier: stringValue(model.usage_multiplier),
  };
}

/**
 * Maps a raw ACP config option, preferring the runtime (or override) value over
 * the option's own current_value and defaulting type to "select" when absent.
 */
function mapSessionConfigOption(
  option: Record<string, unknown>,
  runtimeOptions: Record<string, string>,
) {
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
 * Extracts the ACP model-state snapshot, runtime config, and overrides from a
 * session's metadata; returns {} when the session or its snapshot is missing.
 */
function sessionModelHydrationMetadata(session: TaskSession | null) {
  if (!session) return {};
  const snapshot = objectMap(session.metadata?.acp_model_state);
  if (!snapshot) return {};
  return {
    session,
    snapshot,
    runtime: objectMap(session.metadata?.runtime_config),
    overrides: objectMap(session.metadata?.runtime_config_overrides),
  };
}

/**
 * Builds the sessionModels slice (current model, available models, config
 * options) for a session from its ACP hydration metadata; {} when nothing
 * resolvable exists.
 */
export function buildSessionModelsState(session: TaskSession | null) {
  const metadata = sessionModelHydrationMetadata(session);
  if (!metadata.session || !metadata.snapshot) return {};
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
  if (!currentModelId && models.length === 0 && configOptions.length === 0) return {};

  return {
    sessionModels: {
      bySessionId: {
        [metadata.session.id]: {
          currentModelId,
          models,
          configOptions,
          configOptionsSettled: snapshot.config_options_settled === true,
          configBaseline: stringMap(metadata.session.metadata?.acp_config_baseline),
        },
      },
    },
  };
}
