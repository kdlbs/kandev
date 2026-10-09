import type { Message, TaskSessionState } from "@/lib/types/http";
import {
  lastAgentErrorStamp,
  readLastAgentError,
  readLastAgentErrorIncludingDismissed,
  type LastAgentError,
} from "@/lib/session-last-agent-error";
import { legacyRecoveryMessageMatchesError } from "@/lib/session-recovery-presentation";
export function isSessionActive(state?: TaskSessionState) {
  return state === "RUNNING" || state === "STARTING" || state === "COMPLETED";
}

export function currentSessionRecoveryError(sessionMetadata: Record<string, unknown> | null) {
  if (!sessionMetadata) return null;
  return readLastAgentError(sessionMetadata);
}

export function isCurrentRecoveryMessage(
  isRecoveryMessage: boolean,
  messageRecoveryStamp: string | undefined,
  currentRecoveryError: LastAgentError | null,
  comment: Message,
  sessionMetadata: Record<string, unknown> | null | undefined,
) {
  if (!isRecoveryMessage) return true;
  if (messageRecoveryStamp) {
    const latestError = readLastAgentErrorIncludingDismissed(sessionMetadata);
    if (!latestError || latestError.dismissedAt) return false;
    return lastAgentErrorStamp(latestError) === messageRecoveryStamp;
  }
  if (!currentRecoveryError) return true;
  return legacyRecoveryMessageMatchesError(
    comment.content,
    comment.created_at,
    currentRecoveryError,
  );
}

export function shouldShowRecoveryActions({
  isRecoveryMessage,
  isCurrentRecovery,
  recoveryResolvedDurably,
  agentRebooted,
  recoveryRequested,
  recoveryFailedAgain,
  sessionState,
}: {
  isRecoveryMessage: boolean;
  isCurrentRecovery: boolean;
  recoveryResolvedDurably: boolean;
  agentRebooted: boolean;
  recoveryRequested: boolean;
  recoveryFailedAgain: boolean;
  sessionState?: TaskSessionState;
}) {
  if (!isRecoveryMessage) return true;
  if (!isCurrentRecovery || recoveryResolvedDurably || agentRebooted) return false;
  if (recoveryRequested && !recoveryFailedAgain) return false;
  return !isSessionActive(sessionState);
}
