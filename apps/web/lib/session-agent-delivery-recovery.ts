export type AgentDeliveryRecoveryPhase = "reconnecting" | "uncertain" | "recovered" | "settled";

export type AgentDeliveryRecovery = {
  phase: AgentDeliveryRecoveryPhase;
  revision: number;
  sessionId: string;
  agentExecutionId: string;
  submissionId: string;
  streamId: string;
  incarnationId: string;
  harnessGeneration: number;
  promptGeneration: number;
  message?: string;
};

type RecoveryIdentity = {
  session_id: string;
  agent_execution_id: string;
  submission_id: string;
  stream_id: string;
  incarnation_id: string;
  harness_generation: number;
  prompt_generation: number;
};

export function readAgentDeliveryRecovery(
  metadata: Record<string, unknown> | null | undefined,
): AgentDeliveryRecovery | null {
  const raw = metadata?.agent_delivery_recovery;
  if (!isRecord(raw)) return null;
  const record = raw as Record<string, unknown>;
  if (!isAgentDeliveryRecoveryPhase(record.phase) || !isPositiveSafeInteger(record.revision))
    return null;
  if (!hasRecoveryIdentity(record)) return null;
  return {
    phase: record.phase,
    revision: record.revision,
    sessionId: record.session_id,
    agentExecutionId: record.agent_execution_id,
    submissionId: record.submission_id,
    streamId: record.stream_id,
    incarnationId: record.incarnation_id,
    harnessGeneration: record.harness_generation,
    promptGeneration: record.prompt_generation,
    ...(typeof record.message === "string" ? { message: record.message } : {}),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isAgentDeliveryRecoveryPhase(value: unknown): value is AgentDeliveryRecoveryPhase {
  return (
    value === "reconnecting" ||
    value === "uncertain" ||
    value === "recovered" ||
    value === "settled"
  );
}

function hasRecoveryIdentity(
  record: Record<string, unknown>,
): record is Record<string, unknown> & RecoveryIdentity {
  return (
    isNonEmptyString(record.session_id) &&
    isNonEmptyString(record.agent_execution_id) &&
    isNonEmptyString(record.submission_id) &&
    isNonEmptyString(record.stream_id) &&
    isNonEmptyString(record.incarnation_id) &&
    isPositiveSafeInteger(record.harness_generation) &&
    isPositiveSafeInteger(record.prompt_generation)
  );
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 1;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}
