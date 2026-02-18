import { useEffect, useRef, useState, type MutableRefObject } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { TaskSessionState, Message } from '@/lib/types/http';

interface UseSessionMessagesReturn {
  isLoading: boolean;
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
}

type MessageListResponse = { messages: Message[]; has_more?: boolean; cursor?: string };

const EMPTY_MESSAGES: Message[] = [];
const EMPTY_META = { isLoading: false, hasMore: false, oldestCursor: null };

/** Fetch latest messages via WS and store them. Returns the fetched messages. */
async function fetchAndStoreMessages(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>
): Promise<Message[]> {
  const client = getWebSocketClient();
  if (!client) return [];

  const response = await client.request<MessageListResponse>(
    'message.list',
    { session_id: sessionId, limit: 50, sort: 'desc' },
    10000
  );
  const fetched = [...(response.messages ?? [])].reverse();
  store.getState().setMessages(sessionId, fetched, {
    hasMore: response.has_more ?? false,
    oldestCursor: fetched[0]?.id ?? null,
  });
  return fetched;
}

type FetchMessagesParams = {
  taskSessionId: string;
  store: ReturnType<typeof useAppStoreApi>;
  setIsLoading: (v: boolean) => void;
  setIsWaitingForInitialMessages: (v: boolean) => void;
  initialFetchStartRef: MutableRefObject<number | null>;
  lastFetchedSessionIdRef: MutableRefObject<string | null>;
  onError?: (error: unknown) => void;
};

async function doFetchMessages({
  taskSessionId,
  store,
  setIsLoading,
  setIsWaitingForInitialMessages,
  initialFetchStartRef,
  lastFetchedSessionIdRef,
  onError,
}: FetchMessagesParams): Promise<void> {
  setIsLoading(true);
  store.getState().setMessagesLoading(taskSessionId, true);
  if (initialFetchStartRef.current === null) {
    initialFetchStartRef.current = Date.now();
    setIsWaitingForInitialMessages(true);
  }
  try {
    const fetched = await fetchAndStoreMessages(taskSessionId, store);
    lastFetchedSessionIdRef.current = taskSessionId;
    if (fetched.length > 0) setIsWaitingForInitialMessages(false);
  } catch (error) {
    if (onError) onError(error);
    else console.error('Failed to fetch messages:', error);
    store.getState().setMessages(taskSessionId, []);
    lastFetchedSessionIdRef.current = taskSessionId;
  } finally {
    store.getState().setMessagesLoading(taskSessionId, false);
    setIsLoading(false);
  }
}

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

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!taskSessionId) {
      initialFetchStartRef.current = null;
      lastFetchedSessionIdRef.current = null;
      setIsWaitingForInitialMessages(false);
    }
  }, [taskSessionId, store]);

  useEffect(() => {
    if (!taskSessionId) return;
    if (messages.length > 0) {
      setIsWaitingForInitialMessages(false);
      return;
    }
    if (initialFetchStartRef.current === null) {
      initialFetchStartRef.current = Date.now();
      setIsWaitingForInitialMessages(true);
    }
  }, [taskSessionId, messages.length]);
  /* eslint-enable react-hooks/set-state-in-effect */

  useEffect(() => {
    if (!taskSessionId || connectionStatus !== 'connected') return;

    const sessionChanged = prevSessionIdRef.current !== null &&
                           prevSessionIdRef.current !== taskSessionId;
    prevSessionIdRef.current = taskSessionId;

    if (sessionChanged) {
      lastFetchedSessionIdRef.current = null;
    }

    if (messages.length > 0 && !sessionChanged) {
      lastFetchedSessionIdRef.current = taskSessionId;
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsWaitingForInitialMessages(false);
      return;
    }

    if (lastFetchedSessionIdRef.current === taskSessionId) return;

    void doFetchMessages({
      taskSessionId, store, setIsLoading, setIsWaitingForInitialMessages,
      initialFetchStartRef, lastFetchedSessionIdRef,
    });
  }, [taskSessionId, connectionStatus, messages.length, store]);

  useEffect(() => {
    if (!taskSessionId || connectionStatus !== 'connected') return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribeSession(taskSessionId);
    return () => { unsubscribe(); };
  }, [taskSessionId, connectionStatus]);

  useEffect(() => {
    if (!taskSessionId || !taskSessionState || hasAgentMessage) return;

    const terminalStates = new Set<TaskSessionState>(['WAITING_FOR_INPUT', 'COMPLETED', 'FAILED']);
    if (!terminalStates.has(taskSessionState)) return;

    const key = `${taskSessionId}:${taskSessionState}`;
    if (lastFetchStateKeyRef.current === key) return;
    lastFetchStateKeyRef.current = key;

    void doFetchMessages({
      taskSessionId, store, setIsLoading, setIsWaitingForInitialMessages,
      initialFetchStartRef, lastFetchedSessionIdRef,
      onError: (error) => console.error('Failed to fetch messages after state change:', error),
    });
  }, [taskSessionId, taskSessionState, hasAgentMessage, store]);

  return {
    isLoading: isLoading || isWaitingForInitialMessages || messagesMeta.isLoading,
    messages,
    hasMore: messagesMeta.hasMore,
    oldestCursor: messagesMeta.oldestCursor,
  };
}
