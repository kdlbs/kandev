import { useCallback } from 'react';
import { listTaskSessionMessages } from '@/lib/api';
import { useAppStore } from '@/components/state-provider';

export function useLazyLoadMessages(sessionId: string | null) {
  const hasMore = useAppStore((state) =>
    sessionId ? state.messages.metaBySession[sessionId]?.hasMore ?? false : false
  );
  const oldestCursor = useAppStore((state) =>
    sessionId ? state.messages.metaBySession[sessionId]?.oldestCursor ?? null : null
  );
  const isLoading = useAppStore((state) =>
    sessionId ? state.messages.metaBySession[sessionId]?.isLoading ?? false : false
  );
  const prependMessages = useAppStore((state) => state.prependMessages);
  const setMessagesMetadata = useAppStore((state) => state.setMessagesMetadata);

  const loadMore = useCallback(async () => {
    if (!sessionId || !hasMore || isLoading || !oldestCursor) {
      console.log('[useLazyLoadMessages] Cannot load more:', { sessionId, hasMore, isLoading, oldestCursor });
      return 0;
    }

    console.log('[useLazyLoadMessages] Loading more messages before:', oldestCursor);
    setMessagesMetadata(sessionId, { isLoading: true });
    try {
      const response = await listTaskSessionMessages(sessionId, {
        limit: 20,
        before: oldestCursor,
        sort: 'desc',
      });
      console.log('[useLazyLoadMessages] Received response:', {
        count: response.messages?.length,
        hasMore: response.has_more,
        cursor: response.cursor,
      });
      const orderedMessages = [...(response.messages ?? [])].reverse();
      // After reversing, orderedMessages[0] is the oldest message in this batch
      const newOldestCursor = orderedMessages[0]?.id ?? null;
      console.log('[useLazyLoadMessages] Prepending messages:', {
        count: orderedMessages.length,
        newOldestCursor,
        hasMore: response.has_more,
        firstMsgId: orderedMessages[0]?.id,
        lastMsgId: orderedMessages[orderedMessages.length - 1]?.id,
      });
      prependMessages(sessionId, orderedMessages, {
        hasMore: response.has_more,
        oldestCursor: newOldestCursor,
      });
      return orderedMessages.length;
    } catch (error) {
      console.error('[useLazyLoadMessages] Error loading messages:', error);
      setMessagesMetadata(sessionId, { isLoading: false });
      return 0;
    }
  }, [sessionId, hasMore, isLoading, oldestCursor, prependMessages, setMessagesMetadata]);

  return { loadMore, hasMore, isLoading };
}
