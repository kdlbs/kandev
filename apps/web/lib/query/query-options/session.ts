/**
 * TanStack Query options for the session domain (Wave 5c).
 *
 * Covers: messages, turns, task sessions, task plans, queue.
 *
 * Active-session protection: session message/turn queries use
 * staleTime: 5 * 60_000 so background refetch doesn't clobber
 * in-flight WS streams. The WS bridge (session.ts) owns freshness.
 */

import { queryOptions, infiniteQueryOptions } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import {
  listTaskSessions,
  listTaskSessionMessages,
  listSessionTurns,
} from "@/lib/api/domains/session-api";
import { getTaskPlan, listPlanRevisions } from "@/lib/api/domains/plan-api";
import { getQueueStatus } from "@/lib/api/domains/queue-api";
import type { Message, Turn, TaskPlan, TaskPlanRevision, TaskSession } from "@/lib/types/http";
import type { QueuedMessage, QueueMeta } from "@/lib/state/slices/session/types";

// ---------------------------------------------------------------------------
// staleTime shared across all session-domain queries
// ---------------------------------------------------------------------------

const SESSION_STALE_TIME = 5 * 60_000;

// ---------------------------------------------------------------------------
// Task sessions (per task)
// ---------------------------------------------------------------------------

export const taskSessionsQueryOptions = (taskId: string) =>
  queryOptions({
    queryKey: qk.taskSession.byTask(taskId),
    queryFn: () => listTaskSessions(taskId),
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Single task session (per session ID) — observe-only cache surface
// ---------------------------------------------------------------------------

/**
 * Observe-only query for a single TaskSession keyed by sessionId.
 *
 * `enabled: false` + a null-returning queryFn: this is not a fetching query.
 * It exists so the ~50 by-id reader sites (state.taskSessions.items[sessionId])
 * can read a dedicated exact-key cache that the session-state bridge and the
 * SSR seed populate. A byTask(taskId) observer never fires on a child
 * byTask(taskId) change, so by-id reads need their own key (mirrors the
 * useMessagesBySessionFromCache observe-only pattern). The session list fetch
 * stays on taskSessionsQueryOptions(taskId).
 */
export const sessionByIdQueryOptions = (sessionId: string) =>
  queryOptions<TaskSession | null>({
    queryKey: qk.taskSession.byId(sessionId),
    queryFn: () => null,
    enabled: false,
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

// ---------------------------------------------------------------------------
// Messages (per session — regular list, bridge patches via setQueryData)
// ---------------------------------------------------------------------------

export type MessagesData = {
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
};

/**
 * Regular query (not infinite) for the session message list.
 *
 * Strategy: the WS bridge pushes incremental message add/update events via
 * setQueryData into this cache key. The hook layer handles visibility refetch
 * and terminal-state refetch by calling invalidateQueries on the same key.
 *
 * The `queryFn` fetches the last 50 messages in ascending order and normalises
 * the response into `MessagesData`. It runs on mount, on WS reconnect
 * invalidation, and when invalidated by the terminal-state effect in the hook.
 */
/**
 * Synthetic, client-only chat messages (e.g. the empty-turn notice) live only in
 * the TanStack Query cache — the backend never returns them. A messages refetch
 * (turn-settle resync, running backfill, visibility refetch) would otherwise drop
 * them. Carry them across a refetch by re-merging the ones still tied to a turn
 * present in the fresh server window.
 */
export function isSyntheticMessage(message: Message): boolean {
  return (message.metadata as { empty_turn?: boolean } | undefined)?.empty_turn === true;
}

export function mergeSyntheticMessages(server: Message[], prev: Message[] | undefined): Message[] {
  if (!prev || prev.length === 0) return server;
  const serverIds = new Set(server.map((m) => m.id));
  const serverTurnIds = new Set(server.map((m) => m.turn_id).filter(Boolean));
  const carried = prev.filter(
    (m) =>
      isSyntheticMessage(m) &&
      !serverIds.has(m.id) &&
      // Only keep notices whose turn is still in the live window, so they don't
      // accumulate forever once their turn scrolls out of the 50-message window.
      (m.turn_id == null || serverTurnIds.has(m.turn_id)),
  );
  if (carried.length === 0) return server;
  return [...server, ...carried];
}

export const sessionMessagesQueryOptions = (sessionId: string) =>
  queryOptions<MessagesData>({
    queryKey: qk.session.messages(sessionId),
    queryFn: async (ctx) => {
      const response = await listTaskSessionMessages(sessionId, { limit: 50, sort: "desc" });
      const serverMessages = [...(response.messages ?? [])].reverse();
      const prev = ctx.client.getQueryData<MessagesData>(qk.session.messages(sessionId));
      const messages = mergeSyntheticMessages(serverMessages, prev?.messages);
      return {
        messages,
        hasMore: response.has_more ?? false,
        oldestCursor: serverMessages[0]?.id ?? null,
      };
    },
    staleTime: SESSION_STALE_TIME,
    // Visibility refetch is opted-in per key (via hook layer), not global.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

// ---------------------------------------------------------------------------
// Messages — infinite variant (backfill older messages)
// ---------------------------------------------------------------------------

/**
 * Infinite query for backwards pagination (load older messages).
 *
 * getNextPageParam returns the oldest cursor from the oldest page so callers
 * can load older batches by passing `before: oldestId`.
 *
 * This is kept separate from sessionMessagesQueryOptions so the primary
 * session message cache (live window) doesn't fight the paginated backfill.
 */
export const sessionMessagesInfiniteQueryOptions = (sessionId: string) =>
  infiniteQueryOptions<
    MessagesData,
    Error,
    MessagesData,
    readonly ["session", string, "messages", "infinite"],
    string | null
  >({
    queryKey: qk.session.messagesInfinite(sessionId),
    queryFn: async ({ pageParam }) => {
      const params: Parameters<typeof listTaskSessionMessages>[1] = {
        limit: 50,
        sort: "desc",
      };
      if (pageParam) params.before = pageParam;
      const response = await listTaskSessionMessages(sessionId, params);
      const messages = [...(response.messages ?? [])].reverse();
      return {
        messages,
        hasMore: response.has_more ?? false,
        oldestCursor: messages[0]?.id ?? null,
      };
    },
    initialPageParam: null,
    getNextPageParam: (firstPage) => (firstPage.hasMore ? firstPage.oldestCursor : null),
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

// ---------------------------------------------------------------------------
// Turns (per session)
// ---------------------------------------------------------------------------

export type TurnsData = {
  turns: Turn[];
  activeTurnId: string | null;
};

/**
 * The active turn is the most recently started turn that hasn't completed.
 * Derived from the turn list so a fetch (or SSR snapshot) knows which turn is
 * live without depending on a `turn.started` WS event having been observed —
 * otherwise a session viewed after its turn began (a fresh load, or a
 * client-side session switch) shows no active turn until the next WS event.
 * Mirrors the live bridge, which sets activeTurnId on `session.turn.started`.
 */
export function deriveActiveTurnId(turns: Turn[]): string | null {
  return turns.filter((t) => !t.completed_at).pop()?.id ?? null;
}

export const sessionTurnsQueryOptions = (sessionId: string) =>
  queryOptions<TurnsData>({
    queryKey: qk.session.turns(sessionId),
    queryFn: async () => {
      const response = await listSessionTurns(sessionId);
      const turns = response.turns ?? [];
      return { turns, activeTurnId: deriveActiveTurnId(turns) };
    },
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Task plans (per task)
// ---------------------------------------------------------------------------

export type TaskPlanData = {
  plan: TaskPlan | null;
  lastSeenUpdatedAt: string | null;
};

export const taskPlanQueryOptions = (taskId: string) =>
  queryOptions<TaskPlanData>({
    queryKey: qk.taskSession.plans(taskId),
    queryFn: async () => {
      const plan = await getTaskPlan(taskId);
      return { plan, lastSeenUpdatedAt: plan?.updated_at ?? null };
    },
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });

// ---------------------------------------------------------------------------
// Task plan revisions (per task)
//
// Metadata-only list, newest first. The bridge upserts individual revisions
// from `task.plan.revision.created`; this query factory backfills the full
// list on mount / cache miss. Revision *content* is fetched on demand via the
// Zustand-side `revisionContentCache` (client-only state, stays in Zustand for
// the staged migration).
// ---------------------------------------------------------------------------

export type TaskPlanRevisionsData = {
  revisions: TaskPlanRevision[];
};

/** Sort revisions newest-first, mirroring the Zustand slice's ordering. */
export function sortRevisionsDesc(revisions: TaskPlanRevision[]): TaskPlanRevision[] {
  return [...revisions].sort((a, b) => b.revision_number - a.revision_number);
}

export const taskPlanRevisionsQueryOptions = (taskId: string) =>
  queryOptions<TaskPlanRevisionsData>({
    queryKey: qk.taskSession.plansRevisions(taskId),
    queryFn: async () => {
      const list = await listPlanRevisions(taskId);
      return { revisions: sortRevisionsDesc(list) };
    },
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Queue (per session)
// ---------------------------------------------------------------------------

export type QueueData = {
  entries: QueuedMessage[];
  meta: QueueMeta;
};

export const sessionQueueQueryOptions = (sessionId: string) =>
  queryOptions<QueueData>({
    queryKey: qk.session.queue(sessionId),
    queryFn: async () => {
      const status = await getQueueStatus(sessionId);
      return {
        entries: status.entries ?? [],
        meta: { count: status.count, max: status.max },
      };
    },
    staleTime: SESSION_STALE_TIME,
    refetchOnWindowFocus: false,
  });

// ---------------------------------------------------------------------------
// Namespace export
// ---------------------------------------------------------------------------

export const sessionQueryOptions = {
  taskSessions: taskSessionsQueryOptions,
  sessionById: sessionByIdQueryOptions,
  messages: sessionMessagesQueryOptions,
  messagesInfinite: sessionMessagesInfiniteQueryOptions,
  turns: sessionTurnsQueryOptions,
  taskPlan: taskPlanQueryOptions,
  taskPlanRevisions: taskPlanRevisionsQueryOptions,
  queue: sessionQueueQueryOptions,
};
