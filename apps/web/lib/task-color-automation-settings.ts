import type {
  FixedAutomaticTaskColor,
  SidebarTaskColorAutomation,
  SidebarTaskColorDimension,
  SidebarTaskColorRepositoryTarget,
  SidebarTaskColorRule,
} from "@/lib/types/http-user-settings";

export type {
  FixedAutomaticTaskColor,
  SidebarTaskColorAutomation,
  SidebarTaskColorDimension,
  SidebarTaskColorRepositoryTarget,
  SidebarTaskColorRule,
};

export const AUTOMATIC_TASK_COLOR_DIMENSIONS: readonly SidebarTaskColorDimension[] = [
  "workflow_step",
  "repository",
  "workflow",
  "executor_profile",
  "task_state",
  "priority",
  "origin",
];

export const FIXED_AUTOMATIC_TASK_COLORS: readonly FixedAutomaticTaskColor[] = [
  "gray",
  "red",
  "orange",
  "yellow",
  "green",
  "cyan",
  "blue",
  "indigo",
  "purple",
  "pink",
];

export const MAX_AUTOMATIC_TASK_COLOR_RULES = 50;

export const DEFAULT_SIDEBAR_TASK_COLOR_AUTOMATION: SidebarTaskColorAutomation = {
  enabled: false,
  rules: [],
};

/** Returns a fresh disabled value so callers cannot mutate the shared default. */
export function createDefaultSidebarTaskColorAutomation(): SidebarTaskColorAutomation {
  return { enabled: false, rules: [] };
}

/**
 * Normalizes the portable rule set at every settings-ingestion boundary.
 * Invalid data is isolated to this preference and becomes a disabled empty
 * value, while disabled incomplete rules remain editable and are preserved.
 */
export function parseSidebarTaskColorAutomation(value: unknown): SidebarTaskColorAutomation {
  if (!isRecord(value) || typeof value.enabled !== "boolean" || !Array.isArray(value.rules)) {
    return createDefaultSidebarTaskColorAutomation();
  }
  if (value.rules.length > MAX_AUTOMATIC_TASK_COLOR_RULES) {
    return createDefaultSidebarTaskColorAutomation();
  }

  const rules: SidebarTaskColorRule[] = [];
  const ids = new Set<string>();
  for (const rawRule of value.rules) {
    const rule = parseRule(rawRule, ids);
    if (!rule) return createDefaultSidebarTaskColorAutomation();
    rules.push(rule);
  }
  return { enabled: value.enabled, rules };
}

function parseRule(value: unknown, ids: Set<string>): SidebarTaskColorRule | null {
  if (!isRecord(value)) return null;
  const identity = parseRuleIdentity(value, ids);
  if (!identity) return null;
  const condition = parseCondition(value, identity.enabled);
  if (!condition) return null;
  const output = parseOutput(value, condition.dimension);
  if (!output) return null;
  return { ...identity, condition, output };
}

function parseRuleIdentity(
  value: Record<string, unknown>,
  ids: Set<string>,
): { id: string; enabled: boolean } | null {
  if (typeof value.id !== "string" || !value.id) return null;
  if (byteLength(value.id) > 64 || ids.has(value.id) || typeof value.enabled !== "boolean") {
    return null;
  }
  ids.add(value.id);
  return { id: value.id, enabled: value.enabled };
}

function parseCondition(
  value: Record<string, unknown>,
  enabled: boolean,
): SidebarTaskColorRule["condition"] | null {
  if (!isRecord(value.condition) || typeof value.condition.dimension !== "string") return null;
  const dimension = value.condition.dimension as SidebarTaskColorDimension;
  if (!isDimension(dimension)) return null;
  const label = value.condition.label ?? "";
  if (typeof label !== "string" || Array.from(label).length > 200) return null;
  const target = value.condition.value;
  if (target !== null && target !== undefined && !isTarget(dimension, target)) return null;
  if ((target === null || target === undefined) && enabled) return null;
  return { dimension, value: target ?? null, label };
}

function parseOutput(
  value: Record<string, unknown>,
  dimension: SidebarTaskColorDimension,
): SidebarTaskColorRule["output"] | null {
  if (!isRecord(value.output) || typeof value.output.kind !== "string") return null;
  if (value.output.kind === "fixed" && isFixedColor(value.output.color)) {
    return { kind: "fixed", color: value.output.color };
  }
  if (
    value.output.kind === "workflow_step" &&
    dimension === "workflow_step" &&
    value.output.color === undefined
  ) {
    return { kind: "workflow_step" };
  }
  return null;
}

type ScalarTargetDimension = "executor_profile" | "task_state" | "priority" | "origin";
type StructuredTargetDimension = Exclude<SidebarTaskColorDimension, ScalarTargetDimension>;

const SCALAR_TARGET_DIMENSIONS: readonly ScalarTargetDimension[] = [
  "executor_profile",
  "task_state",
  "priority",
  "origin",
];

const STRUCTURED_TARGET_VALIDATORS: Record<StructuredTargetDimension, (value: unknown) => boolean> =
  {
    workflow_step: (value) => isBoundedExactTarget(value, ["workspace_id", "step_id"], 512),
    workflow: (value) => isBoundedExactTarget(value, ["workspace_id", "workflow_id"], 512),
    repository: isRepositoryTarget,
  };

function isTarget(dimension: SidebarTaskColorDimension, value: unknown): boolean {
  if (isScalarTargetDimension(dimension)) return isBoundedString(value, 512);
  return STRUCTURED_TARGET_VALIDATORS[dimension](value);
}

function isScalarTargetDimension(
  dimension: SidebarTaskColorDimension,
): dimension is ScalarTargetDimension {
  return SCALAR_TARGET_DIMENSIONS.includes(dimension as ScalarTargetDimension);
}

function isRepositoryTarget(value: unknown): boolean {
  if (!isRecord(value) || typeof value.kind !== "string") return false;
  if (value.kind === "workspace") {
    return isBoundedExactTarget(value, ["kind", "workspace_id", "repository_id"], 512);
  }
  if (value.kind === "provider") {
    return isBoundedExactTarget(
      value,
      ["kind", "provider_id", "host", "scope", "provider_repository_id"],
      512,
    );
  }
  return value.kind === "local" && isBoundedExactTarget(value, ["kind", "path"], 4096);
}

function isBoundedString(value: unknown, maxBytes: number): value is string {
  return typeof value === "string" && value.length > 0 && byteLength(value) <= maxBytes;
}

function isBoundedExactTarget(
  value: unknown,
  fields: string[],
  maxBytes: number,
): value is Record<string, string> {
  return (
    isRecord(value) &&
    hasExactStringFields(value, fields) &&
    Object.values(value).every((field) => byteLength(field) <= maxBytes)
  );
}

function hasExactStringFields(
  value: Record<string, unknown>,
  fields: string[],
): value is Record<string, string> {
  return (
    Object.keys(value).length === fields.length &&
    fields.every((field) => typeof value[field] === "string" && value[field].length > 0)
  );
}

function isDimension(value: string): value is SidebarTaskColorDimension {
  return (AUTOMATIC_TASK_COLOR_DIMENSIONS as readonly string[]).includes(value);
}

function isFixedColor(value: unknown): value is FixedAutomaticTaskColor {
  return (
    typeof value === "string" && (FIXED_AUTOMATIC_TASK_COLORS as readonly string[]).includes(value)
  );
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
