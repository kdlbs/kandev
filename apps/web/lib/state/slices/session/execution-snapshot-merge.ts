import type { ExecutionSnapshot } from "@/lib/types/execution";
import type { TaskSession } from "@/lib/types/http";

export function mergeExecutionSnapshot(
  existing: TaskSession,
  incoming: TaskSession,
): ExecutionSnapshot | null {
  const recreated =
    incoming.queue_incarnation_id !== undefined &&
    incoming.queue_incarnation_id !== existing.queue_incarnation_id;
  const current = recreated ? null : existing.execution_snapshot;
  const next = incoming.execution_snapshot;
  if (!validSnapshot(next, existing) || (current && next.sequence <= current.sequence)) {
    return current ?? null;
  }
  return next;
}

function validSnapshot(
  snapshot: ExecutionSnapshot | null | undefined,
  session: TaskSession,
): snapshot is ExecutionSnapshot {
  return Boolean(
    snapshot &&
    snapshot.schema_version === 1 &&
    snapshot.session_id === session.id &&
    snapshot.task_id === session.task_id &&
    snapshot.attempt_id &&
    snapshot.execution_id &&
    Number.isSafeInteger(snapshot.sequence) &&
    snapshot.sequence > 0 &&
    Number.isSafeInteger(snapshot.generation) &&
    snapshot.generation > 0,
  );
}
