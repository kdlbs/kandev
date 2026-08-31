import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";
import {
  joinOlderMessages,
  requestOlderMessages,
} from "@/hooks/domains/session/older-message-pagination";
import type { Message } from "@/lib/types/http";

const debug = createDebugLogger("messages:lazyload");

export const OLDER_PAGE_LIMIT = 20;

/** Safety cap on message pages fetched inside a single accumulating loadMore
 * call, so a pathological session cannot loop forever. */
const MAX_PAGES_PER_LOAD = 10;
const EMPTY_MESSAGES: Message[] = [];

export type LazyLoadMessagesOptions = {
  /** Accumulate at least this many NEW user prompts per loadMore call before
   * returning. A fixed message page can contain only a few user prompts among
   * agent replies, so the panel requests pages until the threshold is met,
   * pagination is exhausted, a zero-result page stops progress, or
   * MAX_PAGES_PER_LOAD is reached. Default: single page. */
  minUserPromptsPerLoad?: number;
  /** Accumulate at least this many NEW text parts per loadMore call. Text parts
   * are message/content rows (plus legacy untyped rows); tool and activity rows
   * remain loaded but do not advance this target. */
  minTextPartsPerLoad?: number;
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

function containsFirstPrompt(messages: Message[]): boolean {
  return messages.some((message) => message.author_type === "user" && message.prompt_index === 1);
}

function visibleHasMore(rawHasMore: boolean, firstPromptLoaded: boolean): boolean {
  return rawHasMore && !firstPromptLoaded;
}

function countUserPrompts(messages: Message[]): number {
  return messages.filter((message) => message.author_type === "user").length;
}

function countTextParts(messages: Message[]): number {
  return messages.filter(
    (message) => !message.type || message.type === "message" || message.type === "content",
  ).length;
}

function loadTargetsReached(args: {
  loadedPrompts: number;
  loadedTextParts: number;
  minUserPromptsPerLoad?: number;
  minTextPartsPerLoad?: number;
}): boolean {
  const { loadedPrompts, loadedTextParts, minUserPromptsPerLoad, minTextPartsPerLoad } = args;
  return (
    (!minUserPromptsPerLoad || loadedPrompts >= minUserPromptsPerLoad) &&
    (!minTextPartsPerLoad || loadedTextParts >= minTextPartsPerLoad)
  );
}

async function loadPagesToTargets(args: {
  fetchPage: () => Promise<{ count: number; prependedMessages: Message[] }>;
  minUserPromptsPerLoad?: number;
  minTextPartsPerLoad?: number;
}): Promise<number> {
  const { fetchPage, minUserPromptsPerLoad, minTextPartsPerLoad } = args;
  let total = 0;
  let loadedPrompts = 0;
  let loadedTextParts = 0;
  for (let page = 0; page < MAX_PAGES_PER_LOAD; page++) {
    const result = await fetchPage();
    total += result.count;
    loadedPrompts += countUserPrompts(result.prependedMessages);
    loadedTextParts += countTextParts(result.prependedMessages);
    if (result.count === 0) break;
    if (
      loadTargetsReached({
        loadedPrompts,
        loadedTextParts,
        minUserPromptsPerLoad,
        minTextPartsPerLoad,
      })
    )
      break;
  }
  return total;
}

function useVisibleHasMore(sessionId: string | null, rawHasMore: boolean): boolean {
  const firstPromptLoaded = useAppStore((state) =>
    sessionId ? containsFirstPrompt(state.messages.bySession[sessionId] ?? EMPTY_MESSAGES) : false,
  );
  return visibleHasMore(rawHasMore, firstPromptLoaded);
}

type LazyLoadState = {
  hasMore: boolean;
  rawHasMore: boolean;
  oldestCursor: string | null;
  isLoadingMore: boolean;
};

/** Builds the coordinated single-page loader shared by visible and raw consumers. */
function useOlderPageFetcher(sessionId: string | null, stateRef: { current: LazyLoadState }) {
  const store = useAppStoreApi();
  const fetchPage = useCallback(
    async (mode: "visible" | "raw" = "visible") => {
      const { hasMore, rawHasMore, isLoadingMore, oldestCursor } = stateRef.current;
      const canLoad = mode === "raw" ? rawHasMore : hasMore;

      if (!sessionId || !oldestCursor) return { count: 0, prependedMessages: EMPTY_MESSAGES };

      // Check the per-session in-flight map BEFORE the local loading guards: a
      // panel callback during transcript loading or automatic backfill joins
      // the existing promise (first-request-wins) instead of skipping.
      const inFlight = joinOlderMessages(sessionId, oldestCursor);
      if (inFlight) {
        debug("loadMore: joining in-flight request", { sessionId, before: oldestCursor });
        try {
          const result = await inFlight;
          const loadedMessages = store.getState().messages.bySession[sessionId] ?? EMPTY_MESSAGES;
          stateRef.current = {
            hasMore: visibleHasMore(result.hasMore, containsFirstPrompt(loadedMessages)),
            rawHasMore: result.hasMore,
            oldestCursor: result.oldestCursor,
            // The coordinator clears the store's shared isLoadingMore when the
            // session's LAST flight settles (this join may be one of several).
            // Reflect the settled store value so a follow-up page in an
            // accumulation loop is not blocked by a stale in-flight flag.
            isLoadingMore:
              store.getState().messages.metaBySession[sessionId]?.isLoadingMore ??
              stateRef.current.isLoadingMore,
          };
          return result;
        } catch (error) {
          console.error("[useLazyLoadMessages] Error loading messages:", error);
          return { count: 0, prependedMessages: EMPTY_MESSAGES };
        }
      }

      if (!canLoad || isLoadingMore) {
        debug("loadMore: skipped", {
          sessionId,
          reason: describeSkip({ sessionId, isLoadingMore, hasMore: canLoad }),
          hasMore: canLoad,
          oldestCursor,
        });
        return { count: 0, prependedMessages: EMPTY_MESSAGES };
      }

      debug("loadMore: requesting older page", {
        sessionId,
        before: oldestCursor,
        limit: OLDER_PAGE_LIMIT,
      });

      // Update ref synchronously so concurrent calls are blocked immediately.
      stateRef.current.isLoadingMore = true;
      try {
        const result = await requestOlderMessages({
          sessionId,
          cursor: oldestCursor,
          limit: OLDER_PAGE_LIMIT,
          store,
        });
        // Sync immediately; the effect may not run before the next page starts.
        const loadedMessages = store.getState().messages.bySession[sessionId] ?? EMPTY_MESSAGES;
        stateRef.current = {
          hasMore: visibleHasMore(result.hasMore, containsFirstPrompt(loadedMessages)),
          rawHasMore: result.hasMore,
          oldestCursor: result.oldestCursor,
          isLoadingMore: false,
        };
        return result;
      } catch (error) {
        console.error("[useLazyLoadMessages] Error loading messages:", error);
        debug("loadMore: error", { sessionId, error });
        stateRef.current.isLoadingMore = false;
        return { count: 0, prependedMessages: EMPTY_MESSAGES };
      }
    },
    [sessionId, stateRef, store],
  );
  return { fetchPage };
}

export function useLazyLoadMessages(sessionId: string | null, options?: LazyLoadMessagesOptions) {
  const { minUserPromptsPerLoad, minTextPartsPerLoad } = options ?? {};
  const [isAccumulating, setIsAccumulating] = useState(false);
  // Use refs for values that should not trigger callback recreation
  const rawHasMore = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.hasMore ?? false) : false,
  );
  const hasMore = useVisibleHasMore(sessionId, rawHasMore);
  const oldestCursor = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.oldestCursor ?? null) : null,
  );
  const isLoadingMore = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.isLoadingMore ?? false) : false,
  );

  // Store current values in refs to avoid recreating loadMore on every state change
  const stateRef = useRef<LazyLoadState>({ hasMore, rawHasMore, oldestCursor, isLoadingMore });
  useEffect(() => {
    stateRef.current = { hasMore, rawHasMore, oldestCursor, isLoadingMore };
  }, [hasMore, rawHasMore, oldestCursor, isLoadingMore]);

  // Visible pagination stops at prompt #1; raw pagination remains available
  // to explicit recovery consumers such as session search.
  const { fetchPage } = useOlderPageFetcher(sessionId, stateRef);

  // Optional per-consumer targets accumulate matching rows across pages until
  // every configured threshold, exhaustion, a zero-result page, or the safety
  // cap. Raw rows still determine progress and cursor movement.
  const loadMore = useCallback(async () => {
    if ((!minUserPromptsPerLoad && !minTextPartsPerLoad) || !sessionId) {
      return (await fetchPage()).count;
    }
    setIsAccumulating(true);
    try {
      return await loadPagesToTargets({
        fetchPage,
        minUserPromptsPerLoad,
        minTextPartsPerLoad,
      });
    } finally {
      setIsAccumulating(false);
    }
  }, [fetchPage, minTextPartsPerLoad, minUserPromptsPerLoad, sessionId]);

  const loadMoreRaw = useCallback(async () => (await fetchPage("raw")).count, [fetchPage]);

  return {
    loadMore,
    loadMoreRaw,
    hasMore,
    rawHasMore,
    isLoadingMore: isLoadingMore || isAccumulating,
  };
}
