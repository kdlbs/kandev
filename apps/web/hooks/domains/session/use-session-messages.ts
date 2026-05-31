import { useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskSessionState, Message } from "@/lib/types/http";
import { listTaskSessionMessages } from "@/lib/api";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const INITIAL_FETCH_LIMIT = 100;
const BACKFILL_PAGE_LIMIT = 100;
const RUNNING_BACKFILL_INITIAL_DELAY_MS = 1200;
const RUNNING_BACKFILL_INTERVAL_MS = 3000;
export const MAX_AUTO_BACKFILL_PAGES = 10;

export function hasUserOrAgentMessage(messages: Message[]): boolean {
  return messages.some(
    (m) => m.type === "message" && (m.author_type === "user" || m.author_type === "agent"),
  );
}

// States where a turn (or the agent boot) is actively progressing.
const ACTIVE_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set(["STARTING", "RUNNING"]);
// States the session settles into once a turn finishes.
const SETTLED_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

/**
 * True when the session just left an active state for a settled one — i.e. a
 * turn (or a resume's agent boot) finished. Session-scoped message updates
 * emitted as the turn winds down (e.g. the `agent_boot` `script_execution`
 * completion during a resume) can be missed if the live subscription lapsed
 * during the resume churn, so this is the signal to refetch and reconcile.
 *
 * `state_changed` / `turn.completed` are broadcast globally (not session-scoped),
 * so the client always observes this transition even when its session
 * subscription was dropped.
 */
export function isTurnSettleTransition(
  prev: TaskSessionState | null,
  next: TaskSessionState | null,
): boolean {
  if (prev === null || next === null) return false;
  return ACTIVE_SESSION_STATES.has(prev) && SETTLED_SESSION_STATES.has(next);
}

export function hasUserPromptInActiveTurn(messages: Message[], activeTurnId: string | null) {
  if (!activeTurnId) return false;
  return messages.some(
    (m) => m.turn_id === activeTurnId && m.type === "message" && m.author_type === "user",
  );
}

export function shouldRunMessageBackfill(params: {
  taskSessionState: TaskSessionState | null;
  connectionStatus: string;
  activeTurnId: string | null;
  messages: Message[];
}) {
  return (
    params.connectionStatus === "connected" &&
    params.taskSessionState === "RUNNING" &&
    hasUserPromptInActiveTurn(params.messages, params.activeTurnId)
  );
}

const debug = createDebugLogger("messages:fetch");

function summarizeMessages(messages: Message[]): {
  count: number;
  byType: Record<string, number>;
  userMessageCount: number;
  agentMessageCount: number;
  oldestCreatedAt: string | null;
  newestCreatedAt: string | null;
} {
  const byType: Record<string, number> = {};
  let userMessageCount = 0;
  let agentMessageCount = 0;
  for (const m of messages) {
    const t = m.type ?? "unknown";
    byType[t] = (byType[t] ?? 0) + 1;
    if (m.type === "message" && m.author_type === "user") userMessageCount++;
    if (m.type === "message" && m.author_type === "agent") agentMessageCount++;
  }
  return {
    count: messages.length,
    byType,
    userMessageCount,
    agentMessageCount,
    oldestCreatedAt: messages[0]?.created_at ?? null,
    newestCreatedAt: messages[messages.length - 1]?.created_at ?? null,
  };
}

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

  const requestParams = {
    session_id: sessionId,
    limit: INITIAL_FETCH_LIMIT,
    sort: "desc" as const,
  };
  debug("message.list request", requestParams);
  const response = await client.request<MessageListResponse>("message.list", requestParams, 10000);
  const fetched = [...(response.messages ?? [])].reverse();
  if (IS_DEBUG) {
    const summary = summarizeMessages(fetched);
    debug("message.list response", {
      sessionId,
      hasMore: response.has_more ?? false,
      cursor: response.cursor ?? null,
      ...summary,
    });
    if (fetched.length > 0 && summary.userMessageCount === 0 && summary.agentMessageCount === 0) {
      debug("WARNING: fetched window contains no user/agent message rows", {
        sessionId,
        limit: requestParams.limit,
        hasMore: response.has_more ?? false,
        byType: summary.byType,
        hint: "The fetch limit may be too small for this session's last turn — user prompt and agent replies live further back. Paginate or raise the limit to see them.",
      });
    }
  }
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

/**
 * When the initial fetch window contains no user/agent message rows (common
 * when the latest turn produced hundreds of tool calls), the chat would render
 * as an opaque collapsed activity group with nothing meaningful to scroll
 * past — the lazy-load sentinel at the top of the list never fires because
 * the user has no anchor to scroll from. Paginate backward via the same HTTP
 * endpoint `useLazyLoadMessages` uses until we span at least one user/agent
 * message or hit the page budget.
 */
export type BackfillStep = "continue" | "stop";

async function fetchAndPrependOlder(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
  oldestCursor: string,
): Promise<number> {
  const response = await listTaskSessionMessages(sessionId, {
    limit: BACKFILL_PAGE_LIMIT,
    before: oldestCursor,
    sort: "desc",
  });
  const ordered = [...(response.messages ?? [])].reverse();
  const newOldestCursor = ordered[0]?.id ?? oldestCursor;
  store.getState().prependMessages(sessionId, ordered, {
    hasMore: response.has_more ?? false,
    oldestCursor: newOldestCursor,
  });
  return ordered.length;
}

