import type { TaskStatusSummaryLaunchQueue } from "@/lib/types/task-status-summary";

export const LAUNCH_QUEUE_CAPACITY_REASON_LABELS = {
  ceiling_total_limit: "task:launchQueueRefusalTotal",
  ceiling_grade_limit: "task:launchQueueRefusalGrade",
  ceiling_both_limits: "task:launchQueueRefusalBoth",
  ceiling_needs_configuration: "task:launchQueueRefusalConfiguration",
  ceiling_unknown_population: "task:launchQueueRefusalUnknown",
} as const;
type CapacityReason = keyof typeof LAUNCH_QUEUE_CAPACITY_REASON_LABELS;

function recordedCapacityReason(reason?: string): CapacityReason | null {
  return reason && Object.hasOwn(LAUNCH_QUEUE_CAPACITY_REASON_LABELS, reason)
    ? (reason as CapacityReason)
    : null;
}

export type LaunchQueueCapacityFreshness = "current" | "stale" | "unavailable";

export type LaunchQueueViewModel = {
  capacityReason?: CapacityReason;
  destinationId: string | null;
  queuedAt: string;
  reason: TaskStatusSummaryLaunchQueue["reason"];
  retrying: boolean;
  capacityFreshness: LaunchQueueCapacityFreshness;
  capacity: {
    inUse: number;
    limit: number;
    observedAt: string;
    policy?: "all";
    grades?: { grade: string; inUse: number | null; limit: number | null }[] | null;
    needsConfiguration?: boolean;
  } | null;
};

const LAUNCH_QUEUE_REASONS = new Set<TaskStatusSummaryLaunchQueue["reason"]>([
  "session_capacity",
  "ownership_unavailable",
  "replay_error",
]);

export const LAUNCH_QUEUE_CAPACITY_STALE_AFTER_MS = 40_000;

function gradeRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function observedCount(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function capacityGradeRows(capacity: NonNullable<TaskStatusSummaryLaunchQueue["capacity"]>) {
  const counts = gradeRecord(capacity.grade_in_use);
  const limits = gradeRecord(capacity.grade_limits);
  if (!counts || !limits) return null;
  return [...new Set([...Object.keys(limits), ...Object.keys(counts)])].map((grade) => ({
    grade,
    inUse: observedCount(counts[grade]),
    limit: observedCount(limits[grade]),
  }));
}

function validCapacity(
  capacity: TaskStatusSummaryLaunchQueue["capacity"],
): LaunchQueueViewModel["capacity"] {
  if (!capacity || !capacity.observed_at) return null;
  if (
    !Number.isInteger(capacity.in_use) ||
    !Number.isInteger(capacity.limit) ||
    capacity.in_use < 0 ||
    capacity.limit < 0
  ) {
    return null;
  }
  if (!Number.isFinite(Date.parse(capacity.observed_at))) return null;
  return {
    inUse: capacity.in_use,
    limit: capacity.limit,
    observedAt: capacity.observed_at,
    ...(capacity.policy === "all"
      ? {
          policy: capacity.policy,
          grades: capacityGradeRows(capacity),
          needsConfiguration: capacity.needs_configuration === true,
        }
      : {}),
  };
}

/** Convert the wire summary into the small, presentation-independent queue model. */
export function buildLaunchQueueViewModel(
  queue: TaskStatusSummaryLaunchQueue | null | undefined,
  options: { now?: number; isConnected?: boolean } = {},
): LaunchQueueViewModel | null {
  if (!queue || !queue.queued_at || !LAUNCH_QUEUE_REASONS.has(queue.reason)) return null;
  const capacity = validCapacity(queue.capacity);
  const capacityReason = recordedCapacityReason(queue.capacity_reason);
  const now = options.now ?? Date.now();
  const isConnected = options.isConnected ?? true;
  let capacityFreshness: LaunchQueueCapacityFreshness = "unavailable";
  if (capacity) {
    const observedAt = Date.parse(capacity.observedAt);
    const age = now - observedAt;
    capacityFreshness =
      !isConnected || age >= LAUNCH_QUEUE_CAPACITY_STALE_AFTER_MS ? "stale" : "current";
  }
  return {
    // Session IDs are ownership keys, not user-facing destination labels. A
    // missing profile projection must fall back to the translated generic
    // label instead of exposing an opaque UUID.
    destinationId: queue.agent_profile_id?.trim() || null,
    ...(capacityReason ? { capacityReason } : {}),
    queuedAt: queue.queued_at,
    reason: queue.reason,
    retrying: queue.retrying,
    capacityFreshness,
    capacity,
  };
}

export type LaunchQueueCapacityCause = {
  key:
    | "task:launchQueueCauseGrade"
    | "task:launchQueueCauseGradeUnmeasured"
    | "task:launchQueueCauseTotal";
  values?: Record<string, string | number>;
};

/**
 * The one limit that holds the launch, from the recorded refusal reason and
 * the capacity sample the payload already carries. A grade refusal names the
 * full grade and its usage when exactly one grade is full; otherwise it says
 * the grade limit without numbers. When both limits refused, the narrower
 * grade limit is the one named, or the overall limit when the sample shows it
 * reached and no single grade can be named.
 */
export function launchQueueCapacityCause(
  view: LaunchQueueViewModel,
): LaunchQueueCapacityCause | null {
  const reason = view.capacityReason;
  if (reason === "ceiling_grade_limit" || reason === "ceiling_both_limits") {
    const full = (view.capacity?.grades ?? []).filter(
      (row) => row.inUse !== null && row.limit !== null && row.inUse >= row.limit,
    );
    if (full.length === 1) {
      const [row] = full;
      return {
        key: "task:launchQueueCauseGrade",
        values: { grade: row.grade, inUse: row.inUse!, limit: row.limit! },
      };
    }
    if (reason === "ceiling_both_limits" && capacityAtLimit(view.capacity))
      return {
        key: "task:launchQueueCauseTotal",
        values: { inUse: view.capacity!.inUse, limit: view.capacity!.limit },
      };
    return { key: "task:launchQueueCauseGradeUnmeasured" };
  }
  if (reason === "ceiling_total_limit" && view.capacity)
    return {
      key: "task:launchQueueCauseTotal",
      values: { inUse: view.capacity.inUse, limit: view.capacity.limit },
    };
  return null;
}

/** A recorded overall limit is full; a limit of zero is not a denominator. */
function capacityAtLimit(capacity: LaunchQueueViewModel["capacity"]): boolean {
  return Boolean(capacity && capacity.limit > 0 && capacity.inUse >= capacity.limit);
}
