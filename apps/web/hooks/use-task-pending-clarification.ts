import { useAppStore } from "@/components/state-provider";
import { hasPendingClarificationForSession } from "@/lib/utils/pending-clarification";

export function useTaskPendingClarification(primarySessionId: string | null | undefined): boolean {
  return useAppStore((state) =>
    hasPendingClarificationForSession(state.messages.bySession, primarySessionId),
  );
}
