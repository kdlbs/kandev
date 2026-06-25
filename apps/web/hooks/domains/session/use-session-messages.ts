import { useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskSessionState, Message } from "@/lib/types/http";
import {
  sessionMessagesLatestQueryOptions,
  sessionTurnsQueryOptions,
  taskSessionQueryOptions,
} from "@/lib/query/query-options";
import { createDebugLogger } from "@/lib/debug/log";
import {
  useUnknownSessionSubscriptionRetry,
  useUnknownSessionSubscriptionRetryEffect,
} from "./use-session-subscription-retry";
import {
  autoBackfillUntilUserMessage,
  fetchAndStoreMessages,
  hasUserOrAgentMessage,
  INITIAL_FETCH_LIMIT,
  isTurnSettleTransition,
  shouldRunMessageBackfill,
} from "./session-message-fetch";

export { shouldRetryUnknownSessionSubscription } from "./use-session-subscription-retry";
export {
  autoBackfillUntilUserMessage,
  commitFetchSeq,
  hasUserOrAgentMessage,
  hasUserPromptInActiveTurn,
  isTurnSettleTransition,
  MAX_AUTO_BACKFILL_PAGES,
  nextFetchSeq,
  runBackfillRound,
  shouldRunMessageBackfill,
} from "./session-message-fetch";

const RUNNING_BACKFILL_INITIAL_DELAY_MS = 1200;
const RUNNING_BACKFILL_INTERVAL_MS = 5000;

const debug = createDebugLogger("messages:fetch");

interface UseSessionMessagesReturn {
  isLoading: boolean;
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
}

const EMPTY_MESSAGES: Message[] = [];
const EMPTY_META = { isLoading: false, hasMore: false, oldestCursor: null };

type FetchMessagesParams = {
  taskSessionId: string;
  store: ReturnType<typeof useAppStoreApi>;
  queryClient: QueryClient;
  setIsLoading: (v: boolean) => void;
  setIsWaitingForInitialMessages: (v: boolean) => void;
  initialFetchStartRef: MutableRefObject<number | null>;
  lastFetchedSessionIdRef: MutableRefObject<string | null>;
  onError?: (error: unknown) => void;
};

