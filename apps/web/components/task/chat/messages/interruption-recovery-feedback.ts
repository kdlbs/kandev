type RecoveryMetadata = {
  retrying?: boolean;
  recovery_mode?: string;
  recovery_phase?: string;
  recovery_disposition?: string;
  recovery_reason?: string;
  attempts_started?: number;
};

export function continuationPhase(metadata: RecoveryMetadata) {
  if (!metadata.retrying || metadata.recovery_mode !== "continue") return undefined;
  const phase = metadata.recovery_phase;
  return phase === "waiting" || phase === "reconnecting" || phase === "continuing"
    ? phase
    : undefined;
}

export function interruptionRecoveryKey(metadata: RecoveryMetadata) {
  if (metadata.recovery_disposition === "cancelled") return "chat:providerRecoveryCancelledBody";
  const count = metadata.attempts_started;
  if (metadata.recovery_disposition === "exhausted" && Number.isInteger(count) && count! > 0)
    return "chat:providerRecoveryExhaustedBody";
  switch (metadata.recovery_reason) {
    case "disabled":
      return "chat:providerRecoveryDisabledBody";
    case "unsafe_work":
      return "chat:providerRecoveryUnsafeBody";
    case "unsupported_restore":
      return "chat:providerRecoveryUnsupportedBody";
    case "missing_evidence":
      return "chat:providerRecoveryEvidenceBody";
  }
  return "chat:providerManualRecoveryBody";
}

export function retryNoticeVisible(state: string | undefined, metadata: RecoveryMetadata) {
  if (!metadata.retrying || state === "COMPLETED" || state === "CANCELLED" || state === "FAILED")
    return false;
  return Boolean(continuationPhase(metadata)) || (state !== "RUNNING" && state !== "STARTING");
}
