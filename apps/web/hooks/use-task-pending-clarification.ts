import { useAppStore } from "@/components/state-provider";
import type { TaskPendingAction } from "@/lib/types/http";
import { hasPendingClarification } from "@/lib/utils/pending-clarification";

type PendingActionFallback = {
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
};

export function useTaskPendingClarification(
  primarySessionId: string | null | undefined,
  fallback?: PendingActionFallback,
): boolean {
  return useAppStore((state) => {
    if (!primarySessionId) return false;
    const messages = state.messages.bySession[primarySessionId];
    if (messages !== undefined) return hasPendingClarification(messages);
    return (
      fallback?.primarySessionState === "WAITING_FOR_INPUT" &&
      fallback.primarySessionPendingAction === "clarification"
    );
  });
}