async function doFetchMessages({
  taskSessionId,
  store,
  queryClient,
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
    const fetched = await fetchAndStoreMessages(taskSessionId, store, queryClient);
    lastFetchedSessionIdRef.current = taskSessionId;
    if (fetched.length > 0) setIsWaitingForInitialMessages(false);
    if (fetched.length > 0 && !hasUserOrAgentMessage(fetched)) {
      await autoBackfillUntilUserMessage(taskSessionId, store, queryClient);
    }
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
    queryClient: QueryClient;
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
  queryClient: QueryClient,
) {
  useEffect(() => {
    if (!taskSessionId) {
      debug("visibilityBackfill: skipped attaching (no sessionId)");
      return;
    }
    debug("visibilityBackfill: attached", { sessionId: taskSessionId });
    const onVisible = () => {
      const visibilityState = document.visibilityState;
      const state = store.getState();
      const existingCount = state.messages.bySession[taskSessionId]?.length ?? 0;
      const newestBefore =
        state.messages.bySession[taskSessionId]?.slice(-1)[0]?.created_at ?? null;
      debug("visibilityBackfill: visibilitychange fired", {
        sessionId: taskSessionId,
        visibilityState,
        connectionStatus: state.connection?.status ?? "unknown",
        existingCount,
        newestBefore,
      });
      if (visibilityState !== "visible") return;
      fetchAndStoreMessages(taskSessionId, store, queryClient)
        .then(() => {
          const afterCount = store.getState().messages.bySession[taskSessionId]?.length ?? 0;
          const newestAfter =
            store.getState().messages.bySession[taskSessionId]?.slice(-1)[0]?.created_at ?? null;
          debug("visibilityBackfill: refetch complete", {
            sessionId: taskSessionId,
            delta: afterCount - existingCount,
            newestBefore,
            newestAfter,
          });
        })
        .catch((err) => {
          debug("visibilityBackfill: refetch failed", { sessionId: taskSessionId, err });
        });
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      document.removeEventListener("visibilitychange", onVisible);
      debug("visibilityBackfill: detached", { sessionId: taskSessionId });
    };
  }, [taskSessionId, store, queryClient]);
}

function useSessionSubscription(
  taskSessionId: string | null,
  connectionStatus: string,
  isSessionStartingOrUnknown: boolean,
  store: ReturnType<typeof useAppStoreApi>,
  queryClient: QueryClient,
) {
  useEffect(() => {
    debug("subscription: effect ran", {
      sessionId: taskSessionId,
      connectionStatus,
      isSessionStartingOrUnknown,
    });
    if (!taskSessionId || connectionStatus !== "connected") {
      debug("subscription: skipped (no session or not connected)", {
        sessionId: taskSessionId,
        connectionStatus,
      });
      return;
    }
    const client = getWebSocketClient();
    if (!client) {
      debug("subscription: skipped (no ws client)", { sessionId: taskSessionId });
      return;
    }
    debug("subscription: subscribing", { sessionId: taskSessionId });
    const unsubscribe = client.subscribeSession(taskSessionId);

    // Re-fetch messages after subscribing to close the gap between SSR
    // (which may have run before the agent responded) and this subscription.
    fetchAndStoreMessages(taskSessionId, store, queryClient).catch(() => {});

    return () => {
      debug("subscription: unsubscribing", { sessionId: taskSessionId });
      unsubscribe();
    };
  }, [taskSessionId, connectionStatus, store, isSessionStartingOrUnknown, queryClient]);
}

/**
 * Refetch messages whenever a turn settles (active → settled). During a resume
 * the agent_boot `script_execution` is created and then marked completed within
 * ~1s, all server-side; if the live session subscription lapsed in that window
 * the completion `session.message.updated` is dropped and the entry renders
 * with a spinner forever (until a manual refresh). The settle transition is
 * delivered globally, so reconciling messages here recovers any session-scoped
 * updates missed while the turn was running.
 */
function useResyncOnTurnSettle(
  taskSessionId: string | null,
  taskSessionState: TaskSessionState | null,
  connectionStatus: string,
  store: ReturnType<typeof useAppStoreApi>,
  queryClient: QueryClient,
) {
  const prevRef = useRef<{ sessionId: string | null; state: TaskSessionState | null }>({
    sessionId: null,
    state: null,
  });
  useEffect(() => {
    const prev = prevRef.current;
    prevRef.current = { sessionId: taskSessionId, state: taskSessionState };
    if (!taskSessionId || connectionStatus !== "connected") return;
    const prevState = prev.sessionId === taskSessionId ? prev.state : null;
    if (!isTurnSettleTransition(prevState, taskSessionState)) return;
    debug("resync on turn settle", {
      sessionId: taskSessionId,
      prev: prevState,
      next: taskSessionState,
    });
    fetchAndStoreMessages(taskSessionId, store, queryClient).catch(() => {});
  }, [taskSessionId, taskSessionState, connectionStatus, store, queryClient]);
}

function useRunningMessageBackfill(
  taskSessionId: string | null,
  shouldBackfill: boolean,
  store: ReturnType<typeof useAppStoreApi>,
  queryClient: QueryClient,
) {
  useEffect(() => {
    if (!taskSessionId || !shouldBackfill) return;

    let inFlight = false;
    const sync = () => {
      if (inFlight) return;
      inFlight = true;
      debug("running backfill", { sessionId: taskSessionId });
      fetchAndStoreMessages(taskSessionId, store, queryClient)
        .catch((err) => {
          debug("running backfill failed", { sessionId: taskSessionId, err });
        })
        .finally(() => {
          inFlight = false;
        });
    };
    const initial = window.setTimeout(sync, RUNNING_BACKFILL_INITIAL_DELAY_MS);
    const interval = window.setInterval(sync, RUNNING_BACKFILL_INTERVAL_MS);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(interval);
    };
  }, [taskSessionId, shouldBackfill, store, queryClient]);
}

