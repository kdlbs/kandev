import type { TaskStatusSummaryActiveError } from "./types/task-status-summary";
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

/** Selects the durable bootstrap failure owned by the currently rendered session. */
export function selectSessionRecoveryError(
  activeError: TaskStatusSummaryActiveError | null | undefined,
  sessionId: string | null | undefined,
): TaskStatusSummaryActiveError | null {
  if (!sessionId || !isBootstrapSessionRecoveryError(activeError) || !activeError) return null;
  return activeError.session_id === sessionId ? activeError : null;
}

export function ownsSessionRecoveryChat(
  activeError: TaskStatusSummaryActiveError | null | undefined,
  sessionId: string | null | undefined,
): boolean {
  return selectSessionRecoveryError(activeError, sessionId) !== null;
}
