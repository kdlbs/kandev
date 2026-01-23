import { useEffect, useRef, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { TaskSessionState, Message } from '@/lib/types/http';

interface UseSessionMessagesReturn {
  isLoading: boolean;
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
}

const EMPTY_MESSAGES: Message[] = [];
const EMPTY_META = { isLoading: false, hasMore: false, oldestCursor: null };

export function useSessionMessages(
  taskSessionId: string | null
): UseSessionMessagesReturn {
  const store = useAppStoreApi();
  const messages = useAppStore((state) =>
    taskSessionId ? state.messages.bySession[taskSessionId] ?? EMPTY_MESSAGES : EMPTY_MESSAGES
  );
  const messagesMeta = useAppStore((state) =>
    taskSessionId
      ? state.messages.metaBySession[taskSessionId] ?? EMPTY_META
      : EMPTY_META
  );
  const taskSessionState = useAppStore((state) =>
    taskSessionId ? state.taskSessions.items[taskSessionId]?.state ?? null : null
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [isLoading, setIsLoading] = useState(false);
  const [isWaitingForInitialMessages, setIsWaitingForInitialMessages] = useState(false);
  const initialFetchStartRef = useRef<number | null>(null);
  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const lastFetchStateKeyRef = useRef<string | null>(null);
  const hasAgentMessage = messages.some((message: Message) => message.author_type === 'agent');

  useEffect(() => {
    if (!taskSessionId) {
      initialFetchStartRef.current = null;
      lastFetchedSessionIdRef.current = null;
      setIsWaitingForInitialMessages(false);
    }
  }, [taskSessionId, store]);

  useEffect(() => {
    if (!taskSessionId) return;
    // Don't set waiting state if messages are already loaded (from SSR or cache)
    if (messages.length > 0) {
      setIsWaitingForInitialMessages(false);
      return;
    }
    if (initialFetchStartRef.current === null) {
      initialFetchStartRef.current = Date.now();
      setIsWaitingForInitialMessages(true);
    }
  }, [taskSessionId, messages.length]);

  // Fetch messages on mount and when session changes
  useEffect(() => {
    if (!taskSessionId) return;
    if (connectionStatus !== 'connected') {
      return;
    }

    // Check if messages are already loaded (from SSR or previous fetch)
    if (messages.length > 0) {
      lastFetchedSessionIdRef.current = taskSessionId;
      setIsWaitingForInitialMessages(false);
      return;
    }

    if (lastFetchedSessionIdRef.current === taskSessionId) {
      return;
    }

    const fetchMessages = async () => {
      const client = getWebSocketClient();
      if (!client) {
        return;
      }

      setIsLoading(true);
      store.getState().setMessagesLoading(taskSessionId, true);
      if (initialFetchStartRef.current === null) {
        initialFetchStartRef.current = Date.now();
        setIsWaitingForInitialMessages(true);
      }

      try {
        const response = await client.request<{ messages: Message[] }>(
          'message.list',
          { session_id: taskSessionId },
          10000
        );
        store.getState().setMessages(taskSessionId, response.messages ?? []);
        lastFetchedSessionIdRef.current = taskSessionId;
        if ((response.messages ?? []).length > 0) {
          setIsWaitingForInitialMessages(false);
        }
      } catch (error) {
        console.error('Failed to fetch messages:', error);
        store.getState().setMessages(taskSessionId, []);
        lastFetchedSessionIdRef.current = taskSessionId;
      } finally {
        store.getState().setMessagesLoading(taskSessionId, false);
        setIsLoading(false);
      }
    };

    fetchMessages();
  }, [taskSessionId, connectionStatus, messages.length, store]);

  useEffect(() => {
    if (!taskSessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribeSession(taskSessionId);
    return () => {
      unsubscribe();
    };
  }, [taskSessionId]);

  useEffect(() => {
    if (!taskSessionId || !taskSessionState) return;
    if (hasAgentMessage) return;

    const terminalStates = new Set<TaskSessionState>(['WAITING_FOR_INPUT', 'COMPLETED', 'FAILED']);
    if (!terminalStates.has(taskSessionState)) {
      return;
    }

    const key = `${taskSessionId}:${taskSessionState}`;
    if (lastFetchStateKeyRef.current === key) {
      return;
    }
    lastFetchStateKeyRef.current = key;

    const fetchMessages = async () => {
      const client = getWebSocketClient();
      if (!client) {
        return;
      }

      setIsLoading(true);
      store.getState().setMessagesLoading(taskSessionId, true);
      try {
        const response = await client.request<{ messages: Message[] }>(
          'message.list',
          { session_id: taskSessionId },
          10000
        );
        store.getState().setMessages(taskSessionId, response.messages ?? []);
        if ((response.messages ?? []).length > 0) {
          setIsWaitingForInitialMessages(false);
        }
      } catch (error) {
        console.error('Failed to fetch messages after state change:', error);
      } finally {
        store.getState().setMessagesLoading(taskSessionId, false);
        setIsLoading(false);
      }
    };

    fetchMessages();
  }, [taskSessionId, taskSessionState, hasAgentMessage, store]);

  return {
    isLoading: isLoading || isWaitingForInitialMessages || messagesMeta.isLoading,
    messages,
    hasMore: messagesMeta.hasMore,
    oldestCursor: messagesMeta.oldestCursor,
  };
}
