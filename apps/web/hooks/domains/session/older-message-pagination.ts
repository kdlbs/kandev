import type { useAppStoreApi } from "@/components/state-provider";
import { listTaskSessionMessages } from "@/lib/api";
import { createDebugLogger } from "@/lib/debug/log";
import type { Message } from "@/lib/types/http";

const debug = createDebugLogger("messages:pagination");

export type OlderPageResult = {
  /** Rows returned by this page. */
  count: number;
  /** Rows actually prepended after deduplication and session-lifecycle checks. */
  prependedMessages: Message[];
  /** The page size this request used (first-request-wins per (session, cursor)). */
  effectiveLimit: number;
  /** Post-merge pagination metadata. */
  hasMore: boolean;
  oldestCursor: string | null;
};

type SessionMessageStore = ReturnType<typeof useAppStoreApi>;

type SessionFlights = {
  /** In-flight request count (all cursors) — drives the isLoadingMore lifecycle. */
  count: number;
  /** One promise per (session, cursor) so followers join, not duplicate. */
  byCursor: Map<string, Promise<OlderPageResult>>;
};

const inFlightBySession = new Map<string, SessionFlights>();

/**
 * Shared per-session older-message coordinator. All older-message consumers —
 * the panel sentinel, the native transcript sentinel, automatic backfill, and
 * the last-prompt/drain preloads — route through here so one (sessionId,
 * cursor) request serves every caller with the first caller's limit, no
 * duplicate request or merge can be issued for one cursor, and `isLoadingMore`
 * cannot be cleared mid-flight by another consumer.
 *
 * A follower joining an in-flight (session, cursor) receives the same promise
 * (and therefore the same effectiveLimit and merged result). Zero-row responses
 * retain the cursor so a later retry re-requests the same page. The
 * `isLoadingMore` metadata flag is owned here: raised for the session's first
 * in-flight request and cleared only when the last one settles, never touching
 * the initial/refetch `isLoading` flag.
 */
/**
 * Returns the in-flight older-page request for (sessionId, cursor), or null
 * when none is pending. Callers use this BEFORE their local loading guards so
 * a concurrent caller (panel sentinel during transcript loading or automatic
 * backfill) joins the existing promise instead of skipping or duplicating.
 */
export function joinOlderMessages(
  sessionId: string,
  cursor: string,
): Promise<OlderPageResult> | null {
  const flights = inFlightBySession.get(sessionId);
  return flights?.byCursor.get(cursor) ?? null;
}

export function requestOlderMessages(params: {
  sessionId: string;
  cursor: string;
  limit: number;
  store: SessionMessageStore;
}): Promise<OlderPageResult> {
  const { sessionId, cursor, limit, store } = params;
  let flights = inFlightBySession.get(sessionId);
  if (!flights) {
    flights = { count: 0, byCursor: new Map() };
    inFlightBySession.set(sessionId, flights);
  }
  const existing = flights.byCursor.get(cursor);
  if (existing) return existing;

  flights.count += 1;
  if (flights.count === 1) {
    store.getState().setMessagesMetadata(sessionId, { isLoadingMore: true });
  }
  const promise = performRequest(sessionId, cursor, limit, store).finally(() => {
    const current = inFlightBySession.get(sessionId);
    if (!current) return;
    current.count -= 1;
    current.byCursor.delete(cursor);
    if (current.count === 0) {
      // Only touch the store when the session still has message state: it
      // may have been deleted while the request was in flight, and clearing
      // the flag would recreate a purged meta entry.
      if (store.getState().messages?.bySession?.[sessionId] !== undefined) {
        store.getState().setMessagesMetadata(sessionId, { isLoadingMore: false });
      }
    }
    if (current.count === 0 && current.byCursor.size === 0) {
      inFlightBySession.delete(sessionId);
    }
  });
  flights.byCursor.set(cursor, promise);
  return promise;
}

async function performRequest(
  sessionId: string,
  cursor: string,
  limit: number,
  store: SessionMessageStore,
): Promise<OlderPageResult> {
  debug("older page: requesting", { sessionId, before: cursor, limit });
  const response = await listTaskSessionMessages(sessionId, {
    limit,
    before: cursor,
    sort: "desc",
  });
  const ordered = [...(response.messages ?? [])].reverse();
  // After reversing, ordered[0] is the oldest message in this batch. A
  // zero-row response retains the cursor so retries re-request the same page.
  const newOldestCursor = ordered[0]?.id ?? cursor;
  const hasMore = response.has_more ?? false;
  debug("older page: response", {
    sessionId,
    fetchedCount: ordered.length,
    responseHasMore: hasMore,
    newOldestId: newOldestCursor,
    newOldestCreatedAt: ordered[0]?.created_at ?? null,
  });
  // The session may have been deleted/archived while this request was in
  // flight: the store purge removes bySession/metaBySession entries, and an
  // unconditional prepend would resurrect orphaned conversation state. A
  // request can only legitimately be in flight when the session's message
  // state existed at start, so its absence now means it was purged.
  const existingMessages = store.getState().messages?.bySession?.[sessionId];
  const existingIds = new Set(existingMessages?.map((message) => message.id));
  const prependedMessages = existingMessages
    ? ordered.filter((message) => !existingIds.has(message.id))
    : [];
  if (existingMessages !== undefined) {
    store.getState().prependMessages(sessionId, ordered, {
      hasMore,
      oldestCursor: newOldestCursor,
    });
  }
  return {
    count: ordered.length,
    prependedMessages,
    effectiveLimit: limit,
    hasMore,
    oldestCursor: newOldestCursor,
  };
}
