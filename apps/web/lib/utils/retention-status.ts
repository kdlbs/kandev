import type {
  RetentionLastSweep,
  RetentionStatus,
  RetentionTableCensus,
  RetentionTableSweepResult,
  RetentionSweptTableResult,
} from "@/lib/types/system";

export const RETENTION_COUNT_KEYS = ["office_routine_runs", "runs", "run_events"] as const;

export type RetentionCountKey = (typeof RETENTION_COUNT_KEYS)[number];
export type RetentionCountState = "unavailable" | "zero" | "measured" | "stale";
export type RetentionSweepOutcome = "none" | "success" | "preview" | "mixed" | "error";

export type RetentionCountView = {
  key: RetentionCountKey;
  census: RetentionTableCensus;
  state: RetentionCountState;
  overThreshold: boolean;
};

export type RetentionSweepView = {
  outcome: RetentionSweepOutcome;
  deletionTotal: number;
  hasDeletion: boolean;
  hasPreview: boolean;
  hasBacklog: boolean;
  hasError: boolean;
};

export type RetentionStatusViewModel = {
  enabled: boolean;
  counts: RetentionCountView[];
  commonAsOf: string | null;
  sweep: RetentionSweepView;
};

const sweepResults = (lastSweep: RetentionLastSweep | null): RetentionTableSweepResult[] => {
  if (!lastSweep) return [];
  return [
    lastSweep.office_routine_runs,
    lastSweep.runs,
    lastSweep.run_events,
    lastSweep.route_attempts,
    lastSweep.run_skills,
  ];
};

const isSweptResult = (result: RetentionTableSweepResult): result is RetentionSweptTableResult =>
  "previewed" in result;

function countState(census: RetentionTableCensus): RetentionCountState {
  if (census.state === "not_computed") return "unavailable";
  if (census.state === "stale") return "stale";
  return census.retained_count === 0 ? "zero" : "measured";
}

function commonTimestamp(counts: RetentionCountView[]): string | null {
  const timestamps = counts
    .filter((count) => count.census.state !== "not_computed" && count.census.as_of)
    .map((count) => count.census.as_of);
  if (timestamps.length !== counts.length || new Set(timestamps).size !== 1) return null;
  return timestamps[0] ?? null;
}

function sweepView(lastSweep: RetentionLastSweep | null): RetentionSweepView {
  if (!lastSweep) {
    return {
      outcome: "none",
      deletionTotal: 0,
      hasDeletion: false,
      hasPreview: false,
      hasBacklog: false,
      hasError: false,
    };
  }
  const results = sweepResults(lastSweep);
  const hasDeletion = results.some((result) => result.deleted > 0);
  const hasPreview = results.some((result) => isSweptResult(result) && result.previewed);
  const hasBacklog = results.some((result) => result.backlog);
  const hasError = results.some((result) => Boolean(result.error));
  let outcome: RetentionSweepOutcome = "success";
  if (hasError) outcome = "error";
  else if (hasDeletion && hasPreview) outcome = "mixed";
  else if (hasPreview) outcome = "preview";
  return {
    outcome,
    deletionTotal: results.reduce((total, result) => total + result.deleted, 0),
    hasDeletion,
    hasPreview,
    hasBacklog,
    hasError,
  };
}

export function buildRetentionStatusViewModel(status: RetentionStatus): RetentionStatusViewModel {
  const thresholds: Record<RetentionCountKey, number> = {
    office_routine_runs: status.settings.routine_runs.warn_rows,
    runs: status.settings.runs.warn_rows,
    run_events: status.settings.run_events.warn_rows,
  };
  const counts = RETENTION_COUNT_KEYS.map((key) => {
    const census = status.retained_counts[key];
    return {
      key,
      census,
      state: countState(census),
      overThreshold: thresholds[key] > 0 && census.retained_count > thresholds[key],
    };
  });
  return {
    enabled: status.settings.enabled,
    counts,
    commonAsOf: commonTimestamp(counts),
    sweep: sweepView(status.last_sweep),
  };
}