function useMessageFetchState(store: ReturnType<typeof useAppStoreApi>) {
  const [isLoading, setIsLoading] = useState(false);
  const [isWaitingForInitialMessages, setIsWaitingForInitialMessages] = useState(false);
  const initialFetchStartRef = useRef<number | null>(null);
  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const refs = useMemo(
    () => ({
      store,
      setIsLoading,
      setIsWaitingForInitialMessages,
      initialFetchStartRef,
      lastFetchedSessionIdRef,
    }),
    [store],
  );
  return {
    isLoading,
    isWaitingForInitialMessages,
    setIsWaitingForInitialMessages,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    refs,
  };
}

function useSessionMessageInputs(taskSessionId: string | null) {
  const sessionQuery = useQuery(taskSessionQueryOptions(taskSessionId ?? ""));
  const turnsQuery = useQuery(sessionTurnsQueryOptions(taskSessionId ?? ""));
  const messages = useAppStore((state) =>
    taskSessionId ? (state.messages.bySession[taskSessionId] ?? EMPTY_MESSAGES) : EMPTY_MESSAGES,
  );
  const messagesMeta = useAppStore((state) =>
    taskSessionId ? (state.messages.metaBySession[taskSessionId] ?? EMPTY_META) : EMPTY_META,
  );
  const storeTaskSessionState = useAppStore((state) =>
    taskSessionId ? (state.taskSessions.items[taskSessionId]?.state ?? null) : null,
  );
  const storeActiveTurnId = useAppStore((state) =>
    taskSessionId ? (state.turns.activeBySession[taskSessionId] ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const taskSessionState = sessionQuery.data?.state ?? storeTaskSessionState;
  const activeTurnId = turnsQuery.data?.activeTurnId ?? storeActiveTurnId;
  return { messages, messagesMeta, taskSessionState, activeTurnId, connectionStatus };
}

function useSessionLifecycleSubscriptions(params: {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  connectionStatus: string;
  activeTurnId: string | null;
  messages: Message[];
  store: ReturnType<typeof useAppStoreApi>;
  queryClient: QueryClient;
}) {
  const {
    taskSessionId,
    taskSessionState,
    connectionStatus,
    activeTurnId,
    messages,
    store,
    queryClient,
  } = params;
  // Bool flips exactly once when a freshly-adopted session leaves STARTING,
  // so the subscription effect re-runs then (covering the backend race where
  // session.subscribe arrives before the session is fully constructed) without
  // churning on every subsequent RUNNING ↔ WAITING_FOR_INPUT transition.
  const isSessionStartingOrUnknown = taskSessionState === null || taskSessionState === "STARTING";
  const unknownSessionRetryToken = useUnknownSessionSubscriptionRetry({
    taskSessionId,
    taskSessionState,
    connectionStatus,
  });

  useSessionSubscription(
    taskSessionId,
    connectionStatus,
    isSessionStartingOrUnknown,
    store,
    queryClient,
  );
  useUnknownSessionSubscriptionRetryEffect({
    taskSessionId,
    connectionStatus,
    retryToken: unknownSessionRetryToken,
  });
  useResyncOnTurnSettle(taskSessionId, taskSessionState, connectionStatus, store, queryClient);
  useRunningMessageBackfill(
    taskSessionId,
    shouldRunMessageBackfill({
      taskSessionState,
      connectionStatus,
      activeTurnId,
      messages,
    }),
    store,
    queryClient,
  );
}

function useLatestMessagesMerge(
  taskSessionId: string | null,
  latestMessages:
    | { messages: Message[]; hasMore: boolean; oldestCursor: string | null }
    | undefined,
  store: ReturnType<typeof useAppStoreApi>,
) {
  useEffect(() => {
    if (!taskSessionId || !latestMessages) return;
    store.getState().mergeMessages(taskSessionId, latestMessages.messages, {
      hasMore: latestMessages.hasMore,
      oldestCursor: latestMessages.oldestCursor,
    });
  }, [taskSessionId, latestMessages, store]);
}

function useInitialMessageWaitState(params: {
  taskSessionId: string | null;
  messageCount: number;
  initialFetchStartRef: MutableRefObject<number | null>;
  lastFetchedSessionIdRef: MutableRefObject<string | null>;
  setIsWaitingForInitialMessages: (waiting: boolean) => void;
}) {
  const {
    taskSessionId,
    messageCount,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
  } = params;

  useEffect(() => {
    if (!taskSessionId) {
      initialFetchStartRef.current = null;
      lastFetchedSessionIdRef.current = null;
      setIsWaitingForInitialMessages(false);
    }
  }, [
    taskSessionId,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
  ]);

  useEffect(() => {
    if (!taskSessionId) return;
    if (messageCount > 0) {
      setIsWaitingForInitialMessages(false);
      return;
    }
    if (initialFetchStartRef.current === null) {
      initialFetchStartRef.current = Date.now();
      setIsWaitingForInitialMessages(true);
    }
  }, [taskSessionId, messageCount, initialFetchStartRef, setIsWaitingForInitialMessages]);
}

export function useSessionMessages(taskSessionId: string | null): UseSessionMessagesReturn {
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const { messages, messagesMeta, taskSessionState, activeTurnId, connectionStatus } =
    useSessionMessageInputs(taskSessionId);
  const latestMessagesQuery = useQuery(
    sessionMessagesLatestQueryOptions(taskSessionId ?? "", INITIAL_FETCH_LIMIT),
  );
  const prevSessionIdRef = useRef<string | null>(null);
  const hasAgentMessage = messages.some((message: Message) => message.author_type === "agent");
  const {
    isLoading,
    isWaitingForInitialMessages,
    setIsWaitingForInitialMessages,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    refs: fetchRefs,
  } = useMessageFetchState(store);

  useLatestMessagesMerge(taskSessionId, latestMessagesQuery.data, store);
  useInitialMessageWaitState({
    taskSessionId,
    messageCount: messages.length,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
  });

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
      setIsWaitingForInitialMessages(false);
      return;
    }

    // Fresh mount with cached messages — show cached instantly, fetch in background
    if (isFreshMount && messages.length > 0) {
      lastFetchedSessionIdRef.current = taskSessionId;
      setIsWaitingForInitialMessages(false);
      fetchAndStoreMessages(taskSessionId, store, queryClient).catch(() => {});
      return;
    }

    if (lastFetchedSessionIdRef.current === taskSessionId) return;

    void doFetchMessages({
      taskSessionId,
      ...fetchRefs,
      queryClient,
    });
  }, [
    taskSessionId,
    connectionStatus,
    messages.length,
    store,
    queryClient,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
    fetchRefs,
  ]);

  useSessionLifecycleSubscriptions({
    taskSessionId,
    taskSessionState,
    connectionStatus,
    activeTurnId,
    messages,
    store,
    queryClient,
  });

  useVisibilityBackfill(taskSessionId, store, queryClient);

  useTerminalStateFetch(taskSessionId, taskSessionState, hasAgentMessage, {
    ...fetchRefs,
    queryClient,
  });

  return {
    isLoading:
      isLoading ||
      isWaitingForInitialMessages ||
      messagesMeta.isLoading ||
      (latestMessagesQuery.isPending && messages.length === 0),
    messages,
    hasMore: messagesMeta.hasMore,
    oldestCursor: messagesMeta.oldestCursor,
  };
}
