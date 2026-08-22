import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";
import {
  joinOlderMessages,
  requestOlderMessages,
} from "@/hooks/domains/session/older-message-pagination";

const debug = createDebugLogger("messages:lazyload");

export const OLDER_PAGE_LIMIT = 20;

/** Safety cap on message pages fetched inside a single loadMore call when
 * `minUserPromptsPerLoad` is set, so a pathological session cannot loop
 * forever. */
const MAX_PAGES_PER_LOAD = 10;

export type LazyLoadMessagesOptions = {
  /** Accumulate at least this many NEW user prompts per loadMore call before
   * returning. A fixed message page can contain only a few user prompts among
   * agent replies, so the panel requests pages until the threshold is met,
   * pagination is exhausted, a zero-result page stops progress, or
   * MAX_PAGES_PER_LOAD is reached. Default: single page (transcript behavior
   * unchanged). */
  minUserPromptsPerLoad?: number;
};

function describeSkip(args: {
  sessionId: string | null;
  isLoadingMore: boolean;
  hasMore: boolean;
}): string {
  if (!args.sessionId) return "no-session";
  if (args.isLoadingMore) return "already-loading";
  if (!args.hasMore) return "no-more";
  return "no-cursor";
}

export function useLazyLoadMessages(sessionId: string | null, options?: LazyLoadMessagesOptions) {
  const { minUserPromptsPerLoad } = options ?? {};
  // Use refs for values that should not trigger callback recreation
  const hasMore = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.hasMore ?? false) : false,
  );
  const oldestCursor = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.oldestCursor ?? null) : null,
  );
  const isLoadingMore = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.isLoadingMore ?? false) : false,
  );

  // Store current values in refs to avoid recreating loadMore on every state change
  const stateRef = useRef({ hasMore, oldestCursor, isLoadingMore });
  useEffect(() => {
    stateRef.current = { hasMore, oldestCursor, isLoadingMore };
  }, [hasMore, oldestCursor, isLoadingMore]);

  const store = useAppStoreApi();

  // Fetches one older-message page at the current cursor. Stable - only
  // depends on sessionId and the store API. Requests go through the shared
  // per-session coordinator, which dedups per (sessionId, cursor): a
  // concurrent caller (panel sentinel, backfill, last-prompt preload) joins
  // the in-flight request instead of duplicating it.
  const fetchPage = useCallback(async () => {
    const { hasMore, isLoadingMore, oldestCursor } = stateRef.current;

    if (!sessionId || !oldestCursor) return 0;

    // Check the per-session in-flight map BEFORE the local loading guards: a
    // panel callback during transcript loading or automatic backfill joins
    // the existing promise (first-request-wins) instead of skipping.
    const inFlight = joinOlderMessages(sessionId, oldestCursor);
    if (inFlight) {
      debug("loadMore: joining in-flight request", { sessionId, before: oldestCursor });
      try {
        const result = await inFlight;
        stateRef.current = {
          hasMore: result.hasMore,
          oldestCursor: result.oldestCursor,
          // The coordinator clears the store's shared isLoadingMore when the
          // session's LAST flight settles (this join may be one of several).
          // Reflect the settled store value so a follow-up page in the
          // minUserPromptsPerLoad loop is not blocked by a stale in-flight
          // flag, while still blocking fresh requests when other flights
          // remain in flight.
          isLoadingMore:
            store.getState().messages.metaBySession[sessionId]?.isLoadingMore ??
            stateRef.current.isLoadingMore,
        };
        return result.count;
      } catch (error) {
        console.error("[useLazyLoadMessages] Error loading messages:", error);
        return 0;
      }
    }

    if (!hasMore || isLoadingMore) {
      debug("loadMore: skipped", {
        sessionId,
        reason: describeSkip({ sessionId, isLoadingMore, hasMore }),
        hasMore,
        oldestCursor,
      });
      return 0;
    }

    debug("loadMore: requesting older page", {
      sessionId,
      before: oldestCursor,
      limit: OLDER_PAGE_LIMIT,
    });

    // Update ref synchronously so concurrent calls are blocked immediately
    stateRef.current.isLoadingMore = true;
    try {
      const result = await requestOlderMessages({
        sessionId,
        cursor: oldestCursor,
        limit: OLDER_PAGE_LIMIT,
        store,
      });
      // Sync ref immediately so the next intersection callback sees correct state
      // (the useEffect sync may not have run yet between store update and next observer fire)
      stateRef.current = {
        hasMore: result.hasMore,
        oldestCursor: result.oldestCursor,
        isLoadingMore: false,
      };
      return result.count;
    } catch (error) {
      console.error("[useLazyLoadMessages] Error loading messages:", error);
      debug("loadMore: error", { sessionId, error });
      stateRef.current.isLoadingMore = false;
      return 0;
    }
  }, [sessionId, store]);

  // With `minUserPromptsPerLoad`, one sentinel trigger accumulates at least
  // that many NEW user prompts: message pages contain agent replies too, so a
  // single page can yield only a few prompts. Loops until the threshold,
  // pagination exhaustion, a zero-result page, or MAX_PAGES_PER_LOAD.
  const loadMore = useCallback(async () => {
    if (!minUserPromptsPerLoad || !sessionId) return fetchPage();
    const countUserPrompts = () =>
      (store.getState().messages.bySession[sessionId] ?? []).filter(
        (message) => message.author_type === "user",
      ).length;
    const before = countUserPrompts();
    let total = 0;
    for (let page = 0; page < MAX_PAGES_PER_LOAD; page++) {
      const added = await fetchPage();
      total += added;
      if (added === 0) break;
      if (countUserPrompts() - before >= minUserPromptsPerLoad) break;
    }
    return total;
  }, [fetchPage, minUserPromptsPerLoad, sessionId, store]);

  return { loadMore, hasMore, isLoadingMore };
}
