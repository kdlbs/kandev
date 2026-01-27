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
  const prevSessionIdRef = useRef<string | null>(null);
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

    // Detect session change to force refetch
    const sessionChanged = prevSessionIdRef.current !== null &&
                           prevSessionIdRef.current !== taskSessionId;

    // Update previous session ref
    prevSessionIdRef.current = taskSessionId;

    // If session changed, reset fetch guard to force refetch
    if (sessionChanged) {
      lastFetchedSessionIdRef.current = null;
    }

    // Check if messages are already loaded (from SSR or previous fetch)
    // BUT: if session just changed, we want to refetch anyway
    if (messages.length > 0 && !sessionChanged) {
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
        // Load most recent messages in descending order, then reverse to show oldest-to-newest
        const response = await client.request<{ messages: Message[]; has_more?: boolean; cursor?: string }>(
          'message.list',
          { session_id: taskSessionId, limit: 50, sort: 'desc' },
          10000
        );
        const messages = [...(response.messages ?? [])].reverse();
        store.getState().setMessages(taskSessionId, messages, {
          hasMore: response.has_more ?? false,
          // oldestCursor should be the first (oldest) message ID for lazy loading older messages
          oldestCursor: messages[0]?.id ?? null,
        });
        lastFetchedSessionIdRef.current = taskSessionId;
        if (messages.length > 0) {
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
        // Load most recent messages in descending order, then reverse to show oldest-to-newest
        const response = await client.request<{ messages: Message[]; has_more?: boolean; cursor?: string }>(
          'message.list',
          { session_id: taskSessionId, limit: 50, sort: 'desc' },
          10000
        );
        const messages = [...(response.messages ?? [])].reverse();
        store.getState().setMessages(taskSessionId, messages, {
          hasMore: response.has_more ?? false,
          // oldestCursor should be the first (oldest) message ID for lazy loading older messages
          oldestCursor: messages[0]?.id ?? null,
        });
        if (messages.length > 0) {
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
