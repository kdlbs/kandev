export type AgentDeliveryRecoveryPhase =
  | "reconnecting"
  | "uncertain"
  | "recovered"
  | "settled"
  | "continued"
  | "restored";

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
  reconstruction?: { processIdentityKnown: boolean };
  historyGap?: AgentDeliveryHistoryGap;
};

export type AgentDeliveryHistoryGap = {
  firstSequence: number;
  lastSequence: number;
  receivedSequence: number;
  projectedSequence: number;
};

function readHistoryGap(raw: unknown): AgentDeliveryHistoryGap | undefined {
  if (
    !isRecord(raw) ||
    !isPositiveSafeInteger(raw.first_sequence) ||
    !isPositiveSafeInteger(raw.last_sequence) ||
    !isNonNegativeSafeInteger(raw.received_sequence) ||
    !isNonNegativeSafeInteger(raw.projected_sequence) ||
    raw.first_sequence > raw.last_sequence ||
    raw.received_sequence !== raw.projected_sequence ||
    raw.first_sequence !== raw.received_sequence + 1
  )
    return undefined;
  return {
    firstSequence: raw.first_sequence,
    lastSequence: raw.last_sequence,
    receivedSequence: raw.received_sequence,
    projectedSequence: raw.projected_sequence,
  };
}

export function readAgentDeliveryHistoryGaps(
  metadata: Record<string, unknown> | null | undefined,
): (AgentDeliveryHistoryGap & { streamId: string })[] {
  const raw = metadata?.agent_delivery_history_gaps;
  if (!isRecord(raw)) return [];
  return Object.entries(raw).flatMap(([streamId, record]) => {
    if (
      !isRecord(record) ||
      record.stream_id !== streamId ||
      !isNonEmptyString(streamId) ||
      !isNonEmptyString(record.incarnation_id) ||
      !isPositiveSafeInteger(record.harness_generation) ||
      !isNonEmptyString(record.submission_id) ||
      !isNonEmptyString(record.payload_hash) ||
      !isNonEmptyString(record.native_session_id)
    )
      return [];
    const gap = readHistoryGap(record.gap);
    return gap ? [{ streamId, ...gap }] : [];
  });
}

type RecoveryIdentity = {
  session_id: string;
  agent_execution_id: string;
  submission_id: string;
  stream_id: string;
  incarnation_id: string;
  harness_generation: number;
  prompt_generation: number;
  reconstruction?: { process_identity_known?: boolean };
};

export function readAgentDeliveryRecovery(
  metadata: Record<string, unknown> | null | undefined,
): AgentDeliveryRecovery | null {
  const raw = metadata?.agent_delivery_recovery;
  if (!isRecord(raw)) return null;
  const record = raw as Record<string, unknown>;
  if (!isAgentDeliveryRecoveryPhase(record.phase) || !isPositiveSafeInteger(record.revision))
    return null;
  if (!hasPersistedRecoveryIdentity(record)) return null;
  const reconstruction = isRecord(record.reconstruction)
    ? { processIdentityKnown: record.reconstruction.process_identity_known === true }
    : undefined;
  const historyGap = readHistoryGap(record.history_gap);
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
    ...(reconstruction ? { reconstruction } : {}),
    ...(historyGap ? { historyGap } : {}),
  };
}

export function isAgentDeliveryRecoveryContinuationEligible(
  recovery: AgentDeliveryRecovery,
): boolean {
  return (
    recovery.agentExecutionId !== "" &&
    recovery.promptGeneration > 0 &&
    (!recovery.reconstruction || recovery.reconstruction.processIdentityKnown)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isAgentDeliveryRecoveryPhase(value: unknown): value is AgentDeliveryRecoveryPhase {
  return (
    value === "reconnecting" ||
    value === "uncertain" ||
    value === "recovered" ||
    value === "settled" ||
    value === "continued" ||
    value === "restored"
  );
}

function hasPersistedRecoveryIdentity(
  record: Record<string, unknown>,
): record is Record<string, unknown> & RecoveryIdentity {
  return (
    isNonEmptyString(record.session_id) &&
    typeof record.agent_execution_id === "string" &&
    isNonEmptyString(record.submission_id) &&
    isNonEmptyString(record.stream_id) &&
    isNonEmptyString(record.incarnation_id) &&
    isPositiveSafeInteger(record.harness_generation) &&
    isNonNegativeSafeInteger(record.prompt_generation)
  );
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 1;
}

function isNonNegativeSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}
