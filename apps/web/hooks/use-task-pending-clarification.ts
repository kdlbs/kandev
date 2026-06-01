import { useQuery } from "@tanstack/react-query";
import { hasPendingClarification } from "@/lib/utils/pending-clarification";
import { sessionMessagesQueryOptions } from "@/lib/query/query-options/session";

export function useTaskPendingClarification(primarySessionId: string | null | undefined): boolean {
  // Read messages from the TanStack Query cache (canonical post-migration), not
  // the Zustand mirror — the latter no longer holds a session's fetched history,
  // so a clarification_request that arrived in the fetched window (rather than a
  // live WS append) would be missed. Observe-only (`enabled: false`): the cache
  // is populated by the active session's query plus the WS bridge; we must not
  // issue a fetch per card here.
  const { data } = useQuery({
    ...sessionMessagesQueryOptions(primarySessionId ?? ""),
    enabled: false,
  });
  if (!primarySessionId) return false;
  return hasPendingClarification(data?.messages);
}
