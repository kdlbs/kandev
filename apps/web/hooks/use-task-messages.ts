import { useEffect, useRef, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { AgentSessionState, Message } from '@/lib/types/http';

interface UseTaskMessagesReturn {
  isLoading: boolean;
}

export function useTaskMessages(
  taskId: string | null,
  agentSessionId: string | null
): UseTaskMessagesReturn {
  const store = useAppStoreApi();
  const messagesState = useAppStore((state) => state.messages);
  const agentSessionState = useAppStore((state) =>
    taskId ? (state.agentSessionStatesByTaskId[taskId] ?? null) : null
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [isLoading, setIsLoading] = useState(false);
  const [isWaitingForInitialMessages, setIsWaitingForInitialMessages] = useState(false);
  const initialFetchStartRef = useRef<number | null>(null);
  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const lastFetchStateKeyRef = useRef<string | null>(null);
  const hasAgentMessage = messagesState.items.some((message) => message.author_type === 'agent');

  useEffect(() => {
    if (!taskId) return;
    store.getState().clearGitStatus();
  }, [store, taskId]);

  useEffect(() => {
    store.getState().setMessagesSessionId(agentSessionId);
    if (!agentSessionId) {
      console.log('[useTaskMessages] no agent_session_id yet, clearing messages');
      store.getState().setMessages(null, []);
      initialFetchStartRef.current = null;
      lastFetchedSessionIdRef.current = null;
      setIsWaitingForInitialMessages(false);
    }
  }, [agentSessionId, store]);

  useEffect(() => {
    if (!agentSessionId) return;
    if (messagesState.items.length > 0) return;
    if (initialFetchStartRef.current === null) {
      initialFetchStartRef.current = Date.now();
      setIsWaitingForInitialMessages(true);
    }
  }, [agentSessionId, messagesState.items.length]);

  // Fetch messages on mount and when session changes
  useEffect(() => {
    if (!agentSessionId) return;
    if (connectionStatus !== 'connected') {
      console.warn('[useTaskMessages] WebSocket not connected yet, waiting to fetch messages');
      return;
    }

    // Set sessionId immediately so that incoming WebSocket notifications are processed
    // before the API call completes (fixes race condition on first agent start)
    store.getState().setMessagesSessionId(agentSessionId);

    if (lastFetchedSessionIdRef.current === agentSessionId && messagesState.items.length > 0) {
      return;
    }

    const fetchMessages = async () => {
      const client = getWebSocketClient();
      if (!client) {
        console.warn('[useTaskMessages] WebSocket client not ready');
        return;
      }

      setIsLoading(true);
      store.getState().setMessagesLoading(true);
      if (initialFetchStartRef.current === null) {
        initialFetchStartRef.current = Date.now();
        setIsWaitingForInitialMessages(true);
      }

      try {
        console.log('[useTaskMessages] requesting message.list', { agentSessionId });
        const response = await client.request<{ messages: Message[] }>(
          'message.list',
          { agent_session_id: agentSessionId },
          10000
        );
        console.log('[useTaskMessages] message.list response:', JSON.stringify(response, null, 2));
        store.getState().setMessages(agentSessionId, response.messages ?? []);
        lastFetchedSessionIdRef.current = agentSessionId;
        if ((response.messages ?? []).length > 0) {
          setIsWaitingForInitialMessages(false);
        }
      } catch (error) {
        console.error('Failed to fetch messages:', error);
        store.getState().setMessages(agentSessionId, []);
        lastFetchedSessionIdRef.current = agentSessionId;
      } finally {
        setIsLoading(false);
      }
    };

    fetchMessages();
  }, [agentSessionId, connectionStatus, messagesState.items.length, store]);

  // Subscribe to task for real-time updates
  useEffect(() => {
    if (!taskId) return;

    const client = getWebSocketClient();
    if (!client) {
      console.warn('[useTaskMessages] WebSocket client not ready for subscribe');
      return;
    }

    // Subscribe to task updates
    console.log('[useTaskMessages] subscribing to task', { taskId });
    client.subscribe(taskId);

    return () => {
      // Unsubscribe when leaving
      console.log('[useTaskMessages] unsubscribing from task', { taskId });
      client.unsubscribe(taskId);
    };
  }, [taskId]);

  useEffect(() => {
    if (!agentSessionId || !agentSessionState || !taskId) return;
    if (hasAgentMessage) return;

    const terminalStates = new Set<AgentSessionState>(['WAITING_FOR_INPUT', 'COMPLETED', 'FAILED']);
    if (!terminalStates.has(agentSessionState)) {
      return;
    }

    const key = `${agentSessionId}:${agentSessionState}`;
    if (lastFetchStateKeyRef.current === key) {
      return;
    }
    lastFetchStateKeyRef.current = key;

    const fetchMessages = async () => {
      const client = getWebSocketClient();
      if (!client) {
        console.warn('[useTaskMessages] WebSocket client not ready for state fetch');
        return;
      }

      setIsLoading(true);
      store.getState().setMessagesLoading(true);
      try {
        console.log('[useTaskMessages] requesting message.list after state change', { agentSessionId, agentSessionState });
        const response = await client.request<{ messages: Message[] }>(
          'message.list',
          { agent_session_id: agentSessionId },
          10000
        );
        store.getState().setMessages(agentSessionId, response.messages ?? []);
        if ((response.messages ?? []).length > 0) {
          setIsWaitingForInitialMessages(false);
        }
      } catch (error) {
        console.error('Failed to fetch messages after state change:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchMessages();
  }, [agentSessionId, agentSessionState, hasAgentMessage, store, taskId]);

  return {
    isLoading: isLoading || isWaitingForInitialMessages,
  };
}
