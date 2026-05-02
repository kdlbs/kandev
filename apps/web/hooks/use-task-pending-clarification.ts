import { useAppStore } from "@/components/state-provider";
import { hasPendingClarification } from "@/lib/utils/pending-clarification";

export function useTaskPendingClarification(primarySessionId: string | null | undefined): boolean {
  return useAppStore((state) =>
    primarySessionId ? hasPendingClarification(state.messages.bySession[primarySessionId]) : false,
  );
}
