import type { LastAgentError } from "./session-last-agent-error";
import type { AgentErrorCause } from "./types/task-status-summary";
import type { RuntimeStorageFailure, RuntimeStorageHealth } from "./types/http";
import { parseStrictRfc3339Timestamp } from "./utils/strict-timestamp";

export type { RuntimeStorageHealth };

export type AgentStorageEvidence = {
  reason: StorageReason;
  location: "local_runtime" | "executor_runtime" | "unknown";
  executorLabel?: string;
  observedAt?: string;
  diskAvailableBytes?: number;
  diskRequiredBytes?: number;
  retainedBytes?: number;
  limitBytes?: number;
  eventBytes?: number;
};

export type StorageReason =
  | "disk_space_low"
  | "filesystem_full"
  | "disk_quota_exceeded"
  | "journal_limit_reached"
  | "stream_limit_reached"
  | "event_too_large"
  | "storage_write_failed";

export type StorageCauseKind = "capacity" | "limit" | "oversized" | "unknown";

const STORAGE_REASONS: ReadonlySet<string> = new Set<StorageReason>([
  "disk_space_low",
  "filesystem_full",
  "disk_quota_exceeded",
  "journal_limit_reached",
  "stream_limit_reached",
  "event_too_large",
  "storage_write_failed",
]);

function safeInteger(value: unknown, minimum: number): number | undefined {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum)
    return undefined;
  return value;
}

function boundedLabel(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const trimmed = value.trim();
  if (!trimmed || trimmed.length > 64 || /[/\\@\u0000-\u001f]/.test(trimmed)) return undefined;
  return trimmed;
}

/**
 * Accepts only the closed storage reason and nonnegative safe integers.
 * Malformed or foreign evidence never becomes a disk claim.
 */
export function normalizeAgentErrorStorage(value: unknown): AgentStorageEvidence | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  if (typeof record.reason !== "string" || !STORAGE_REASONS.has(record.reason)) return null;
  const location =
    record.location === "local_runtime" || record.location === "executor_runtime"
      ? record.location
      : "unknown";
  const observedAt = readStrictTimestamp(record.observed_at);
  const executorLabel = boundedLabel(record.executor_label);
  return {
    reason: record.reason as StorageReason,
    location,
    ...(executorLabel ? { executorLabel } : {}),
    ...(observedAt ? { observedAt } : {}),
    ...readStorageMeasurements(record),
  };
}

function readStorageMeasurements(record: Record<string, unknown>) {
  const measurements: Partial<AgentStorageEvidence> = {};
  const available = safeInteger(record.disk_available_bytes, 0);
  const required = safeInteger(record.disk_required_bytes, 0);
  const retained = safeInteger(record.retained_bytes, 0);
  const limit = safeInteger(record.limit_bytes, 1);
  const eventBytes = safeInteger(record.event_bytes, 0);
  if (available !== undefined) measurements.diskAvailableBytes = available;
  if (required !== undefined) measurements.diskRequiredBytes = required;
  if (retained !== undefined) measurements.retainedBytes = retained;
  if (limit !== undefined) measurements.limitBytes = limit;
  if (eventBytes !== undefined) measurements.eventBytes = eventBytes;
  return measurements;
}

export function storageCauseFromCause(cause: AgentErrorCause | null | undefined) {
  if (!cause || cause.operation !== "delivery" || cause.code !== "delivery_storage") return null;
  return normalizeAgentErrorStorage(cause.storage);
}

/** Reads typed storage evidence from a persisted session error. */
export function storageCauseFromError(
  error: LastAgentError | null | undefined,
): AgentStorageEvidence | null {
  if (!error?.causes?.length) return null;
  for (const cause of error.causes) {
    const storage = storageCauseFromCause(cause);
    if (storage) return storage;
  }
  return null;
}

export function storageCauseKind(reason: StorageReason | undefined): StorageCauseKind {
  switch (reason) {
    case "disk_space_low":
    case "filesystem_full":
    case "disk_quota_exceeded":
      return "capacity";
    case "journal_limit_reached":
    case "stream_limit_reached":
      return "limit";
    case "event_too_large":
      return "oversized";
    default:
      return "unknown";
  }
}

export function isLocalStorageCause(cause: AgentStorageEvidence): boolean {
  return cause.location === "local_runtime";
}

export function runtimeStorageUsagePercent(
  health: Pick<RuntimeStorageHealth, "total_bytes" | "available_bytes"> | null | undefined,
): number | null {
  const total = health?.total_bytes;
  const available = health?.available_bytes;
  if (
    typeof total !== "number" ||
    typeof available !== "number" ||
    !Number.isSafeInteger(total) ||
    !Number.isSafeInteger(available) ||
    total <= 0 ||
    available < 0 ||
    available > total
  ) {
    return null;
  }
  return ((total - available) / total) * 100;
}

export const RUNTIME_STORAGE_STALE_AFTER_MS = 90_000;

/**
 * Freshness is receipt age plus local monotonic elapsed time. Source and
 * browser clocks never decide whether a sample is current.
 */
export function isRuntimeStorageStale(
  ageSeconds: number | null | undefined,
  elapsedMs: number,
): boolean {
  const age =
    typeof ageSeconds === "number" && Number.isFinite(ageSeconds) && ageSeconds > 0
      ? ageSeconds
      : 0;
  const elapsed = Number.isFinite(elapsedMs) && elapsedMs > 0 ? elapsedMs : 0;
  return age * 1000 + elapsed > RUNTIME_STORAGE_STALE_AFTER_MS;
}

/**
 * A newer observation from the same backend epoch wins. A newer epoch always
 * wins so a backend restart is not ignored by an old client revision.
 */
