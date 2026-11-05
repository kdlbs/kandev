import { isExecutionObservationBound } from "@/lib/execution-status";
import type { ExecutionObservation, ExecutionSnapshot } from "@/lib/types/execution";
import type { TaskSession } from "@/lib/types/http";

export function mergeExecutionObservation(
  existing: TaskSession,
  incoming: TaskSession,
  snapshot: ExecutionSnapshot | null,
): ExecutionObservation | null {
  const recreated =
    incoming.queue_incarnation_id !== undefined &&
    incoming.queue_incarnation_id !== existing.queue_incarnation_id;
  const current =
    !recreated && isExecutionObservationBound(existing.execution_observation, snapshot)
      ? existing.execution_observation
      : null;
  const next = incoming.execution_observation;
  if (!isExecutionObservationBound(next, snapshot)) return current;
  if (current && !canAdvance(current, next)) return current;
  return next;
}

function canAdvance(current: ExecutionObservation, next: ExecutionObservation): boolean {
  if (
    (current.status === "stopped" || current.status === "failed") &&
    next.status !== current.status
  ) {
    return false;
  }
  return (
    next.observer_generation > current.observer_generation ||
    (next.observer_generation === current.observer_generation && next.revision > current.revision)
  );
}
