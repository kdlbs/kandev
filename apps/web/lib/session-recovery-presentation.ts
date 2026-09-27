import type { TaskStatusSummaryActiveError } from "./types/task-status-summary";
import { lastAgentErrorStamp, readLastAgentError } from "./session-last-agent-error";
import type { LastAgentError } from "./session-last-agent-error";
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
    scope: "session",
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
  if (activeError.scope === "task") return null;
  return activeError.session_id === sessionId ? activeError : null;
}

export function ownsSessionRecoveryChat(
  activeError: TaskStatusSummaryActiveError | null | undefined,
  sessionId: string | null | undefined,
  sessionMetadata?: Record<string, unknown> | null,
): boolean {
  return selectSessionRecoveryError(activeError, sessionId, sessionMetadata) !== null;
}

/** Matches a pre-stamp recovery row to the active session error safely. */
export function legacyRecoveryMessageMatchesError(
  contentValue: string,
  createdAt: string | undefined,
  currentError: LastAgentError,
): boolean {
  const content = contentValue.trim();
  const message = currentError.message.trim();
  if (!content || !message) return false;

  const contentMatches =
    content === message ||
    content === `Agent encountered an error: ${message}` ||
    content === `Agent startup failed: ${message}` ||
    ((content.startsWith("Agent encountered an error:") ||
      content.startsWith("Agent startup failed:")) &&
      content.endsWith(message));
  if (!contentMatches) return false;

  // Metadata is written before the transcript row. Exclude an older legacy
  // row with the same text when both sides carry usable timestamps.
  if (!currentError.occurredAt) return true;
  const occurredAt = Date.parse(currentError.occurredAt);
  const messageCreatedAt = Date.parse(createdAt ?? "");
  return (
    Number.isNaN(occurredAt) || Number.isNaN(messageCreatedAt) || messageCreatedAt >= occurredAt
  );
}

export function hasSessionRecoveryMessage(
  messages: readonly { session_id?: string; metadata?: Record<string, unknown> | null }[],
  sessionId: string | null | undefined,
  stamp: string | null | undefined,
): boolean {
  if (!sessionId || !stamp) return false;
  return messages.some(
    (message) =>
      message.session_id === sessionId &&
      message.metadata?.scope !== "task" &&
      message.metadata?.recovery_actions === true &&
      (message.metadata?.error_stamp ?? message.metadata?.recovery_stamp) === stamp,
  );
}

export function sessionRecoveryOwnerId(
  failure: SessionRecoveryFailure | null | undefined,
): string | undefined {
  return failure?.outcome === "recovery_failed" && failure.workspaceAttemptId
    ? `session-recovery-owner-${failure.workspaceAttemptId}`
    : undefined;
}
