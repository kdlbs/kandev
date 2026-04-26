import { useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskSessionState, Message } from "@/lib/types/http";

interface UseSessionMessagesReturn {
  isLoading: boolean;
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
}

type MessageListResponse = { messages: Message[]; has_more?: boolean; cursor?: string };

const EMPTY_MESSAGES: Message[] = [];
const EMPTY_META = { isLoading: false, hasMore: false, oldestCursor: null };

/** Fetch latest messages via WS and merge with any that arrived via live notifications. */
async function fetchAndStoreMessages(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
): Promise<Message[]> {
  const client = getWebSocketClient();
  if (!client) {
    return [];
  }

  const response = await client.request<MessageListResponse>(
    "message.list",
    { session_id: sessionId, limit: 50, sort: "desc" },
    10000,
  );
  const fetched = [...(response.messages ?? [])].reverse();
  // Merge: keep WS-delivered messages that aren't in the fetch response.
  // This prevents a slow fetch (sent before messages existed) from wiping
  // messages that arrived via real-time notifications while the fetch was
  // in flight.
  const existing = store.getState().messages.bySession[sessionId] ?? [];
  const fetchedIds = new Set(fetched.map((m) => m.id));
  const extras = existing.filter((m) => !fetchedIds.has(m.id));
  const merged =
    extras.length > 0
      ? [...fetched, ...extras].sort(
          (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
        )
      : fetched;

  store.getState().setMessages(sessionId, merged, {
    hasMore: response.has_more ?? false,
    oldestCursor: merged[0]?.id ?? null,
  });
  return merged;
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
    else console.error("Failed to fetch messages:", error);
    store.getState().setMessages(taskSessionId, []);
    lastFetchedSessionIdRef.current = taskSessionId;
  } finally {
    store.getState().setMessagesLoading(taskSessionId, false);
    setIsLoading(false);
  }
}

function useTerminalStateFetch(
  taskSessionId: string | null,
  taskSessionState: TaskSessionState | null,
  hasAgentMessage: boolean,
  refs: {
    store: ReturnType<typeof useAppStoreApi>;
    setIsLoading: (v: boolean) => void;
    setIsWaitingForInitialMessages: (v: boolean) => void;
    initialFetchStartRef: MutableRefObject<number | null>;
    lastFetchedSessionIdRef: MutableRefObject<string | null>;
  },
) {
  const lastFetchStateKeyRef = useRef<string | null>(null);
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!taskSessionId || connectionStatus !== "connected") return;
    if (!taskSessionState || hasAgentMessage) return;

    const terminalStates = new Set<TaskSessionState>(["WAITING_FOR_INPUT", "COMPLETED", "FAILED"]);
    if (!terminalStates.has(taskSessionState)) return;

    const key = `${taskSessionId}:${taskSessionState}`;
    if (lastFetchStateKeyRef.current === key) return;
    lastFetchStateKeyRef.current = key;

    void doFetchMessages({
      taskSessionId,
      ...refs,
      onError: (error) => console.error("Failed to fetch messages after state change:", error),
    });
  }, [taskSessionId, taskSessionState, hasAgentMessage, connectionStatus, refs]);
}

