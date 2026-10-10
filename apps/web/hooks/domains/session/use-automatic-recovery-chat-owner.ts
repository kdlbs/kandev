import { useAppStore } from "@/components/state-provider";
import { selectActiveSessionRecovery } from "@/lib/active-session-recovery";
import {
  matchingAutomaticRecovery,
  type SessionRecoveryOwner,
} from "@/lib/session-recovery-presentation";
import { statusSummaryTaskError } from "@/lib/task-status-summary";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

/** Uses the composer's eligibility before transferring outer request feedback. */
export function useAutomaticRecoveryChatOwner({
  taskId,
  sessionId,
  recovery,
  summary,
  disabled = false,
}: {
  taskId: string | null | undefined;
  sessionId: string | null;
  recovery: SessionRecoveryOwner;
  summary?: TaskStatusSummary | null;
  disabled?: boolean;
}): boolean {
  return useAppStore((state) => {
    if (disabled || !sessionId || statusSummaryTaskError(summary)) return false;
    const session = state.taskSessions.items[sessionId];
    if (
      !session ||
      session.is_passthrough ||
      !matchingAutomaticRecovery(recovery, taskId, sessionId)
    )
      return false;
    return (
      selectActiveSessionRecovery(
        session,
        state.messages.bySession[sessionId] ?? [],
        summary?.active_error,
      ) !== null
    );
  });
}
