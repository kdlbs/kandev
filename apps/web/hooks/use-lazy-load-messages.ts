import { useCallback } from 'react';
import { listAgentSessionMessages } from '@/lib/http';
import { useAppStore } from '@/components/state-provider';

export function useLazyLoadMessages(sessionId: string | null) {
  const { hasMore, oldestCursor, isLoading } = useAppStore((state) => state.messages);
  const prependMessages = useAppStore((state) => state.prependMessages);
  const setMessagesMetadata = useAppStore((state) => state.setMessagesMetadata);

  const loadMore = useCallback(async () => {
    if (!sessionId || !hasMore || isLoading || !oldestCursor) {
      return 0;
    }

    setMessagesMetadata({ isLoading: true });
    try {
      const response = await listAgentSessionMessages(sessionId, {
        limit: 20,
        before: oldestCursor,
        sort: 'desc',
      });
      const orderedMessages = [...(response.messages ?? [])].reverse();
      prependMessages(orderedMessages, {
        hasMore: response.has_more,
        oldestCursor: response.cursor || (orderedMessages[0]?.id ?? null),
      });
      return orderedMessages.length;
    } catch {
      setMessagesMetadata({ isLoading: false });
      return 0;
    }
  }, [sessionId, hasMore, isLoading, oldestCursor, prependMessages, setMessagesMetadata]);

  return { loadMore, hasMore, isLoading };
}