// Silent WS disconnects (NAT timeout, laptop sleep, suspended tab) leave
// connectionStatus stuck at "connected" and no resubscribe fires. Backfill
// whenever the tab regains visibility to recover missed messages without
// requiring a page refresh.
export function useVisibilityBackfill(
  taskSessionId: string | null,
  store: ReturnType<typeof useAppStoreApi>,
) {
  useEffect(() => {
    if (!taskSessionId) return;
    const onVisible = () => {
      if (document.visibilityState === "visible") {
        fetchAndStoreMessages(taskSessionId, store).catch(() => {});
      }
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [taskSessionId, store]);
}

function useSessionSubscription(
  taskSessionId: string | null,
  connectionStatus: string,
  isSessionStartingOrUnknown: boolean,
  store: ReturnType<typeof useAppStoreApi>,
) {
  useEffect(() => {
    if (!taskSessionId || connectionStatus !== "connected") {
      return;
    }
    const client = getWebSocketClient();
    if (!client) {
      return;
    }
    const unsubscribe = client.subscribeSession(taskSessionId);

    // Re-fetch messages after subscribing to close the gap between SSR
    // (which may have run before the agent responded) and this subscription.
    fetchAndStoreMessages(taskSessionId, store).catch(() => {});

    return () => {
      unsubscribe();
    };
  }, [taskSessionId, connectionStatus, store, isSessionStartingOrUnknown]);
}

export function useSessionMessages(taskSessionId: string | null): UseSessionMessagesReturn {
  const store = useAppStoreApi();
  const messages = useAppStore((state) =>
    taskSessionId ? (state.messages.bySession[taskSessionId] ?? EMPTY_MESSAGES) : EMPTY_MESSAGES,
  );
  const messagesMeta = useAppStore((state) =>
    taskSessionId ? (state.messages.metaBySession[taskSessionId] ?? EMPTY_META) : EMPTY_META,
  );
  const taskSessionState = useAppStore((state) =>
    taskSessionId ? (state.taskSessions.items[taskSessionId]?.state ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [isLoading, setIsLoading] = useState(false);
  const [isWaitingForInitialMessages, setIsWaitingForInitialMessages] = useState(false);
  const initialFetchStartRef = useRef<number | null>(null);
  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const prevSessionIdRef = useRef<string | null>(null);
  const hasAgentMessage = messages.some((message: Message) => message.author_type === "agent");

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
    if (!taskSessionId || connectionStatus !== "connected") return;

    const isFreshMount = prevSessionIdRef.current === null;
    const sessionChanged =
      prevSessionIdRef.current !== null && prevSessionIdRef.current !== taskSessionId;
    prevSessionIdRef.current = taskSessionId;

    if (sessionChanged) {
      lastFetchedSessionIdRef.current = null;
    }

    // Normal re-render with cached messages — skip fetch
    if (messages.length > 0 && !sessionChanged && !isFreshMount) {
      lastFetchedSessionIdRef.current = taskSessionId;
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsWaitingForInitialMessages(false);
      return;
    }

    // Fresh mount with cached messages — show cached instantly, fetch in background
    if (isFreshMount && messages.length > 0) {
      lastFetchedSessionIdRef.current = taskSessionId;
      setIsWaitingForInitialMessages(false);
      fetchAndStoreMessages(taskSessionId, store).catch(() => {});
      return;
    }

    if (lastFetchedSessionIdRef.current === taskSessionId) return;

    void doFetchMessages({
      taskSessionId,
      store,
      setIsLoading,
      setIsWaitingForInitialMessages,
      initialFetchStartRef,
      lastFetchedSessionIdRef,
    });
  }, [taskSessionId, connectionStatus, messages.length, store]);

  // Bool flips exactly once when a freshly-adopted session leaves STARTING,
  // so the subscription effect re-runs then (covering the backend race where
  // session.subscribe arrives before the session is fully constructed) without
  // churning on every subsequent RUNNING ↔ WAITING_FOR_INPUT transition.
  const isSessionStartingOrUnknown = taskSessionState === null || taskSessionState === "STARTING";

  useSessionSubscription(taskSessionId, connectionStatus, isSessionStartingOrUnknown, store);
  useVisibilityBackfill(taskSessionId, store);

  const terminalFetchRefs = useMemo(
    () => ({
      store,
      setIsLoading,
      setIsWaitingForInitialMessages,
      initialFetchStartRef,
      lastFetchedSessionIdRef,
    }),
    [store],
  );
  useTerminalStateFetch(taskSessionId, taskSessionState, hasAgentMessage, terminalFetchRefs);

  return {
    isLoading: isLoading || isWaitingForInitialMessages || messagesMeta.isLoading,
    messages,
    hasMore: messagesMeta.hasMore,
    oldestCursor: messagesMeta.oldestCursor,
  };
}
