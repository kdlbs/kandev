import type { ContextCompactionReporting, ContextWindowEntry } from "./types";

function normalizeCompactionCount(value: unknown): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0;
  return Math.max(0, Math.trunc(value));
}

export function normalizeCompactionReporting(
  value: unknown,
): ContextCompactionReporting | undefined {
  if (!value || typeof value !== "object") return undefined;
  const reporting = value as Record<string, unknown>;
  const source =
    reporting.source === "usage_drop" || reporting.source === "copilot_native"
      ? reporting.source
      : undefined;
  const availability =
    reporting.availability === "available" || reporting.availability === "unavailable"
      ? reporting.availability
      : undefined;
  const coverage =
    reporting.coverage === "observed" || reporting.coverage === "partial"
      ? reporting.coverage
      : undefined;
  const revision =
    typeof reporting.revision === "number" && Number.isFinite(reporting.revision)
      ? Math.max(0, Math.trunc(reporting.revision))
      : 0;
  if (!source || !availability || !coverage) return undefined;
  return { source, availability, coverage, revision };
}

export function parseContextWindowEntry(
  value: unknown,
  timestamp?: string,
  compactionCount?: unknown,
  compactionReporting?: unknown,
): ContextWindowEntry | null {
  if (!value || typeof value !== "object") return null;

  const contextWindow = value as Record<string, unknown>;
  const source =
    contextWindow.source === "acp" || contextWindow.source === "api"
      ? contextWindow.source
      : undefined;

  return {
    size: (contextWindow.size as number) ?? 0,
    used: (contextWindow.used as number) ?? 0,
    remaining: (contextWindow.remaining as number) ?? 0,
    efficiency: (contextWindow.efficiency as number) ?? 0,
    compactionCount: normalizeCompactionCount(compactionCount),
    compactionReporting: normalizeCompactionReporting(compactionReporting),
    source,
    timestamp: timestamp ?? (contextWindow.timestamp as string | undefined),
  };
}

function newerReporting(
  previous: ContextCompactionReporting | undefined,
  incoming: ContextCompactionReporting | undefined,
): ContextCompactionReporting | undefined {
  if (!incoming) return previous;
  if (!previous) return incoming;
  if (incoming.revision < previous.revision) return previous;
  if (incoming.revision === previous.revision && previous.coverage === "partial") {
    return previous;
  }
  return incoming;
}

/**
 * Merge a live gauge sample with the already-known lifetime count. A delayed
 * gauge or HTTP snapshot must never lower a newer count or replace newer
 * reporting metadata.
 */
export function mergeCompactionCountValue(current: unknown, incoming: unknown): number | undefined {
  if (current === undefined && incoming === undefined) return undefined;
  return Math.max(normalizeCompactionCount(current), normalizeCompactionCount(incoming));
}

export function mergeCompactionReportingValue(
  current: unknown,
  incoming: unknown,
): ContextCompactionReporting | undefined {
  return newerReporting(
    normalizeCompactionReporting(current),
    normalizeCompactionReporting(incoming),
  );
}

/** Clear only the gauge while retaining the lifetime count and reporting. */
export function clearContextWindowGauge(
  existing: ContextWindowEntry | undefined,
): ContextWindowEntry | null {
  if (!existing) return null;
  return {
    ...existing,
    size: 0,
    used: 0,
    remaining: 0,
    efficiency: 0,
    source: undefined,
  };
}

function sameCompactionReporting(
  left: ContextCompactionReporting | undefined,
  right: ContextCompactionReporting | undefined,
): boolean {
  if (left === right) return true;
  if (!left || !right) return false;
  return (
    left.revision === right.revision &&
    left.source === right.source &&
    left.availability === right.availability &&
    left.coverage === right.coverage
  );
}

export function sameContextWindowEntry(
  left: ContextWindowEntry | undefined,
  right: ContextWindowEntry | undefined,
): boolean {
  if (left === right) return true;
  if (!left || !right) return false;
  return (
    left.size === right.size &&
    left.used === right.used &&
    left.remaining === right.remaining &&
    left.efficiency === right.efficiency &&
    left.compactionCount === right.compactionCount &&
    left.source === right.source &&
    left.timestamp === right.timestamp &&
    sameCompactionReporting(left.compactionReporting, right.compactionReporting)
  );
}

