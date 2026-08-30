import { useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskSessionState, Message } from "@/lib/types/http";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { ensureSessionTurnsLoaded } from "./use-session-turns-hydration";
import {
  useUnknownSessionSubscriptionRetry,
  useUnknownSessionSubscriptionRetryEffect,
} from "./use-session-subscription-retry";
import { doFetchMessages } from "./use-session-message-fetch";
import { reconcileLatestMessageWindow } from "./message-window-reconciliation";
import { t } from "@/lib/i18n";

export { shouldRetryUnknownSessionSubscription } from "./use-session-subscription-retry";
// Test seam: exported for direct unit coverage of hydration dedup/guards.
export { ensureSessionTurnsLoaded } from "./use-session-turns-hydration";

const INITIAL_FETCH_LIMIT = 100;
const RUNNING_BACKFILL_INITIAL_DELAY_MS = 1200;
const RUNNING_BACKFILL_INTERVAL_MS = 5000;

// Monotonic guard against stale concurrent fetches. Each fetch claims an
// increasing sequence number before awaiting; a completion only merges if no
// newer fetch for the same session has already merged, so an older in-flight
// fetch cannot overwrite newer state (WS-delivered messages or a newer fetch's
// snapshot) when two fetches for the same session race.
let fetchSeqCounter = 0;
const lastAppliedFetchSeq = new Map<string, number>();

/** Claims the next monotonic fetch sequence number. */
export function nextFetchSeq(): number {
  fetchSeqCounter += 1;
  return fetchSeqCounter;
}

/**
 * Records that the fetch identified by `seq` is about to merge for `sessionId`.
 * Returns false when a newer fetch has already merged for the session, so the
 * caller can skip the stale merge.
 */
export function commitFetchSeq(sessionId: string, seq: number): boolean {
  const applied = lastAppliedFetchSeq.get(sessionId) ?? 0;
  if (seq < applied) return false;
  lastAppliedFetchSeq.set(sessionId, seq);
  return true;
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
  // The active prompt may be the missed WS frame; start backfill once the turn exists.
  return (
    params.connectionStatus === "connected" &&
    params.taskSessionState === "RUNNING" &&
    params.activeTurnId !== null
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
  isInitialMessagesLoading: boolean;
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
}

type MessageListResponse = { messages: Message[]; has_more?: boolean; cursor?: string };
type InFlightMessageRequest = {
  readiness: Promise<void>;
  promise: Promise<MessageListResponse>;
  cachedAtRequest: Message[];
};

const EMPTY_MESSAGES: Message[] = [];
const EMPTY_META = { isLoading: false, isLoadingMore: false, hasMore: false, oldestCursor: null };
const inFlightMessageRequests = new Map<string, InFlightMessageRequest>();

/** Debug-only summary of a fetch response (no-op unless debug logging is on). */
function logFetchSummary(
  sessionId: string,
  fetched: Message[],
  response: MessageListResponse,
  limit: number,
): void {
  if (!isDebug()) return;
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
      limit,
      hasMore: response.has_more ?? false,
      byType: summary.byType,
      hint: t("task:messageFetchLimitHint"),
    });
  }
}

function requestSessionMessages(
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  sessionId: string,
  readiness: Promise<void>,
  cachedAtRequest: Message[],
): InFlightMessageRequest {
  const existing = inFlightMessageRequests.get(sessionId);
  if (existing?.readiness === readiness) return existing;

  const requestParams = {
    session_id: sessionId,
    limit: INITIAL_FETCH_LIMIT,
    sort: "desc" as const,
  };
  const promise = client.request<MessageListResponse>("message.list", requestParams, 10000);
  const entry = { readiness, promise, cachedAtRequest: [...cachedAtRequest] };
  inFlightMessageRequests.set(sessionId, entry);
  void promise.then(
    () => {
      window.setTimeout(() => {
        if (inFlightMessageRequests.get(sessionId) === entry) {
          inFlightMessageRequests.delete(sessionId);
        }
      }, 0);
    },
    () => {
      window.setTimeout(() => {
        if (inFlightMessageRequests.get(sessionId) === entry) {
          inFlightMessageRequests.delete(sessionId);
        }
      }, 0);
    },
  );
  return entry;
}

