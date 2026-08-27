import { useEffect, useRef, useState } from "react";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useAppStore } from "@/components/state-provider";

/** Hard cap on background pagination when draining older messages so a
 *  runaway session (or buggy `has_more=true` with empty pages) can't loop
 *  forever. 1000 messages is the budget; a final batch may overshoot by at
 *  most one page (mixed first-request-wins page sizes). */
export const MAX_DRAIN_MESSAGES = 1000;

export type DrainOlderMessagesOptions = {
  /** Use backend-wide pagination for explicit reverse-search backfill. */
  rawPagination?: boolean;
};

/**
 * When `active` flips true, walk the message pagination cursor until the
 * server reports no more older messages (or the message budget is hit). The
 * default visible path is used by the transcript's scroll-to-start affordance
 * and stops at the first prompt. The Ctrl+R reverse-search overlay opts into
 * raw pagination so it can search the complete backend session history.
 *
 * Step-driven, not an imperative loop: each batch only fires once `isLoadingMore`
 * (older-page, from `useLazyLoadMessages`, reactive) is confirmed false AND the
 * initial/refetch `messagesLoading` flag is clear, so this never races a
 * concurrent caller sharing the same session (e.g. the transcript's own
 * last-prompt preload effect) — it waits for that fetch to actually finish and
 * re-reads the resulting `hasMore` instead of guessing from an ambiguous
 * "0 fetched" return that could mean either genuine exhaustion or a no-op
 * against someone else's in-flight request. Cumulative fetched count is
 * tracked from `loadMore`'s return value: the drain stops at the first batch
 * whose cumulative count reaches or exceeds MAX_DRAIN_MESSAGES, and stops
 * immediately after a zero-result batch even when `hasMore` remains true.
 */
export function useDrainOlderMessages(
  sessionId: string | null,
  active: boolean,
  options?: DrainOlderMessagesOptions,
) {
  const { loadMore, loadMoreRaw, hasMore, rawHasMore, isLoadingMore } =
    useLazyLoadMessages(sessionId);
  const rawPagination = options?.rawPagination ?? false;
  const drainLoadMore = rawPagination ? loadMoreRaw : loadMore;
  const drainHasMore = rawPagination ? rawHasMore : hasMore;
  const messagesLoading = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.isLoading ?? false) : false,
  );
  const [fetchedMessageCount, setFetchedMessageCount] = useState(0);
  const wasActiveRef = useRef(false);

  // Reset the cumulative counter whenever a fresh drain starts, so a later
  // drain isn't pre-capped by an earlier one that ran to completion.
  useEffect(() => {
    if (active && !wasActiveRef.current) {
      setFetchedMessageCount(0);
    }
    wasActiveRef.current = active;
  }, [active]);

  const isDraining =
    active && Boolean(sessionId) && drainHasMore && fetchedMessageCount < MAX_DRAIN_MESSAGES;

  useEffect(() => {
    if (!isDraining || !sessionId || messagesLoading || isLoadingMore) return;
    let cancelled = false;
    void drainLoadMore()
      .then((fetched) => {
        if (cancelled) return;
        setFetchedMessageCount((count) => {
          const next = count + fetched;
          // Cap at the budget so a large final batch cannot push a later
          // drain's accounting past the ceiling.
          return Math.min(next, MAX_DRAIN_MESSAGES);
        });
        if (fetched === 0) {
          // No progress: stop immediately even if hasMore remains true
          // (retained-cursor zero-row drain termination).
          setFetchedMessageCount(MAX_DRAIN_MESSAGES);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          console.error("[useDrainOlderMessages] drain failed:", error);
          // Stop retrying this session on a hard failure rather than
          // hammering it once more every render.
          setFetchedMessageCount(MAX_DRAIN_MESSAGES);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [isDraining, sessionId, messagesLoading, isLoadingMore, drainLoadMore, fetchedMessageCount]);

  return { isDraining };
}
