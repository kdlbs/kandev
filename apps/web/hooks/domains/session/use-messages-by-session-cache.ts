import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import type { Message } from "@/lib/types/http";
import { sessionMessagesQueryOptions } from "@/lib/query/query-options/session";

/**
 * Stabilize a derived array of primary session IDs so the reference only
 * changes when the actual contents change. Prevents downstream effects/queries
 * (bulk WS subscribe, the message-cache read below) from tearing down and
 * recreating on every kanban snapshot update.
 */
export function useStablePrimarySessionIds(
  allTasks: Array<{ primarySessionId?: string | null }>,
): string[] {
  const key = useMemo(
    () =>
      allTasks
        .map((t) => t.primarySessionId)
        .filter((id): id is string => id != null)
        .join("\0"),
    [allTasks],
  );
  return useMemo(() => (key ? key.split("\0") : []), [key]);
}

/**
 * Build a `{ sessionId -> messages }` map for the sidebar's pending-permission /
 * pending-clarification indicators by reading the TanStack Query message cache
 * (`qk.session.messages(sid)`) — the canonical message store after the message
 * migration.
 *
 * Why not Zustand `messages.bySession`: the migrated `useSessionMessages`
 * fetches a session's history window into TQ only (not Zustand). A
 * `permission_request` that arrived in that fetched window — rather than via a
 * live WS append — is therefore present in TQ but absent from the Zustand
 * mirror. Deriving the sidebar indicator from Zustand missed the amber
 * pending-permission icon for exactly that case (regression caught by the
 * `chat/permission-approval` "Sidebar pending permission" e2e). The chat reads
 * the same TQ key, so the two surfaces now agree by construction.
 *
 * `enabled: false` — we only OBSERVE the cache. The sidebar's bulk-subscribe
 * effect plus the WS bridge populate these keys; issuing a fetch per sidebar
 * session here would be wasteful and unnecessary. `setQueryData` from the
 * bridge still notifies these disabled observers.
 */
export function useMessagesBySessionFromCache(
  sessionIds: string[],
): Record<string, readonly Message[] | undefined> {
  return useQueries({
    queries: sessionIds.map((sid) => ({
      ...sessionMessagesQueryOptions(sid),
      enabled: false,
    })),
    combine: (results): Record<string, readonly Message[] | undefined> => {
      const map: Record<string, readonly Message[] | undefined> = {};
      results.forEach((result, index) => {
        const msgs = result.data?.messages;
        if (msgs) map[sessionIds[index]] = msgs;
      });
      return map;
    },
  });
}
