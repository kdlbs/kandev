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
import { getTaskPlan } from "@/lib/api/domains/plan-api";
import { getQueueStatus } from "@/lib/api/domains/queue-api";
import type { Message, Turn, TaskPlan } from "@/lib/types/http";
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
export const sessionMessagesQueryOptions = (sessionId: string) =>
  queryOptions<MessagesData>({
    queryKey: qk.session.messages(sessionId),
    queryFn: async () => {
      const response = await listTaskSessionMessages(sessionId, { limit: 50, sort: "desc" });
      const messages = [...(response.messages ?? [])].reverse();
      return {
        messages,
        hasMore: response.has_more ?? false,
        oldestCursor: messages[0]?.id ?? null,
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

export const sessionTurnsQueryOptions = (sessionId: string) =>
  queryOptions<TurnsData>({
    queryKey: qk.session.turns(sessionId),
    queryFn: async () => {
      const response = await listSessionTurns(sessionId);
      return { turns: response.turns ?? [], activeTurnId: null };
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
  messages: sessionMessagesQueryOptions,
  messagesInfinite: sessionMessagesInfiniteQueryOptions,
  turns: sessionTurnsQueryOptions,
  taskPlan: taskPlanQueryOptions,
  queue: sessionQueueQueryOptions,
};
