import { isExecutionObservationBound } from "./execution-status";
import { parseTurnTimestamp } from "./state/slices/session/turn-actions";
import type { ConnectionStatus } from "./types/connection";
import type {
  ExecutionFacts,
  ExecutionSnapshot,
  ExecutionObservation,
  LaunchCheckReceipt,
} from "./types/execution";
import type { TaskSession } from "./types/http";

type ExecutionSession = Pick<
  TaskSession,
  "id" | "task_id" | "execution_snapshot" | "execution_observation"
>;

export function unavailableExecutionFacts(): ExecutionFacts {
  return {
    schema_version: 1,
    process: { state: "unknown" },
    launch_checks: { state: "unavailable" },
    runtime: { state: "unsupported" },
    audit: { integrity: "unchecked", finalization: "unknown", signer: "unchecked" },
  };
}

export function getExecutionFacts(
  session: ExecutionSession,
  connection: ConnectionStatus,
  now: number,
): ExecutionFacts | null {
  const snapshot = session.execution_snapshot;
  const observation = session.execution_observation;
  if (
    snapshot?.session_id !== session.id ||
    snapshot.task_id !== session.task_id ||
    !isExecutionObservationBound(observation, snapshot)
  )
    return null;
  const facts = observation.facts;
  if (facts?.schema_version !== 1) return unavailableExecutionFacts();
  const result = {
    ...facts,
    process: { ...facts.process },
    runtime: { ...facts.runtime },
    audit: getCurrentAuditFacts(facts.audit, observation, connection, now),
  };
  if (
    facts.process.state === "running" &&
    !processIsFresh(facts.process, observation, connection, now)
  ) {
    result.process.state = "unknown";
    result.process.valid_until = undefined;
  }
  if (!validLaunchReceipt(facts.launch_checks.receipt, snapshot, now))
    result.launch_checks = { state: "unavailable" };
  if (connection !== "connected" && facts.runtime.state === "reported")
    result.runtime.state = "unavailable";
  return result;
}

function getCurrentAuditFacts(
  captured: ExecutionFacts["audit"],
  observation: ExecutionObservation,
  connection: ConnectionStatus,
  now: number,
): ExecutionFacts["audit"] {
  const audit = { ...captured };
  const checked = timestampMillis(observation.checked_at);
  if (
    audit.signer === "trusted" &&
    (connection !== "connected" || checked === null || checked > now)
  )
    audit.signer = "untrusted";
  return audit;
}

function validLaunchReceipt(
  receipt: LaunchCheckReceipt | undefined,
  snapshot: ExecutionSnapshot,
  now: number,
): boolean {
  if (!receipt || !snapshot.sandbox) return false;
  const checked = timestampMillis(receipt.checked_at);
  return (
    checked !== null &&
    checked <= now &&
    receipt.policy_digest === snapshot.sandbox.policy_digest &&
    receipt.package_digest === snapshot.sandbox.package_digest &&
    receipt.runtime_digest === snapshot.sandbox.runtime_digest &&
    receipt.checks.join(",") ===
      "policy_ceiling,runtime_checksum,private_artifacts,process_containment"
  );
}

export function executionObservationDeadline(
  session: TaskSession | undefined,
  now = Date.now(),
): number | null {
  const value = session?.execution_observation;
  if (!value) return null;
  const candidates: number[] = [];
  const checked = timestampMillis(value.checked_at);
  if (["active", "starting", "configured"].includes(value.status) && checked !== null)
    candidates.push(checked + 30_000);
  if (value.status === "active") {
    const until = timestampMillis(value.valid_until);
    if (until !== null) candidates.push(until);
  }
  if (value.facts?.process.state === "running") {
    const until = timestampMillis(value.facts.process.valid_until);
    if (until !== null) candidates.push(until);
  }
  const future = candidates.filter((deadline) => deadline > now);
  return future.length ? Math.min(...future) : null;
}

function timestampMillis(value: string | undefined): number | null {
  const timestamp = parseTurnTimestamp(value);
  return timestamp === null ? null : Number(timestamp / BigInt(1_000_000));
}

function processIsFresh(
  process: ExecutionFacts["process"],
  observation: ExecutionObservation,
  connection: ConnectionStatus,
  now: number,
): boolean {
  if (connection !== "connected" || observation.status === "stopped") return false;
  const checked = timestampMillis(process.checked_at);
  const until = timestampMillis(process.valid_until);
  const enclosing = timestampMillis(observation.checked_at);
  return (
    checked !== null &&
    until !== null &&
    enclosing !== null &&
    checked <= enclosing &&
    enclosing <= now &&
    until === checked + 30_000 &&
    now < until
  );
}
