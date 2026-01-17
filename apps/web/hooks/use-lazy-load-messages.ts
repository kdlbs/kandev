import { useCallback } from 'react';
import { listTaskSessionMessages } from '@/lib/http';
import { useAppStore } from '@/components/state-provider';

export function useLazyLoadMessages(sessionId: string | null) {
  const { hasMore, oldestCursor, isLoading } = useAppStore((state) =>
    sessionId
      ? state.messages.metaBySession[sessionId] ?? { hasMore: false, oldestCursor: null, isLoading: false }
      : { hasMore: false, oldestCursor: null, isLoading: false }
  );
  const prependMessages = useAppStore((state) => state.prependMessages);
  const setMessagesMetadata = useAppStore((state) => state.setMessagesMetadata);

  const loadMore = useCallback(async () => {
    if (!sessionId || !hasMore || isLoading || !oldestCursor) {
      return 0;
    }

    setMessagesMetadata(sessionId, { isLoading: true });
    try {
      const response = await listTaskSessionMessages(sessionId, {
        limit: 20,
        before: oldestCursor,
        sort: 'desc',
      });
      const orderedMessages = [...(response.messages ?? [])].reverse();
      prependMessages(sessionId, orderedMessages, {
        hasMore: response.has_more,
        oldestCursor: response.cursor || (orderedMessages[0]?.id ?? null),
      });
      return orderedMessages.length;
    } catch {
      setMessagesMetadata(sessionId, { isLoading: false });
      return 0;
    }
  }, [sessionId, hasMore, isLoading, oldestCursor, prependMessages, setMessagesMetadata]);

  return { loadMore, hasMore, isLoading };
}
