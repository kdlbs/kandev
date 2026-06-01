// Repro: new task → wait env prep → messages should appear without refresh.
// Bug fix: messages are now read from TanStack Query (the WS bridge writes
// `session.message.added` events into qk.session.messages(sid) via setQueryData),
// not from Zustand `messages.bySession` — the latter raced with the initial
// fetchAndStoreMessages call and would render empty for the first user view of
// a freshly-created session. The Zustand mirror is still maintained by the WS
// handler (lib/ws/handlers/messages.ts) for transitional consumers.
import { useEffect, useRef } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskSessionState, Message } from "@/lib/types/http";
import { listTaskSessionMessages } from "@/lib/api";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";
import { qk } from "@/lib/query/keys";
import {
  sessionMessagesQueryOptions,
  sessionTurnsQueryOptions,
} from "@/lib/query/query-options/session";
import { useTaskSessionById } from "./use-task-session-by-id";
import { prependMessagesIntoCache } from "./message-cache";

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

const EMPTY_MESSAGES: Message[] = [];

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

type BackfillStore = {
  getState: () => {
    messages: {
      bySession: Record<string, Message[] | undefined>;
      metaBySession: Record<
        string,
        { hasMore: boolean; oldestCursor: string | null; isLoading: boolean } | undefined
      >;
    };
    prependMessages: (
      sessionId: string,
      messages: Message[],
      meta: { hasMore: boolean; oldestCursor: string | null },
    ) => void;
  };
};

async function fetchAndPrependOlder(
  sessionId: string,
  store: BackfillStore,
  oldestCursor: string,
  queryClient?: QueryClient,
): Promise<number> {
  const response = await listTaskSessionMessages(sessionId, {
    limit: BACKFILL_PAGE_LIMIT,
    before: oldestCursor,
    sort: "desc",
  });
  const ordered = [...(response.messages ?? [])].reverse();
  const newOldestCursor = ordered[0]?.id ?? oldestCursor;
  const meta = { hasMore: response.has_more ?? false, oldestCursor: newOldestCursor };
  store.getState().prependMessages(sessionId, ordered, meta);
  if (queryClient) prependMessagesIntoCache(queryClient, sessionId, ordered, meta);
  return ordered.length;
}

export async function runBackfillRound(
  sessionId: string,
  store: BackfillStore,
  round: number,
  queryClient?: QueryClient,
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
    const added = await fetchAndPrependOlder(sessionId, store, meta.oldestCursor, queryClient);
    return added === 0 ? "stop" : "continue";
  } catch (err) {
    debug("autoBackfill: fetch failed, stopping", { sessionId, round, err });
    return "stop";
  }
}

export async function autoBackfillUntilUserMessage(
  sessionId: string,
  store: BackfillStore,
  queryClient?: QueryClient,
): Promise<void> {
  for (let round = 0; round < MAX_AUTO_BACKFILL_PAGES; round++) {
    const step = await runBackfillRound(sessionId, store, round, queryClient);
    if (step === "stop") return;
  }
  debug("autoBackfill: hit page budget without finding user/agent message", {
    sessionId,
    pageBudget: MAX_AUTO_BACKFILL_PAGES,
    messageBudget: MAX_AUTO_BACKFILL_PAGES * BACKFILL_PAGE_LIMIT,
  });
}

// Silent WS disconnects (NAT timeout, laptop sleep, suspended tab) leave
// connectionStatus stuck at "connected" and no resubscribe fires. Invalidate
// the messages cache whenever the tab regains visibility to recover missed
// messages without requiring a page refresh.
export function useVisibilityBackfill(taskSessionId: string | null, queryClient: QueryClient) {
  useEffect(() => {
    if (!taskSessionId) {
      debug("visibilityBackfill: skipped attaching (no sessionId)");
      return;
    }
    debug("visibilityBackfill: attached", { sessionId: taskSessionId });
    const onVisible = () => {
      const visibilityState = document.visibilityState;
      debug("visibilityBackfill: visibilitychange fired", {
        sessionId: taskSessionId,
        visibilityState,
      });
      if (visibilityState !== "visible") return;
      void queryClient.invalidateQueries({ queryKey: qk.session.messages(taskSessionId) });
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      document.removeEventListener("visibilitychange", onVisible);
      debug("visibilityBackfill: detached", { sessionId: taskSessionId });
    };
  }, [taskSessionId, queryClient]);
}

function useTerminalStateRefetch(
  taskSessionId: string | null,
  taskSessionState: TaskSessionState | null,
  hasAgentMessage: boolean,
  queryClient: QueryClient,
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

    void queryClient.invalidateQueries({ queryKey: qk.session.messages(taskSessionId) });
  }, [taskSessionId, taskSessionState, hasAgentMessage, connectionStatus, queryClient]);
}

function useSessionSubscription(
  taskSessionId: string | null,
  connectionStatus: string,
  isSessionStartingOrUnknown: boolean,
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

    // Re-invalidate the messages cache after subscribing to close any gap
    // between SSR / first mount (which may have run before the agent
    // responded) and this subscription. TQ refetches on the same key.
    void queryClient.invalidateQueries({ queryKey: qk.session.messages(taskSessionId) });

    return () => {
      debug("subscription: unsubscribing", { sessionId: taskSessionId });
      unsubscribe();
    };
  }, [taskSessionId, connectionStatus, queryClient, isSessionStartingOrUnknown]);
}