/** Fetch latest messages via WS and merge with any that arrived via live notifications. */
async function fetchAndStoreMessages(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
  isActive?: () => boolean,
): Promise<Message[]> {
  const client = getWebSocketClient();
  if (!client) {
    return [];
  }
  // Initial and recovery fetches must not overtake the server-side
  // session.subscribe registration. The client returns an already-resolved
  // promise when no durable subscription is active, preserving fetch behavior
  // for non-session callers while gating this hook's normal subscription path.
  const readiness = client.getSessionSubscriptionReadiness(sessionId);
  await readiness;
  if (isActive && !isActive()) return [];
  // The messages fetch is the session-entry chokepoint: any path that opens a
  // session's transcript must also make its turns resolvable. Start it only
  // after subscription acknowledgement so the REST snapshot cannot race the
  // initial WebSocket subscription registration.
  void ensureSessionTurnsLoaded(sessionId, store, { readiness });
  const cachedAtRequest = store.getState().messages.bySession[sessionId] ?? [];
  const seq = nextFetchSeq();
  // Keep the snapshot on the deduplicated request. Concurrent callers may
  // observe different cache contents, but reconciliation must use the
  // baseline captured by the caller that actually issued the network request.
  const request = requestSessionMessages(client, sessionId, readiness, cachedAtRequest);
  const response = await request.promise;
  if (isActive && !isActive()) return [];
  const fetched = [...(response.messages ?? [])].reverse();
  logFetchSummary(sessionId, fetched, response, INITIAL_FETCH_LIMIT);
  const { messages: merged, oldestCursor } = reconcileLatestMessageWindow({
    cachedAtRequest: request.cachedAtRequest,
    cachedAtResponse: store.getState().messages.bySession[sessionId] ?? [],
    fetched,
  });

  // Stale-fetch guard: if a newer fetch for this session already merged while
  // this one was in flight, skip the merge so the older snapshot can't drop
  // newer messages. The newer state is already in the store.
  if (!commitFetchSeq(sessionId, seq)) {
    debug("message.list stale fetch skipped", { sessionId, seq });
    return store.getState().messages.bySession[sessionId] ?? merged;
  }

  // Idempotent merge: the store reconciles this snapshot against current state,
  // preserving object/array identity for unchanged messages so the periodic
  // refetch doesn't re-render the whole chat (see reconcileMessages).
  store.getState().mergeMessages(sessionId, merged, {
    hasMore: response.has_more ?? false,
    oldestCursor,
  });
  // The store now holds the identity-reconciled array; callers only read length
  // and message content from the return, so `merged` is equivalent.
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
      fetchAndStoreMessages,
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
  useForegroundRefresh(
    () => {
      if (!taskSessionId) return;
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
    },
    Boolean(taskSessionId),
    taskSessionId,
  );
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
    const subscription = client.subscribeSessionWithReady(taskSessionId);
    let active = true;

    // Re-fetch messages after the server acknowledges the subscription to
    // close the gap between SSR (which may have run before the agent
    // responded) and this subscription.
    void subscription.ready
      .then(() => {
        if (!active) return;
        return fetchAndStoreMessages(taskSessionId, store, () => active);
      })
      .catch(() => {});

    return () => {
      active = false;
      debug("subscription: unsubscribing", { sessionId: taskSessionId });
      subscription.unsubscribe();
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

function useSessionLifecycleSubscriptions(params: {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  connectionStatus: string;
  activeTurnId: string | null;
  messages: Message[];
  store: ReturnType<typeof useAppStoreApi>;
}) {
  const { taskSessionId, taskSessionState, connectionStatus, activeTurnId, messages, store } =
    params;
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

  useSessionSubscription(taskSessionId, connectionStatus, isSessionStartingOrUnknown, store);
  useUnknownSessionSubscriptionRetryEffect({
    taskSessionId,
    connectionStatus,
    retryToken: unknownSessionRetryToken,
  });
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

  useSessionLifecycleSubscriptions({
    taskSessionId,
    taskSessionState,
    connectionStatus,
    activeTurnId,
    messages,
    store,
  });

  useEffect(() => {
    if (!taskSessionId) {
      initialFetchStartRef.current = null;
      lastFetchedSessionIdRef.current = null;
      setIsWaitingForInitialMessages(false);
      return;
    }
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
    let active = true;
    const deactivate = () => {
      active = false;
    };
    if (!taskSessionId || connectionStatus !== "connected") {
      return deactivate;
    }

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
      return deactivate;
    }

    // Fresh mount with cached messages — show cached instantly, fetch in background
    if (isFreshMount && messages.length > 0) {
      lastFetchedSessionIdRef.current = taskSessionId;
      setIsWaitingForInitialMessages(false);
      fetchAndStoreMessages(taskSessionId, store, () => active).catch(() => {});
      return deactivate;
    }

    if (lastFetchedSessionIdRef.current === taskSessionId) {
      return deactivate;
    }

    void doFetchMessages({
      taskSessionId,
      ...fetchRefs,
      fetchAndStoreMessages,
      isActive: () => active,
    });
    return deactivate;
  }, [
    taskSessionId,
    connectionStatus,
    messages.length,
    store,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
    fetchRefs,
  ]);

  useVisibilityBackfill(taskSessionId, store);

  useTerminalStateFetch(taskSessionId, taskSessionState, hasAgentMessage, fetchRefs);

  return {
    isLoading: isLoading || isWaitingForInitialMessages || messagesMeta.isLoading,
    isInitialMessagesLoading: isWaitingForInitialMessages,
    messages,
    hasMore: messagesMeta.hasMore,
    oldestCursor: messagesMeta.oldestCursor,
  };
}
