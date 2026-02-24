import { useCallback, useEffect, useRef } from "react";
import { listTaskSessionMessages } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";

export function useLazyLoadMessages(sessionId: string | null) {
  // Use refs for values that should not trigger callback recreation
  const hasMore = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.hasMore ?? false) : false,
  );
  const oldestCursor = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.oldestCursor ?? null) : null,
  );
  const isLoading = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.isLoading ?? false) : false,
  );

  // Store current values in refs to avoid recreating loadMore on every state change
  const stateRef = useRef({ hasMore, oldestCursor, isLoading });
  useEffect(() => {
    stateRef.current = { hasMore, oldestCursor, isLoading };
  }, [hasMore, oldestCursor, isLoading]);

  const prependMessages = useAppStore((state) => state.prependMessages);
  const setMessagesMetadata = useAppStore((state) => state.setMessagesMetadata);

  // Stable loadMore - only depends on sessionId and store actions
  const loadMore = useCallback(async () => {
    const { hasMore, isLoading, oldestCursor } = stateRef.current;

    console.log("[LazyLoad] loadMore called", { sessionId, hasMore, isLoading, oldestCursor });
    if (!sessionId || !hasMore || isLoading || !oldestCursor) {
      console.log("[LazyLoad] loadMore skipped — guard failed");
      return 0;
    }

    setMessagesMetadata(sessionId, { isLoading: true });
    try {
      const response = await listTaskSessionMessages(sessionId, {
        limit: 20,
        before: oldestCursor,
        sort: "desc",
      });
      const orderedMessages = [...(response.messages ?? [])].reverse();
      // After reversing, orderedMessages[0] is the oldest message in this batch
      const newOldestCursor = orderedMessages[0]?.id ?? null;
      console.log("[LazyLoad] fetched", orderedMessages.length, "messages, hasMore:", response.has_more, "newOldestCursor:", newOldestCursor);
      prependMessages(sessionId, orderedMessages, {
        hasMore: response.has_more,
        oldestCursor: newOldestCursor,
      });
      return orderedMessages.length;
    } catch (error) {
      console.error("[useLazyLoadMessages] Error loading messages:", error);
      setMessagesMetadata(sessionId, { isLoading: false });
      return 0;
    }
  }, [sessionId, prependMessages, setMessagesMetadata]);

  return { loadMore, hasMore, isLoading };
}
