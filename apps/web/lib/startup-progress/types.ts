/**
 * Wire shapes for GET /ready's `startup` field, mirroring
 * apps/backend/internal/startup's Snapshot/StepSnapshot JSON tags
 * (AC-PLATFORM-STARTUP-PROGRESS-002/003).
 */

export type StartupPhase =
  | "opening_database"
  | "backing_up_database"
  | "applying_migrations"
  | "initializing_services"
  | "recovering_sessions"
  | "ready";

export type StartupMeasure = "opaque" | "counting" | "counted";

export type StartupUnit = "rows" | "messages" | "turns" | "sessions" | "bytes" | "stores";

export type StartupStepSnapshot = {
  id: string;
  label_key: string;
  measure: StartupMeasure;
  unit: StartupUnit;
  elapsed_ms: number;
  done?: number;
  total?: number;
  rate_per_second?: number;
  eta_ms?: number;
  since_advance_ms?: number;
  stalled: boolean;
};

export type StartupSnapshot = {
  phase: StartupPhase;
  boot: number;
  seq: number;
  elapsed_ms: number;
  phase_elapsed_ms: number;
  step?: StartupStepSnapshot;
};

const STARTUP_PHASES = new Set<StartupPhase>([
  "opening_database",
  "backing_up_database",
  "applying_migrations",
  "initializing_services",
  "recovering_sessions",
  "ready",
]);
const STARTUP_MEASURES = new Set<StartupMeasure>(["opaque", "counting", "counted"]);
const STARTUP_UNITS = new Set<StartupUnit>([
  "rows",
  "messages",
  "turns",
  "sessions",
  "bytes",
  "stores",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function isNonNegativeNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function hasValidStepIdentity(value: Record<string, unknown>): boolean {
  return (
    typeof value.id === "string" &&
    value.id.length > 0 &&
    typeof value.label_key === "string" &&
    value.label_key.length > 0 &&
    typeof value.measure === "string" &&
    STARTUP_MEASURES.has(value.measure as StartupMeasure) &&
    typeof value.unit === "string" &&
    STARTUP_UNITS.has(value.unit as StartupUnit) &&
    isNonNegativeInteger(value.elapsed_ms) &&
    typeof value.stalled === "boolean"
  );
}

function hasValidOptionalStepValues(value: Record<string, unknown>): boolean {
  const optionalIntegers = ["done", "total", "eta_ms", "since_advance_ms"] as const;
  for (const key of optionalIntegers) {
    if (value[key] !== undefined && !isNonNegativeInteger(value[key])) return false;
  }
  if (value.rate_per_second !== undefined && !isNonNegativeNumber(value.rate_per_second)) {
    return false;
  }
  return true;
}

function parseStartupStep(value: unknown): StartupStepSnapshot | null {
  if (!isRecord(value) || !hasValidStepIdentity(value) || !hasValidOptionalStepValues(value)) {
    return null;
  }
  return value as StartupStepSnapshot;
}

/** Returns a trusted startup snapshot or null for malformed wire data. */
export function parseStartupSnapshot(value: unknown): StartupSnapshot | null {
  if (!isRecord(value)) return null;
  if (
    typeof value.phase !== "string" ||
    !STARTUP_PHASES.has(value.phase as StartupPhase) ||
    !isNonNegativeInteger(value.boot) ||
    !isNonNegativeInteger(value.seq) ||
    !isNonNegativeInteger(value.elapsed_ms) ||
    !isNonNegativeInteger(value.phase_elapsed_ms)
  ) {
    return null;
  }
  if (value.step !== undefined) {
    const step = parseStartupStep(value.step);
    if (!step) return null;
  }
  return value as StartupSnapshot;
}
