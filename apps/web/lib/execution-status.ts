import type { ConnectionStatus } from "./types/connection";
import type { ExecutionStatus, ExecutionSnapshot, ExecutionObservation } from "./types/execution";
import type { TaskSession } from "./types/http";

export function getExecutionStatus(
  session: Pick<TaskSession, "id" | "task_id" | "execution_snapshot" | "execution_observation">,
  connection: ConnectionStatus,
  now: number,
): ExecutionStatus {
  const snapshot = session.execution_snapshot;
  if (
    !snapshot ||
    snapshot.schema_version !== 1 ||
    snapshot.session_id !== session.id ||
    snapshot.task_id !== session.task_id
  )
    return "unknown";
  if (!snapshot.sandbox) return "not_configured";
  const value = session.execution_observation;
  if (!isExecutionObservationBound(value, snapshot)) return "unknown";
  if (value.status === "stopped" || value.status === "failed") return value.status;
  if (connection !== "connected" || !isRecentObservation(value.checked_at, now)) return "unknown";
  if (value.status === "active") return hasFreshEvidence(value, now) ? "active" : "unknown";
  if (value.status === "configured" || value.status === "starting") return value.status;
  return "unknown";
}

function isRecentObservation(checkedAt: string, now: number): boolean {
  const checked = Date.parse(checkedAt);
  return Number.isFinite(checked) && checked <= now && now - checked < 30_000;
}

function hasFreshEvidence(value: ExecutionObservation, now: number): boolean {
  const observed = Date.parse(value.evidence_at ?? "");
  const until = Date.parse(value.valid_until ?? "");
  return (
    value.reason === "" &&
    /^[a-z0-9._-]{1,64}$/.test(value.mechanism ?? "") &&
    Number.isFinite(observed) &&
    observed <= Date.parse(value.checked_at) &&
    until === observed + 30_000 &&
    now < until
  );
}

export function isExecutionObservationBound(
  value: ExecutionObservation | null | undefined,
  snapshot: ExecutionSnapshot | null,
): value is ExecutionObservation {
  if (!value || !snapshot?.sandbox || value.schema_version !== 1) return false;
  const identityMatches =
    value.session_id === snapshot.session_id &&
    value.task_id === snapshot.task_id &&
    value.attempt_id === snapshot.attempt_id &&
    value.attempt_id === snapshot.sandbox.attempt_id &&
    value.snapshot_sequence === snapshot.sequence &&
    value.generation === snapshot.sandbox.generation;
  return (
    identityMatches &&
    [value.observer_generation, value.revision, value.attempt_version].every(
      (number) => Number.isSafeInteger(number) && number > 0,
    )
  );
}
