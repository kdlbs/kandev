import { useCallback } from 'react';
import { listTaskComments } from '@/lib/http';
import { useAppStore } from '@/components/state-provider';

export function useLazyLoadComments(taskId: string | null) {
  const { hasMore, oldestCursor, isLoading } = useAppStore((state) => state.comments);
  const prependComments = useAppStore((state) => state.prependComments);
  const setCommentsMetadata = useAppStore((state) => state.setCommentsMetadata);

  const loadMore = useCallback(async () => {
    if (!taskId || !hasMore || isLoading || !oldestCursor) {
      return 0;
    }

    setCommentsMetadata({ isLoading: true });
    try {
      const response = await listTaskComments(taskId, {
        limit: 20,
        before: oldestCursor,
        sort: 'desc',
      });
      const orderedComments = [...(response.comments ?? [])].reverse();
      prependComments(orderedComments, {
        hasMore: response.has_more,
        oldestCursor: response.cursor || (orderedComments[0]?.id ?? null),
      });
      return orderedComments.length;
    } catch {
      setCommentsMetadata({ isLoading: false });
      return 0;
    }
  }, [taskId, hasMore, isLoading, oldestCursor, prependComments, setCommentsMetadata]);

  return { loadMore, hasMore, isLoading };
}