export async function runBackfillRound(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
  round: number,
): Promise<BackfillStep> {
  const meta = store.getState().messages.metaBySession[sessionId];
  const messages = store.getState().messages.bySession[sessionId] ?? [];
  if (hasUserOrAgentMessage(messages)) return "stop";
  if (!meta?.hasMore || !meta.oldestCursor) {
    debug("autoBackfill: stopping (no more older messages)", {
      sessionId,
      round,
      hasMore: meta?.hasMore ?? false,
    });
    return "stop";
  }
  debug("autoBackfill: window has no user/agent message, fetching older", {
    sessionId,
    round,
    currentCount: messages.length,
    oldestCursor: meta.oldestCursor,
  });
  try {
    const added = await fetchAndPrependOlder(sessionId, store, meta.oldestCursor);
    return added === 0 ? "stop" : "continue";
  } catch (err) {
    debug("autoBackfill: fetch failed, stopping", { sessionId, round, err });
    return "stop";
  }
}

export async function autoBackfillUntilUserMessage(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
): Promise<void> {
  for (let round = 0; round < MAX_AUTO_BACKFILL_PAGES; round++) {
    const step = await runBackfillRound(sessionId, store, round);
    if (step === "stop") return;
  }
  debug("autoBackfill: hit page budget without finding user/agent message", {
    sessionId,
    pageBudget: MAX_AUTO_BACKFILL_PAGES,
    messageBudget: MAX_AUTO_BACKFILL_PAGES * BACKFILL_PAGE_LIMIT,
  });
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
    if (fetched.length > 0 && !hasUserOrAgentMessage(fetched)) {
      await autoBackfillUntilUserMessage(taskSessionId, store);
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
      fetchAndStoreMessages(taskSessionId, store)
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
  }, [taskSessionId, store]);
}

function useSessionSubscription(
  taskSessionId: string | null,
  connectionStatus: string,
  isSessionStartingOrUnknown: boolean,
  store: ReturnType<typeof useAppStoreApi>,
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
    fetchAndStoreMessages(taskSessionId, store).catch(() => {});

    return () => {
      debug("subscription: unsubscribing", { sessionId: taskSessionId });
      unsubscribe();
    };
  }, [taskSessionId, connectionStatus, store, isSessionStartingOrUnknown]);
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
    fetchAndStoreMessages(taskSessionId, store).catch(() => {});
  }, [taskSessionId, taskSessionState, connectionStatus, store]);
}

function useRunningMessageBackfill(
  taskSessionId: string | null,
  shouldBackfill: boolean,
  store: ReturnType<typeof useAppStoreApi>,
) {
  useEffect(() => {
    if (!taskSessionId || !shouldBackfill) return;

    let inFlight = false;
    const sync = () => {
      if (inFlight) return;
      inFlight = true;
      debug("running backfill", { sessionId: taskSessionId });
      fetchAndStoreMessages(taskSessionId, store)
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
  }, [taskSessionId, shouldBackfill, store]);
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
  const messages = useAppStore((state) =>
    taskSessionId ? (state.messages.bySession[taskSessionId] ?? EMPTY_MESSAGES) : EMPTY_MESSAGES,
  );
  const messagesMeta = useAppStore((state) =>
    taskSessionId ? (state.messages.metaBySession[taskSessionId] ?? EMPTY_META) : EMPTY_META,
  );
  const taskSessionState = useAppStore((state) =>
    taskSessionId ? (state.taskSessions.items[taskSessionId]?.state ?? null) : null,
  );
  const activeTurnId = useAppStore((state) =>
    taskSessionId ? (state.turns.activeBySession[taskSessionId] ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  return { messages, messagesMeta, taskSessionState, activeTurnId, connectionStatus };
}

export function useSessionMessages(taskSessionId: string | null): UseSessionMessagesReturn {
  const store = useAppStoreApi();
  const { messages, messagesMeta, taskSessionState, activeTurnId, connectionStatus } =
    useSessionMessageInputs(taskSessionId);
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
    if (messages.length > 0) {
      setIsWaitingForInitialMessages(false);
      return;
    }
    if (initialFetchStartRef.current === null) {
      initialFetchStartRef.current = Date.now();
      setIsWaitingForInitialMessages(true);
    }
  }, [taskSessionId, messages.length, initialFetchStartRef, setIsWaitingForInitialMessages]);

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
      fetchAndStoreMessages(taskSessionId, store).catch(() => {});
      return;
    }

    if (lastFetchedSessionIdRef.current === taskSessionId) return;

    void doFetchMessages({
      taskSessionId,
      ...fetchRefs,
    });
  }, [
    taskSessionId,
    connectionStatus,
    messages.length,
    store,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
    fetchRefs,
  ]);

  // Bool flips exactly once when a freshly-adopted session leaves STARTING,
  // so the subscription effect re-runs then (covering the backend race where
  // session.subscribe arrives before the session is fully constructed) without
  // churning on every subsequent RUNNING ↔ WAITING_FOR_INPUT transition.
  const isSessionStartingOrUnknown = taskSessionState === null || taskSessionState === "STARTING";

  useSessionSubscription(taskSessionId, connectionStatus, isSessionStartingOrUnknown, store);
  useResyncOnTurnSettle(taskSessionId, taskSessionState, connectionStatus, store);
  useRunningMessageBackfill(
    taskSessionId,
    shouldRunMessageBackfill({
      taskSessionState,
      connectionStatus,
      activeTurnId,
      messages,
    }),
    store,
  );
  useVisibilityBackfill(taskSessionId, store);

  useTerminalStateFetch(taskSessionId, taskSessionState, hasAgentMessage, fetchRefs);

  return {
    isLoading: isLoading || isWaitingForInitialMessages || messagesMeta.isLoading,
    messages,
    hasMore: messagesMeta.hasMore,
    oldestCursor: messagesMeta.oldestCursor,
  };
}