function metadataIsNewer(
  current: ContextWindowEntry | undefined,
  count: unknown,
  reporting: unknown,
): boolean {
  if (!current) return true;
  const metadataCount = normalizeCompactionCount(count);
  const metadataRevision = normalizeCompactionReporting(reporting)?.revision ?? -1;
  const cacheRevision = current.compactionReporting?.revision ?? -1;
  return metadataCount > current.compactionCount || metadataRevision > cacheRevision;
}

/**
 * Reconcile retained session metadata with the live cache. `undefined` means
 * no change, `null` means clear the gauge, and an entry means publish it.
 * Hydration passes `allowExplicitClear=false` so a stale null snapshot cannot
 * erase a newer live cache.
 */
export type ReconcileContextWindowInput = {
  current: ContextWindowEntry | undefined;
  gauge: unknown;
  hasGauge: boolean;
  count: unknown;
  reporting: unknown;
  timestamp: string | undefined;
  allowExplicitClear: boolean;
};

export function reconcileContextWindowMetadata(
  input: ReconcileContextWindowInput,
): ContextWindowEntry | null | undefined {
  const { current, gauge, hasGauge, count, reporting, timestamp, allowExplicitClear } = input;
  if (hasGauge && gauge === null) {
    if (!allowExplicitClear) {
      // Hydration must not clear a live gauge, but a newer retained count still
      // has to reach the cache so the next gauge inherits it.
      if (!current || !metadataIsNewer(current, count, reporting)) return undefined;
      return patchContextWindowCompaction(current, count, reporting, timestamp);
    }
    if (!current) return null;
    const cleared = clearContextWindowGauge(current);
    if (!cleared) return null;
    cleared.compactionCount = mergeCompactionCountValue(count, current.compactionCount) ?? 0;
    cleared.compactionReporting = mergeCompactionReportingValue(
      reporting,
      current.compactionReporting,
    );
    return cleared;
  }
  if (!hasGauge) {
    if (!current || !metadataIsNewer(current, count, reporting)) return undefined;
    return patchContextWindowCompaction(current, count, reporting, timestamp);
  }
  if (!current) {
    return parseContextWindowEntry(gauge, timestamp, count, reporting);
  }
  // Every non-null gauge sample is a live reading. Normal token growth changes
  // neither the count nor the reporting revision, so freshness checks belong
  // only to count-only metadata updates. mergeContextWindowEntry keeps the
  // lifetime count and reporting metadata monotonic.
  const parsed = parseContextWindowEntry(gauge, timestamp, count, reporting);
  if (!parsed) return undefined;
  return mergeContextWindowEntry(current, parsed);
}

export function mergeContextWindowEntry(
  previous: ContextWindowEntry | undefined,
  incoming: ContextWindowEntry,
): ContextWindowEntry {
  if (!previous) return incoming;
  return {
    ...incoming,
    compactionCount: Math.max(previous.compactionCount, incoming.compactionCount),
    compactionReporting: newerReporting(previous.compactionReporting, incoming.compactionReporting),
    timestamp: incoming.timestamp ?? previous.timestamp,
  };
}

/**
 * Apply a count-only native update without inventing a gauge sample. The
 * lifetime count still persists in session metadata, so the next real gauge
 * inherits it; before then there is nothing reliable to render.
 */
export function patchContextWindowCompaction(
  existing: ContextWindowEntry | undefined,
  compactionCount: unknown,
  compactionReporting: unknown,
  timestamp?: string,
): ContextWindowEntry | null {
  if (!existing) return null;
  const count = normalizeCompactionCount(compactionCount);
  const reporting = normalizeCompactionReporting(compactionReporting);
  return {
    ...existing,
    compactionCount: Math.max(existing.compactionCount, count),
    compactionReporting: newerReporting(existing.compactionReporting, reporting),
    timestamp: timestamp ?? existing.timestamp,
  };
}
