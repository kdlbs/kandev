import type { TaskStatusSummaryLaunchQueue } from "@/lib/types/task-status-summary";

export type LaunchQueueCapacityFreshness = "current" | "stale" | "unavailable";

export type LaunchQueueViewModel = {
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
    queuedAt: queue.queued_at,
    reason: queue.reason,
    retrying: queue.retrying,
    capacityFreshness,
    capacity,
  };
}
