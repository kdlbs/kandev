import { useTaskPendingInput, type PendingInputFallback } from "@/hooks/use-task-pending-input";

/**
 * Backwards-compatible wrapper: the boolean "does the primary session have a
 * pending clarification" reading. New surfaces should prefer
 * {@link useTaskPendingInput}, which also exposes the pending-permission flag.
 */
export function useTaskPendingClarification(
  primarySessionId: string | null | undefined,
  fallback?: PendingInputFallback,
): boolean {
  return useTaskPendingInput(primarySessionId, fallback).clarification;
}