function useAutoBackfillOnLoad(
  taskSessionId: string | null,
  messages: Message[],
  isFetched: boolean,
  store: BackfillStore,
  queryClient: QueryClient,
) {
  const lastBackfilledRef = useRef<string | null>(null);
  useEffect(() => {
    if (!taskSessionId || !isFetched) return;
    if (messages.length === 0) return;
    if (hasUserOrAgentMessage(messages)) return;
    if (lastBackfilledRef.current === taskSessionId) return;
    lastBackfilledRef.current = taskSessionId;
    if (IS_DEBUG) debug("autoBackfill: kicking off", summarizeMessages(messages));
    void autoBackfillUntilUserMessage(taskSessionId, store, queryClient);
  }, [taskSessionId, messages, isFetched, store, queryClient]);
}

/**
 * Refetch messages whenever a turn settles (active → settled). During a resume
 * the agent_boot `script_execution` is created and then marked completed within
 * ~1s, all server-side; if the live session subscription lapsed in that window
 * the completion `session.message.updated` is dropped and the entry renders
 * with a spinner forever (until a manual refresh). The settle transition is
 * delivered globally, so reconciling messages here recovers any session-scoped
 * updates missed while the turn was running.
 *
 * TQ port of main's `useResyncOnTurnSettle`: instead of calling
 * `fetchAndStoreMessages`, invalidate the messages query so TQ refetches and
 * the bridge-fed cache reconciles.
 */
function useResyncOnTurnSettle(
  taskSessionId: string | null,
  taskSessionState: TaskSessionState | null,
  connectionStatus: string,
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
    void queryClient.invalidateQueries({ queryKey: qk.session.messages(taskSessionId) });
  }, [taskSessionId, taskSessionState, connectionStatus, queryClient]);
}

/**
 * While a session is RUNNING and the active turn already carries the user
 * prompt, periodically refetch messages to recover dropped streaming events
 * (restores planning-chat streaming — main's fix #1197). TQ port: invalidate
 * the messages query on a 1.2s initial + 3s interval cadence instead of
 * `fetchAndStoreMessages`.
 */
function useRunningMessageBackfill(
  taskSessionId: string | null,
  shouldBackfill: boolean,
  queryClient: QueryClient,
) {
  useEffect(() => {
    if (!taskSessionId || !shouldBackfill) return;

    const sync = () => {
      debug("running backfill", { sessionId: taskSessionId });
      void queryClient.invalidateQueries({ queryKey: qk.session.messages(taskSessionId) });
    };
    const initial = window.setTimeout(sync, RUNNING_BACKFILL_INITIAL_DELAY_MS);
    const interval = window.setInterval(sync, RUNNING_BACKFILL_INTERVAL_MS);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(interval);
    };
  }, [taskSessionId, shouldBackfill, queryClient]);
}

function adaptStoreForBackfill(store: ReturnType<typeof useAppStoreApi>): BackfillStore {
  return {
    getState: () => {
      const s = store.getState();
      return {
        messages: s.messages,
        prependMessages: s.prependMessages,
      };
    },
  };
}

/**
 * Read the active turn id from the TQ turns cache (derived in the turns
 * queryFn). Observe-only — the turns query is mounted/owned elsewhere; this is
 * a passive read so the running-backfill gate (`shouldRunMessageBackfill`) can
 * see whether the active turn already carries the user prompt.
 */
function useActiveTurnId(taskSessionId: string | null): string | null {
  const { data } = useQuery({
    ...sessionTurnsQueryOptions(taskSessionId ?? ""),
    enabled: false,
  });
  if (!taskSessionId) return null;
  return data?.activeTurnId ?? null;
}

export function useSessionMessages(taskSessionId: string | null): UseSessionMessagesReturn {
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const taskSessionState = useTaskSessionById(taskSessionId)?.state ?? null;
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Source of truth: TanStack Query. The WS bridge writes message.added /
  // message.updated events into this cache key via setQueryData. The queryFn
  // runs on mount and on invalidation (visibility, terminal state, subscribe).
  const {
    data,
    isLoading: isQueryLoading,
    isFetched,
  } = useQuery({
    ...sessionMessagesQueryOptions(taskSessionId ?? ""),
    enabled: !!taskSessionId && connectionStatus === "connected",
  });

  const messages = data?.messages ?? EMPTY_MESSAGES;
  const hasMore = data?.hasMore ?? false;
  const oldestCursor = data?.oldestCursor ?? null;

  const hasAgentMessage = messages.some((message: Message) => message.author_type === "agent");
  // Active turn id is derived in the turns query (TQ-canonical post-migration).
  const activeTurnId = useActiveTurnId(taskSessionId);

  const isSessionStartingOrUnknown = taskSessionState === null || taskSessionState === "STARTING";

  useSessionSubscription(taskSessionId, connectionStatus, isSessionStartingOrUnknown, queryClient);
  useVisibilityBackfill(taskSessionId, queryClient);
  useTerminalStateRefetch(taskSessionId, taskSessionState, hasAgentMessage, queryClient);
  useResyncOnTurnSettle(taskSessionId, taskSessionState, connectionStatus, queryClient);
  // Restore planning-chat streaming (#1197): poll-refetch while a turn runs.
  useRunningMessageBackfill(
    taskSessionId,
    shouldRunMessageBackfill({ taskSessionState, connectionStatus, activeTurnId, messages }),
    queryClient,
  );

  const backfillStore = adaptStoreForBackfill(store);
  useAutoBackfillOnLoad(taskSessionId, messages, isFetched, backfillStore, queryClient);

  // Surface a loading flag while the query hasn't fetched yet OR while the
  // initial window is empty waiting for a freshly-started session's first
  // events. Components key their "isInitialLoading" off `messages.length === 0`,
  // so this aligns with existing UI behavior.
  const isLoading = isQueryLoading || (!!taskSessionId && !isFetched);

  return {
    isLoading,
    messages,
    hasMore,
    oldestCursor,
  };
}
