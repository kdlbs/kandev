import { useEffect, useCallback } from 'react';
import { useAppStore } from '@/components/state-provider';
import {
  queueMessage,
  cancelQueuedMessage,
  getQueueStatus,
  updateQueuedMessage,
} from '@/lib/api/domains/queue-api';

export type MessageAttachment = {
  type: string;
  data: string;
  mime_type: string;
};

export function useQueue(sessionId: string | null) {
  const queueStatus = useAppStore((state) =>
    sessionId ? state.queue.bySessionId[sessionId] : undefined
  );
  const isLoading = useAppStore((state) =>
    sessionId ? state.queue.isLoading[sessionId] ?? false : false
  );
  const setQueueStatus = useAppStore((state) => state.setQueueStatus);
  const setQueueLoading = useAppStore((state) => state.setQueueLoading);

  // Fetch queue status on mount
  useEffect(() => {
    if (!sessionId) return;

    const fetchStatus = async () => {
      try {
        setQueueLoading(sessionId, true);
        const status = await getQueueStatus(sessionId);
        setQueueStatus(sessionId, status);
      } catch (error) {
        console.error('Failed to fetch queue status:', error);
      } finally {
        setQueueLoading(sessionId, false);
      }
    };

    fetchStatus();
  }, [sessionId, setQueueStatus, setQueueLoading]);

  const queue = useCallback(
    async (
      taskId: string,
      content: string,
      model?: string,
      planMode?: boolean,
      attachments?: MessageAttachment[]
    ) => {
      if (!sessionId) return;

      try {
        setQueueLoading(sessionId, true);
        const queuedMsg = await queueMessage({
          session_id: sessionId,
          task_id: taskId,
          content,
          model,
          plan_mode: planMode,
          attachments,
        });

        setQueueStatus(sessionId, {
          is_queued: true,
          message: queuedMsg,
        });
      } catch (error) {
        console.error('Failed to queue message:', error);
        throw error;
      } finally {
        setQueueLoading(sessionId, false);
      }
    },
    [sessionId, setQueueStatus, setQueueLoading]
  );

  const cancel = useCallback(async () => {
    if (!sessionId) return;

    try {
      setQueueLoading(sessionId, true);
      await cancelQueuedMessage(sessionId);
      setQueueStatus(sessionId, {
        is_queued: false,
        message: null,
      });
    } catch (error) {
      console.error('Failed to cancel queue:', error);
      throw error;
    } finally {
      setQueueLoading(sessionId, false);
    }
  }, [sessionId, setQueueStatus, setQueueLoading]);

  const updateContent = useCallback(
    async (content: string) => {
      if (!sessionId) return;

      try {
        const status = await updateQueuedMessage(sessionId, content);
        setQueueStatus(sessionId, status);
      } catch (error) {
        console.error('Failed to update queued message:', error);
        throw error;
      }
    },
    [sessionId, setQueueStatus]
  );

  return {
    queueStatus,
    isQueued: queueStatus?.is_queued ?? false,
    queuedMessage: queueStatus?.message,
    isLoading,
    queue,
    cancel,
    updateContent,
  };
}
