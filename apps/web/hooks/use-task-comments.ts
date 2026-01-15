import { useEffect, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { Comment } from '@/lib/types/http';

interface UseTaskCommentsReturn {
  isLoading: boolean;
}

export function useTaskComments(taskId: string | null): UseTaskCommentsReturn {
  const store = useAppStoreApi();
  const commentsState = useAppStore((state) => state.comments);
  const [isLoading, setIsLoading] = useState(false);

  // Fetch comments on mount and when task changes
  useEffect(() => {
    if (!taskId) return;

    // Set taskId immediately so that incoming WebSocket notifications are processed
    // before the API call completes (fixes race condition on first agent start)
    store.getState().setCommentsTaskId(taskId);

    // Clear git status when switching tasks to avoid showing stale data
    store.getState().clearGitStatus();

    if (commentsState.taskId === taskId) {
      return;
    }

    const fetchComments = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      setIsLoading(true);
      store.getState().setCommentsLoading(true);

      try {
        const response = await client.request<{ comments: Comment[] }>('comment.list', { task_id: taskId }, 10000);
        console.log('[useTaskComments] comment.list response:', JSON.stringify(response, null, 2));
        store.getState().setComments(taskId, response.comments ?? []);
      } catch (error) {
        console.error('Failed to fetch comments:', error);
        store.getState().setComments(taskId, []);
      } finally {
        setIsLoading(false);
      }
    };

    fetchComments();
  }, [commentsState.taskId, taskId, store]);

  // Subscribe to task for real-time updates
  useEffect(() => {
    if (!taskId) return;

    const client = getWebSocketClient();
    if (!client) return;

    // Subscribe to task updates
    client.subscribe(taskId);

    return () => {
      // Unsubscribe when leaving
      client.unsubscribe(taskId);
    };
  }, [taskId]);

  return {
    isLoading,
  };
}

