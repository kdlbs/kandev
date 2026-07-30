import { useEffect, useState } from "react";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";

/** Hard cap on background pagination batches when draining older messages so
 *  a runaway session (or buggy `has_more=true` with empty pages) can't loop
 *  forever. 50 × 20 messages = 1000-message ceiling. */
const MAX_DRAIN_BATCHES = 50;

/**
 * When `active` flips true, walk the message pagination cursor until the
 * server reports no more older messages (or the cap is hit). Used by the
 * Ctrl+R reverse-search overlay so the user can fuzzy-search the entire
 * session history, not only the pages already loaded by the chat list, and
 * by the transcript's scroll-to-start affordance so it lands on the true
 * first prompt instead of a partial-page boundary.
 *
 * Step-driven, not an imperative loop: each batch only fires once `isLoading`
 * (from `useLazyLoadMessages`, reactive) is confirmed false, so this never
 * races a concurrent caller sharing the same session (e.g. the transcript's
 * own last-prompt preload effect) — it waits for that fetch to actually
 * finish and re-reads the resulting `hasMore` instead of guessing from an
 * ambiguous "0 fetched" return that could mean either genuine exhaustion or
 * a no-op against someone else's in-flight request.
 */
export function useDrainOlderMessages(sessionId: string | null, active: boolean) {
  const { loadMore, hasMore, isLoading } = useLazyLoadMessages(sessionId);
  const [batchCount, setBatchCount] = useState(0);

  // Reset the batch counter whenever a fresh drain starts, so a later drain
  // isn't pre-capped by an earlier one that ran to completion.
  useEffect(() => {
    if (active) setBatchCount(0);
  }, [active]);

  const isDraining = active && Boolean(sessionId) && hasMore && batchCount < MAX_DRAIN_BATCHES;

  useEffect(() => {
    if (!isDraining || !sessionId || isLoading) return;
    let cancelled = false;
    void loadMore()
      .then(() => {
        if (!cancelled) setBatchCount((count) => count + 1);
      })
      .catch((error) => {
        if (!cancelled) {
          console.error("[useDrainOlderMessages] drain failed:", error);
          // Stop retrying this session on a hard failure rather than
          // hammering it once more every render.
          setBatchCount(MAX_DRAIN_BATCHES);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [isDraining, sessionId, isLoading, loadMore, batchCount]);

  return { isDraining };
}
