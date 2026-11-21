import type { NativeDashboard, NativeSharedCapacityWaiting } from "@/lib/types/control-native";
import type { SessionCapacityObservation } from "@/lib/types/system";
import type { ControlRead } from "@/hooks/domains/coordinator/use-control-resource";

const GRADE_WAIT_REASONS = ["ceiling_grade_limit", "ceiling_both_limits"] as const;

/** WIP is summed only for complete visible-workspace reads; shared waiters are never summed. */
export function capacityDashboardCounts(
  workspaceIds: string[],
  reads: Array<{ id: string } & ControlRead<NativeDashboard>> | undefined,
) {
  const rows = [...new Set(workspaceIds)].map((id) => reads?.find((row) => row.id === id));
  const complete = rows.every(
    (row) =>
      row?.status === "ready" &&
      !row.stale &&
      Number.isSafeInteger(row.data?.wip_waiting_tasks) &&
      (row.data?.wip_waiting_tasks ?? -1) >= 0,
  );
  const wipWaiting = complete
    ? rows.reduce((sum, row) => sum + (row?.data?.wip_waiting_tasks ?? 0), 0)
    : null;
  const sharedWaiting = rows.find(
    (row) => row?.status === "ready" && !row.stale && row.data?.shared_capacity_waiting,
  )?.data?.shared_capacity_waiting;
  return { wipWaiting, sharedWaiting };
}

/** A global shared snapshot is used once; unknown is never interpreted as zero. */
export function capacityGradeWaiting(
  observation: SessionCapacityObservation | undefined,
  known: boolean,
  sharedWaiting?: NativeSharedCapacityWaiting,
): number | null {
  if (sharedWaiting) return sharedWaiting.known && known ? sharedWaiting.grade : null;
  if (!known || observation?.waiting?.known !== true) return null;
  const counts = observation.waiting.counts;
  if (!counts) return null;
  const values = GRADE_WAIT_REASONS.map((reason) => counts[reason]);
  if (!values.every((value) => Number.isSafeInteger(value) && value >= 0)) return null;
  return values.reduce((sum, value) => sum + value, 0);
}

export function capacityWaitingTotal(observation?: SessionCapacityObservation): number | null {
  if (!observation?.waiting?.known || !observation.waiting.counts) return null;
  const values = Object.values(observation.waiting.counts);
  if (!values.every((value) => Number.isSafeInteger(value) && value >= 0)) return null;
  return values.reduce((sum, value) => sum + value, 0);
}
