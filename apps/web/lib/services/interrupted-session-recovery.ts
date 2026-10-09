import { getWebSocketClient } from "@/lib/ws/connection";
import {
  requestSessionRecover,
  sessionDeliveryRecoveryResponse,
  type InterruptedSessionResumeRequest,
  type SessionDeliveryRecoveryResponse,
} from "./session-recovery-service";

export type InterruptedRecoveryCheckpoint = {
  taskId: string;
  sessionId: string;
  request: InterruptedSessionResumeRequest;
  result?: SessionDeliveryRecoveryResponse;
};
type ContinueOptions = {
  taskId: string;
  sessionId: string;
  observed: SessionDeliveryRecoveryResponse;
  instruction: string;
  acknowledged: boolean;
  failureMessage: string;
};
const pending = new Map<string, Promise<InterruptedRecoveryCheckpoint>>();
const checkpointKey = (taskId: string, sessionId: string) =>
  `kandev:interrupted-recovery:${taskId}:${sessionId}`;

export function readInterruptedCheckpoint(
  taskId: string,
  sessionId: string,
): InterruptedRecoveryCheckpoint | null {
  try {
    const raw = window.sessionStorage.getItem(checkpointKey(taskId, sessionId));
    if (!raw) return null;
    const value = JSON.parse(raw) as InterruptedRecoveryCheckpoint;
    if (
      value.taskId !== taskId ||
      value.sessionId !== sessionId ||
      !value.request?.acknowledge_interruption ||
      typeof value.request.instruction !== "string" ||
      typeof value.request.idempotency_key !== "string" ||
      !sessionDeliveryRecoveryResponse({
        task_id: taskId,
        session_id: sessionId,
        outcome: "uncertain",
        recovery_revision: value.request.recovery_revision,
        recovery_identity: value.request.recovery_identity,
        allowed_actions: ["resume_interrupted"],
      })
    )
      return null;
    if (value.result && !sessionDeliveryRecoveryResponse(value.result)) return null;
    return value;
  } catch {
    return null;
  }
}

function persistCheckpoint(value: InterruptedRecoveryCheckpoint, failureMessage: string) {
  try {
    const key = checkpointKey(value.taskId, value.sessionId);
    const encoded = JSON.stringify(value);
    window.sessionStorage.setItem(key, encoded);
    if (window.sessionStorage.getItem(key) !== encoded) throw new Error(failureMessage);
  } catch {
    throw new Error(failureMessage);
  }
}

export function continueInterruptedSession(options: ContinueOptions) {
  const key = checkpointKey(options.taskId, options.sessionId);
  const existing = pending.get(key);
  if (existing) return existing;
  const operation = runInterruptedContinuation(options).finally(() => pending.delete(key));
  pending.set(key, operation);
  return operation;
}

async function prepareCheckpoint(options: ContinueOptions): Promise<InterruptedRecoveryCheckpoint> {
  const { taskId, sessionId, observed, instruction, acknowledged, failureMessage } = options;
  if (!validContinuationInstruction(instruction, acknowledged)) {
    throw new Error(failureMessage);
  }
  const previous = readInterruptedCheckpoint(taskId, sessionId);
  if (
    previous &&
    previous.request.recovery_revision === observed.recovery_revision &&
    JSON.stringify(previous.request.recovery_identity) ===
      JSON.stringify(observed.recovery_identity)
  ) {
    if (previous.request.instruction !== instruction) throw new Error(failureMessage);
    return previous;
  }
  const current = await requestSessionRecover({
    taskId,
    sessionId,
    action: "retry_connection",
    failureMessage,
  });
  if (current && current.outcome !== "uncertain") {
    window.sessionStorage.setItem(
      `${checkpointKey(taskId, sessionId)}:result`,
      JSON.stringify(current),
    );
    throw Object.assign(new Error(failureMessage), { deliveryRecovery: current });
  }
  if (
    !current ||
    current.outcome !== "uncertain" ||
    !current.allowed_actions?.includes("resume_interrupted") ||
    !current.recovery_identity ||
    current.recovery_revision !== observed.recovery_revision ||
    JSON.stringify(current.recovery_identity) !== JSON.stringify(observed.recovery_identity)
  ) {
    throw new Error(failureMessage);
  }
  const value: InterruptedRecoveryCheckpoint = {
    taskId,
    sessionId,
    request: {
      acknowledge_interruption: true,
      recovery_revision: current.recovery_revision,
      recovery_identity: current.recovery_identity,
      instruction,
      idempotency_key: `interrupted:${current.recovery_identity.submission_id}:${current.recovery_revision}`,
    },
  };
  persistCheckpoint(value, failureMessage);
  return value;
}

async function runInterruptedContinuation(options: ContinueOptions) {
  const checkpoint = await prepareCheckpoint(options);
  const client = getWebSocketClient();
  if (!client) throw new Error(options.failureMessage);
  // Each item uses the bounded batch endpoint so progress and saved results
  // survive partial failures without repeating an accepted instruction.
  const response = await client.request<{ results?: unknown[]; completed?: number }>(
    "session.recover_batch",
    { items: [{ task_id: options.taskId, session_id: options.sessionId, ...checkpoint.request }] },
    150_000,
  );
  const result =
    response.completed === 1 && response.results?.length === 1
      ? sessionDeliveryRecoveryResponse(response.results[0])
      : null;
  if (!result || result.task_id !== options.taskId || result.session_id !== options.sessionId) {
    throw new Error(options.failureMessage);
  }
  const completed = { ...checkpoint, result };
  persistCheckpoint(completed, options.failureMessage);
  return completed;
}

export function readInterruptedRecoveryResult(
  taskId: string,
  sessionId: string,
): SessionDeliveryRecoveryResponse | null {
  const checkpoint = readInterruptedCheckpoint(taskId, sessionId);
  if (checkpoint?.result) return checkpoint.result;
  try {
    const result = sessionDeliveryRecoveryResponse(
      JSON.parse(
        window.sessionStorage.getItem(`${checkpointKey(taskId, sessionId)}:result`) ?? "null",
      ),
    );
    return result?.task_id === taskId && result.session_id === sessionId ? result : null;
  } catch {
    return null;
  }
}

function validContinuationInstruction(instruction: string, acknowledged: boolean) {
  return (
    acknowledged && !!instruction.trim() && new TextEncoder().encode(instruction).length <= 32_768
  );
}