export function shouldAcceptRuntimeStorageHealth(
  previous: RuntimeStorageHealth | null | undefined,
  next: RuntimeStorageHealth | null | undefined,
): boolean {
  if (!next) return false;
  if (!previous) return true;
  if (next.epoch !== previous.epoch) return next.epoch > previous.epoch;
  return next.revision >= previous.revision;
}

export function storageAdvisoryVisible(health: RuntimeStorageHealth | null | undefined): boolean {
  return health?.state === "warning" || health?.write_pressure === true;
}

export function storageEvidenceToWire(
  storage: AgentStorageEvidence,
): NonNullable<AgentErrorCause["storage"]> {
  return {
    reason: storage.reason,
    location: storage.location,
    ...(storage.executorLabel ? { executor_label: storage.executorLabel } : {}),
    ...(storage.observedAt ? { observed_at: storage.observedAt } : {}),
    ...(storage.diskAvailableBytes !== undefined
      ? { disk_available_bytes: storage.diskAvailableBytes }
      : {}),
    ...(storage.diskRequiredBytes !== undefined
      ? { disk_required_bytes: storage.diskRequiredBytes }
      : {}),
    ...(storage.retainedBytes !== undefined ? { retained_bytes: storage.retainedBytes } : {}),
    ...(storage.limitBytes !== undefined ? { limit_bytes: storage.limitBytes } : {}),
    ...(storage.eventBytes !== undefined ? { event_bytes: storage.eventBytes } : {}),
  };
}

function optionalSafeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function readRuntimeStorageState(value: unknown): RuntimeStorageHealth["state"] | null {
  return value === "warning" || value === "normal" || value === "unavailable" ? value : null;
}

function readRuntimeStorageLocation(value: unknown): RuntimeStorageHealth["location"] {
  return value === "local_runtime" || value === "executor_runtime" ? value : "unknown";
}

function readStrictTimestamp(value: unknown): string | undefined {
  return typeof value === "string" && parseStrictRfc3339Timestamp(value) !== null
    ? value
    : undefined;
}

function readRuntimeStorageFailure(value: unknown): RuntimeStorageFailure | undefined {
  const storage = normalizeAgentErrorStorage(value);
  return storage ? storageEvidenceToWire(storage) : undefined;
}

/**
 * Accepts only the version-1 advisory shape. Malformed measurements and
 * unsupported states stay absent instead of becoming a claim.
 */
export function normalizeRuntimeStorageHealth(value: unknown): RuntimeStorageHealth | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  const state = readRuntimeStorageState(record.state);
  const epoch = optionalSafeInteger(record.epoch);
  const revision = optionalSafeInteger(record.revision);
  if (!state || epoch === undefined || revision === undefined) return null;
  return {
    state,
    location: readRuntimeStorageLocation(record.location),
    ...readRuntimeStorageOptionalFields(record),
    revision,
    epoch,
  };
}

function readRuntimeStorageOptionalFields(
  record: Record<string, unknown>,
): Partial<RuntimeStorageHealth> {
  const storageKind = record.storage_kind;
  const label =
    typeof record.executor_label === "string" && record.executor_label.trim()
      ? record.executor_label.trim().slice(0, 64)
      : undefined;
  const observedAt = readStrictTimestamp(record.observed_at);
  const deliveryState =
    typeof record.delivery_state === "string" ? record.delivery_state.slice(0, 64) : undefined;
  const failure = readRuntimeStorageFailure(record.failure_storage);
  return {
    ...readRuntimeStorageMeasurements(record),
    ...(storageKind === "journal" || storageKind === "workspace"
      ? { storage_kind: storageKind }
      : {}),
    ...(label ? { executor_label: label } : {}),
    ...(observedAt ? { observed_at: observedAt } : {}),
    ...(record.write_pressure === true ? { write_pressure: true } : {}),
    ...(record.producer_paused === true ? { producer_paused: true } : {}),
    ...(deliveryState ? { delivery_state: deliveryState } : {}),
    ...(failure ? { failure_storage: failure } : {}),
  };
}

function readRuntimeStorageMeasurements(
  record: Record<string, unknown>,
): Partial<RuntimeStorageHealth> {
  const measurements: Partial<RuntimeStorageHealth> = {};
  const totalBytes = optionalSafeInteger(record.total_bytes);
  const availableBytes = optionalSafeInteger(record.available_bytes);
  const ageSeconds = optionalSafeInteger(record.age_seconds);
  if (totalBytes !== undefined) measurements.total_bytes = totalBytes;
  if (availableBytes !== undefined) measurements.available_bytes = availableBytes;
  if (ageSeconds !== undefined) measurements.age_seconds = ageSeconds;
  return measurements;
}

/**
 * Merge runtime-only storage state at the shared session boundary. Incoming
 * snapshots with an older epoch or revision never replace a newer live update.
 */
export function mergeRuntimeStorageHealth(
  existing: RuntimeStorageHealth | null | undefined,
  incoming: RuntimeStorageHealth | null | undefined,
): RuntimeStorageHealth | undefined {
  if (incoming === undefined) return existing ?? undefined;
  if (incoming === null) return undefined;
  const next = normalizeRuntimeStorageHealth(incoming);
  const prior = existing ? normalizeRuntimeStorageHealth(existing) : null;
  if (!next) return prior ?? existing ?? undefined;
  if (!prior) return next;
  return shouldAcceptRuntimeStorageHealth(prior, next) ? next : prior;
}

export function formatStorageUsage(value: number | null): string | null {
  if (value === null || !Number.isFinite(value)) return null;
  return value >= 99.95 ? "100" : value.toFixed(1).replace(/\.0$/, "");
}
