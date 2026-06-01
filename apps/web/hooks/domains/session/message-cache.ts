import type { QueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { MessagesData } from "@/lib/query/query-options/session";
import type { Message } from "@/lib/types/http";

/**
 * Prepend older messages into the TanStack Query cache at
 * qk.session.messages(sid), deduping by id and updating hasMore/oldestCursor.
 *
 * Shared by `useLazyLoadMessages` (scroll-up pagination) and
 * `useSessionMessages` (auto-backfill) so the cache-merge semantics stay
 * identical in both paths.
 */
export function prependMessagesIntoCache(
  queryClient: QueryClient,
  sessionId: string,
  older: Message[],
  meta: { hasMore: boolean; oldestCursor: string | null },
): void {
  queryClient.setQueryData<MessagesData>(qk.session.messages(sessionId), (prev) => {
    const existing = prev?.messages ?? [];
    const existingIds = new Set(existing.map((m) => m.id));
    const merged = [...older.filter((m) => !existingIds.has(m.id)), ...existing];
    return {
      messages: merged,
      hasMore: meta.hasMore,
      oldestCursor: meta.oldestCursor ?? prev?.oldestCursor ?? null,
    };
  });
}
