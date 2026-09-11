import type { TaskStatusSummaryActiveError } from "./types/task-status-summary";
import { lastAgentErrorStamp, readLastAgentError } from "./session-last-agent-error";
import type {
  ResumptionState,
  SessionRecoveryFailure,
} from "@/hooks/domains/session/use-session-resumption";

/** Automatic recovery state shared with the session-owned bootstrap card. */
export type SessionRecoveryOwner = {
  resumptionState: ResumptionState;
  error: string | null;
  notice: string | null;
  recoveryFailure: SessionRecoveryFailure | null;
  resumeSession: () => Promise<boolean>;
};

export function isSessionRecoveryBusy(state: ResumptionState): boolean {
  return state === "checking" || state === "resuming";
}

/** Bootstrap failures are session-owned and must have a stable identity. */
export function isBootstrapSessionRecoveryError(
  error: TaskStatusSummaryActiveError | null | undefined,
): boolean {
  return error?.phase === "bootstrap" && Boolean(error.session_id && error.stamp);
}

function sessionMetadataRecoveryError(
  sessionId: string,
  metadata: Record<string, unknown> | null | undefined,
): TaskStatusSummaryActiveError | null {
  const lastError = readLastAgentError(metadata);
  if (!lastError || lastError.phase !== "bootstrap") return null;

  const error: TaskStatusSummaryActiveError = {
    session_id: sessionId,
    stamp: lastAgentErrorStamp(lastError),
    occurred_at: lastError.occurredAt ?? "",
    preview: lastError.message,
  };
  if (lastError.taskRepositoryId) error.task_repository_id = lastError.taskRepositoryId;
  if (lastError.details) error.details = lastError.details;
  if (lastError.code) error.category = lastError.code;
  if (lastError.executionId ?? lastError.agentExecutionId) {
    error.execution_id = lastError.executionId ?? lastError.agentExecutionId;
  }
  if (lastError.phase) error.phase = lastError.phase;
  if (lastError.attemptId) error.attempt_id = lastError.attemptId;
  if (lastError.causes) error.causes = lastError.causes;
  if (lastError.recoveryActions) error.recovery_actions = lastError.recoveryActions;
  return error;
}

/** Selects the durable bootstrap failure owned by the currently rendered session. */
export function selectSessionRecoveryError(
  activeError: TaskStatusSummaryActiveError | null | undefined,
  sessionId: string | null | undefined,
  sessionMetadata?: Record<string, unknown> | null,
): TaskStatusSummaryActiveError | null {
  if (!sessionId) return null;
  const persistedError = sessionMetadataRecoveryError(sessionId, sessionMetadata);
  if (persistedError) return persistedError;
  if (!isBootstrapSessionRecoveryError(activeError) || !activeError) return null;
  return activeError.session_id === sessionId ? activeError : null;
}

export function ownsSessionRecoveryChat(
  activeError: TaskStatusSummaryActiveError | null | undefined,
  sessionId: string | null | undefined,
  sessionMetadata?: Record<string, unknown> | null,
): boolean {
  return selectSessionRecoveryError(activeError, sessionId, sessionMetadata) !== null;
}
