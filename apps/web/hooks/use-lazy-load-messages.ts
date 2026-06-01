// Repro: new task → wait env prep → messages should appear without refresh.
// Bug fix: messages now read from TanStack Query (which the WS bridge populates
// via setQueryData on session.message.added) instead of Zustand
// `messages.bySession`, which used to race with the initial fetchAndStoreMessages
// call and would render empty until a manual refresh seeded the Zustand mirror.
// The Zustand mirror is still updated by the WS handler (lib/ws/handlers/messages.ts)
// for transitional consumers that haven't migrated yet.
import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { listTaskSessionMessages } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { MessagesData } from "@/lib/query/query-options/session";
import { createDebugLogger } from "@/lib/debug/log";
import { prependMessagesIntoCache } from "./domains/session/message-cache";

const debug = createDebugLogger("messages:lazyload");

function describeSkip(args: {
  sessionId: string | null;
  isLoading: boolean;
  hasMore: boolean;
}): string {
  if (!args.sessionId) return "no-session";
  if (args.isLoading) return "already-loading";
  if (!args.hasMore) return "no-more";
  return "no-cursor";
}

type LoadMoreResponseLog = {
  sessionId: string;
  requestedBefore: string;
  ordered: Array<{
    id: string;
    created_at: string;
    type?: string;
    author_type?: string;
  }>;
  responseHasMore: boolean;
  newOldestCursor: string | null;
};

function logLoadMoreResponse(args: LoadMoreResponseLog) {
  const { sessionId, requestedBefore, ordered, responseHasMore, newOldestCursor } = args;
  const first = ordered[0];
  debug("loadMore: response", {
    sessionId,
    requestedBefore,
    fetchedCount: ordered.length,
    responseHasMore,
    newOldestId: newOldestCursor,
    newOldestCreatedAt: first?.created_at ?? null,
    newOldestType: first?.type ?? null,
    newOldestAuthor: first?.author_type ?? null,
  });
  if (ordered.length === 0 && responseHasMore) {
    debug("loadMore: WARNING empty batch with has_more=true — pagination may be stuck", {
      sessionId,
      before: requestedBefore,
    });
  }
  if (!responseHasMore && ordered.length > 0) {
    debug("loadMore: reached oldest — check that the first prompt is present", {
      sessionId,
      newOldestId: newOldestCursor,
      newOldestAuthor: first?.author_type,
      newOldestType: first?.type,
    });
  }
}

export function useLazyLoadMessages(sessionId: string | null) {
  const queryClient = useQueryClient();

  // Read live state from the TanStack Query cache; subscribe so the component
  // re-renders when the bridge writes new messages or when loadMore mutates it.
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!sessionId) return;
    const unsub = queryClient.getQueryCache().subscribe((event) => {
      // Only react to actual data mutations. Without this filter,
      // `observerOptionsUpdated` events — which fire on every render that
      // passes a new options object to `useQuery` — would call setTick on
      // every render, causing an infinite render loop. See React error #185.
      if (event.type !== "updated") return;
      const key = event.query.queryKey;
      if (
        Array.isArray(key) &&
        key[0] === "session" &&
        key[1] === sessionId &&
        key[2] === "messages"
      ) {
        setTick((t) => t + 1);
      }
    });
    return unsub;
  }, [queryClient, sessionId]);

  const cached = sessionId
    ? queryClient.getQueryData<MessagesData>(qk.session.messages(sessionId))
    : undefined;
  const hasMore = cached?.hasMore ?? false;
  const oldestCursor = cached?.oldestCursor ?? null;

  // Transitional mirror — keep Zustand metadata in sync so any not-yet-migrated
  // consumer (e.g. the lazy-load isLoading badge) still sees the right values.
  const prependMessages = useAppStore((state) => state.prependMessages);
  const setMessagesMetadata = useAppStore((state) => state.setMessagesMetadata);

  // Local isLoading flag — managed inside the hook, not Zustand, because TQ is
  // the source of truth now. Refs avoid recreating loadMore on every change.
  const [isLoading, setIsLoading] = useState(false);
  const stateRef = useRef({ hasMore, oldestCursor, isLoading });
  useEffect(() => {
    stateRef.current = { hasMore, oldestCursor, isLoading };
  }, [hasMore, oldestCursor, isLoading, tick]);

  const loadMore = useCallback(async () => {
    const { hasMore, isLoading, oldestCursor } = stateRef.current;

    if (!sessionId || !hasMore || isLoading || !oldestCursor) {
      debug("loadMore: skipped", {
        sessionId,
        reason: describeSkip({ sessionId, isLoading, hasMore }),
        hasMore,
        oldestCursor,
      });
      return 0;
    }

    debug("loadMore: requesting older page", { sessionId, before: oldestCursor, limit: 20 });

    stateRef.current.isLoading = true;
    setIsLoading(true);
    setMessagesMetadata(sessionId, { isLoading: true });
    try {
      const response = await listTaskSessionMessages(sessionId, {
        limit: 20,
        before: oldestCursor,
        sort: "desc",
      });
      const orderedMessages = [...(response.messages ?? [])].reverse();
      const newOldestCursor = orderedMessages[0]?.id ?? null;
      logLoadMoreResponse({
        sessionId,
        requestedBefore: oldestCursor,
        ordered: orderedMessages,
        responseHasMore: response.has_more,
        newOldestCursor,
      });

      // Write into the TQ cache (the source of truth for the UI).
      prependMessagesIntoCache(queryClient, sessionId, orderedMessages, {
        hasMore: response.has_more,
        oldestCursor: newOldestCursor,
      });
      // Mirror into Zustand for transitional consumers.
      prependMessages(sessionId, orderedMessages, {
        hasMore: response.has_more,
        oldestCursor: newOldestCursor,
      });

      stateRef.current = {
        hasMore: response.has_more,
        oldestCursor: newOldestCursor,
        isLoading: false,
      };
      setIsLoading(false);
      return orderedMessages.length;
    } catch (error) {
      console.error("[useLazyLoadMessages] Error loading messages:", error);
      debug("loadMore: error", { sessionId, error });
      stateRef.current.isLoading = false;
      setIsLoading(false);
      setMessagesMetadata(sessionId, { isLoading: false });
      return 0;
    }
  }, [sessionId, prependMessages, setMessagesMetadata, queryClient]);

  return { loadMore, hasMore, isLoading };
}
