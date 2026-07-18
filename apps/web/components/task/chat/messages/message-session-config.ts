type TurnConfigOption = {
  id: string;
  name?: string;
  value: string;
  value_name?: string;
};

type TurnConfigSnapshot = {
  model?: string;
  config_options?: TurnConfigOption[];
  config_baseline?: Record<string, string>;
};

const TURN_CONFIG_SNAPSHOT_KEY = "runtime_config_snapshot";

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

function recordValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function parseConfigOption(value: unknown): TurnConfigOption | null {
  const option = recordValue(value);
  if (!option) return null;
  const id = stringValue(option.id);
  const selectedValue = stringValue(option.value);
  if (!id || !selectedValue) return null;
  return {
    id,
    name: stringValue(option.name) ?? undefined,
    value: selectedValue,
    value_name: stringValue(option.value_name) ?? undefined,
  };
}

function parseConfigBaseline(value: unknown): Record<string, string> | undefined {
  const baseline = recordValue(value);
  if (!baseline) return undefined;
  return Object.fromEntries(
    Object.entries(baseline).filter((entry): entry is [string, string] => {
      return typeof entry[1] === "string";
    }),
  );
}

function parseTurnConfigSnapshot(metadata: Record<string, unknown> | null | undefined) {
  const raw = recordValue(metadata?.[TURN_CONFIG_SNAPSHOT_KEY]);
  if (!raw) return null;
  return {
    model: stringValue(raw.model) ?? undefined,
    config_options: Array.isArray(raw.config_options)
      ? raw.config_options.map(parseConfigOption).filter((option) => option !== null)
      : [],
    config_baseline: parseConfigBaseline(raw.config_baseline),
  } satisfies TurnConfigSnapshot;
}

function humanizeConfigKey(key: string): string {
  return key
    .replace(/[_-]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .trim()
    .replace(/^./, (char) => char.toUpperCase());
}

function changedConfigOptions(snapshot: TurnConfigSnapshot): TurnConfigOption[] {
  const baseline = snapshot.config_baseline;
  return (snapshot.config_options ?? []).filter((option) => {
    if (option.id === "model") return false;
    return (
      baseline === undefined ||
      !Object.hasOwn(baseline, option.id) ||
      baseline[option.id] !== option.value
    );
  });
}

export function formatMessageSessionConfig(
  messageMetadata: Record<string, unknown> | null | undefined,
  turnMetadata: Record<string, unknown> | null | undefined,
): string | null {
  const snapshot = parseTurnConfigSnapshot(turnMetadata);
  const model =
    stringValue(messageMetadata?.model) ??
    stringValue(turnMetadata?.model) ??
    snapshot?.model ??
    null;
  if (!model) return null;
  if (!snapshot) return model;
  const details = changedConfigOptions(snapshot).map((option) => {
    const name = option.name ?? humanizeConfigKey(option.id);
    return `${name}: ${option.value_name ?? option.value}`;
  });
  return details.length > 0 ? `${model} · ${details.join(" · ")}` : model;
}
