import type { ToolPayloadRetentionStatus } from "@/lib/types/tool-payload-retention";

export type BackupPreparationCorrelation = {
  activeRevision: number | null;
  settledRevision: number;
};

export type BackupPreparationObservation = {
  correlation: BackupPreparationCorrelation;
  shouldInvalidate: boolean;
};

const TERMINAL_STATES = new Set<ToolPayloadRetentionStatus["preparation"]["state"]>([
  "ready",
  "failed",
]);
const ACTIVE_STATES = new Set<ToolPayloadRetentionStatus["preparation"]["state"]>([
  "pending",
  "running",
]);

function isTerminal(state: ToolPayloadRetentionStatus["preparation"]["state"]) {
  return TERMINAL_STATES.has(state);
}

function settleRevision(
  correlation: BackupPreparationCorrelation,
  revision: number,
): BackupPreparationCorrelation {
  return {
    activeRevision: null,
    settledRevision: Math.max(correlation.settledRevision, revision),
  };
}

function isObservedAttempt(status: ToolPayloadRetentionStatus, saveCandidateRevision?: number) {
  const { revision } = status.policy;
  const { state, choice } = status.preparation;
  if (choice !== "backup") return false;
  if (saveCandidateRevision === revision) {
    return ACTIVE_STATES.has(state) || isTerminal(state);
  }
  return saveCandidateRevision === undefined && ACTIVE_STATES.has(state);
}

function isAttemptExit(status: ToolPayloadRetentionStatus) {
  const { state, choice } = status.preparation;
  return isTerminal(state) || state === "none" || choice !== "backup";
}

export function observeBackupPreparation(
  current: BackupPreparationCorrelation,
  status: ToolPayloadRetentionStatus,
  saveCandidateRevision?: number,
): BackupPreparationObservation {
  const revision = status.policy.revision;
  const activeRevision = current.activeRevision;
  if (activeRevision !== null && revision < activeRevision) {
    return { correlation: current, shouldInvalidate: false };
  }

  let correlation = current;
  let shouldInvalidate = false;
  if (activeRevision !== null) {
    const replaced = revision > activeRevision;
    const exited = revision === activeRevision && isAttemptExit(status);
    if (replaced || exited) {
      correlation = settleRevision(current, activeRevision);
      shouldInvalidate = true;
    }
  }

  if (
    correlation.activeRevision === null &&
    revision > correlation.settledRevision &&
    isObservedAttempt(status, saveCandidateRevision)
  ) {
    if (isTerminal(status.preparation.state)) {
      correlation = settleRevision(correlation, revision);
      shouldInvalidate = true;
    } else {
      correlation = { ...correlation, activeRevision: revision };
    }
  }

  return { correlation, shouldInvalidate };
}
