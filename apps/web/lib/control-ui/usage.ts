import type {
  ExecutionUsageReport,
  ObservedUsageSample,
  ProfileExecutionUsage,
  CachedProfileUsage,
} from "@/lib/api/domains/provider-usage-api";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function timestamp(value: unknown) {
  return typeof value === "string" && parseStrictRfc3339Timestamp(value) !== null;
}

function validUsageWindow(value: unknown) {
  if (
    !record(value) ||
    typeof value.label !== "string" ||
    (value.model !== undefined && typeof value.model !== "string") ||
    typeof value.utilization_observed !== "boolean" ||
    typeof value.reset_at_observed !== "boolean"
  )
    return false;
  return (
    (!value.utilization_observed ||
      (typeof value.utilization_pct === "number" &&
        Number.isFinite(value.utilization_pct) &&
        value.utilization_pct >= 0)) &&
    (!value.reset_at_observed || timestamp(value.reset_at))
  );
}

function validUsageSample(value: unknown) {
  return (
    record(value) &&
    typeof value.provider === "string" &&
    value.provider.length > 0 &&
    typeof value.account_ref === "string" &&
    value.account_ref.length > 0 &&
    (value.organization_ref === undefined || typeof value.organization_ref === "string") &&
    timestamp(value.fetched_at) &&
    Array.isArray(value.windows) &&
    value.windows.every(validUsageWindow)
  );
}

function validUsageEntry(value: unknown) {
  if (
    !record(value) ||
    typeof value.execution_id !== "string" ||
    !value.execution_id ||
    typeof value.session_id !== "string" ||
    !validUsageReport(value.report)
  )
    return false;
  return true;
}

function validUsageReport(report: unknown) {
  if (!record(report)) return false;
  const samplesValid = [report.current, report.previous].every(
    (sample) => sample === undefined || validUsageSample(sample),
  );
  return (
    typeof report.source_known === "boolean" &&
    typeof report.limited === "boolean" &&
    typeof report.reason === "string" &&
    (report.model === undefined || typeof report.model === "string") &&
    (report.blocking_window === undefined || typeof report.blocking_window === "string") &&
    (report.binding_changed === undefined || typeof report.binding_changed === "boolean") &&
    (!report.source_known || validUsageSample(report.current)) &&
    samplesValid
  );
}

/** Retain unknown source facts, but require a proved cache-only read and exact profile identity. */
export function isCachedProfileUsage(
  value: unknown,
  profileId: string,
): value is CachedProfileUsage {
  return (
    record(value) &&
    value.profile_id === profileId &&
    value.cached_only === true &&
    Array.isArray(value.executions) &&
    value.executions.every(validUsageEntry)
  );
}

export type ProviderAccountObservation = {
  key: string;
  provider?: string;
  account?: string;
  organization?: string;
  sample?: ObservedUsageSample;
  entries: ProfileExecutionUsage[];
};

/** Account windows are one provider sample, never a sum across model executions. */
export function groupExecutionUsage(
  entries: ProfileExecutionUsage[],
): ProviderAccountObservation[] {
  const groups = new Map<string, ProviderAccountObservation>();
  const executions = new Map<string, ProfileExecutionUsage>();
  for (const entry of entries) {
    const previous = executions.get(entry.execution_id);
    if (!previous || sampleTime(entry.report.current) > sampleTime(previous.report.current)) {
      executions.set(entry.execution_id, entry);
    }
  }
  for (const entry of executions.values()) {
    const { key, sample } = accountObservation(entry);
    const group = groups.get(key) ?? {
      key,
      provider: sample?.provider,
      account: sample?.account_ref,
      organization: sample?.organization_ref,
      entries: [],
    };
    group.entries.push(entry);
    if (sample && (!group.sample || sampleTime(sample) > sampleTime(group.sample))) {
      group.sample = sample;
    }
    groups.set(key, group);
  }
  return [...groups.values()];
}

function accountObservation(entry: ProfileExecutionUsage) {
  const sample = entry.report.source_known ? entry.report.current : undefined;
  const key =
    sample?.provider && sample.account_ref
      ? JSON.stringify([sample.provider, sample.account_ref, sample.organization_ref ?? ""])
      : entry.execution_id;
  return { key, sample };
}

function sampleTime(sample?: ObservedUsageSample): number {
  const at = sample ? Date.parse(sample.fetched_at) : NaN;
  return Number.isFinite(at) ? at : -Infinity;
}

export function observedLimitState(report: ExecutionUsageReport, now = Date.now()) {
  if (!report.source_known) return "unknown";
  if (!report.limited) return "not-limited";
  const reset = Date.parse(report.reset_at ?? "");
  return Number.isFinite(reset) && reset <= now ? "expired" : "limited";
}

/** A model's card uses its latest saved report, not contradictory execution history. */
export function latestModelUsage(entries: ProfileExecutionUsage[]) {
  const models = new Map<string, ProfileExecutionUsage>();
  for (const entry of entries) {
    const key = entry.report.model || entry.execution_id;
    const previous = models.get(key);
    if (!previous || sampleTime(entry.report.current) > sampleTime(previous.report.current))
      models.set(key, entry);
  }
  return [...models.values()];
}
