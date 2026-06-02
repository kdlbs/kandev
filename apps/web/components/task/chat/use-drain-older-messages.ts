import { useEffect, useState } from "react";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";

/** Hard cap on background pagination batches when draining older messages so
 *  a runaway session (or buggy `has_more=true` with empty pages) can't loop
 *  forever. 50 × 20 messages = 1000-message ceiling. */
const MAX_DRAIN_BATCHES = 50;

/** When `active` flips true, walk the message pagination cursor until the
 *  server reports no more older messages (or the cap is hit). Used by the
 *  Ctrl+R reverse-search overlay so the user can fuzzy-search the entire
 *  session history, not only the pages already loaded by the chat list. */
export function useDrainOlderMessages(sessionId: string | null, active: boolean) {
  const { loadMore } = useLazyLoadMessages(sessionId);
  const [isDraining, setIsDraining] = useState(false);
  useEffect(() => {
    if (!active || !sessionId) return;
    let cancelled = false;
    setIsDraining(true);
    void (async () => {
      try {
        for (let i = 0; i < MAX_DRAIN_BATCHES; i++) {
          if (cancelled) return;
          const fetched = await loadMore();
          if (fetched === 0) break;
        }
      } catch (error) {
        console.error("[useDrainOlderMessages] drain failed:", error);
      } finally {
        if (!cancelled) setIsDraining(false);
      }
    })();
    return () => {
      cancelled = true;
      setIsDraining(false);
    };
  }, [active, sessionId, loadMore]);
  return { isDraining };
}
