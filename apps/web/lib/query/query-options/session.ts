import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { getPlanRevision, getTaskPlan, listPlanRevisions } from "@/lib/api/domains/plan-api";
import { getQueueStatus } from "@/lib/api/domains/queue-api";
import {
  fetchTaskSession,
  listSessionTurns,
  listTaskSessionMessages,
  listTaskSessions,
  searchSessionMessages,
} from "@/lib/api/domains/session-api";
import type { ListTurnsResponse, Message } from "@/lib/types/http";
import { qk } from "../keys";
import { withSignal } from "./utils";

export type SessionMessagesLatestData = {
  messages: Message[];
  hasMore: boolean;
  oldestCursor: string | null;
};

export type SessionTurnsData = ListTurnsResponse & {
  activeTurnId: string | null;
};

export function taskSessionsQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.taskSession.byTask(taskId),
    queryFn: ({ signal }) => listTaskSessions(taskId, withSignal(signal)),
    enabled: Boolean(taskId),
  });
}

export function taskSessionQueryOptions(sessionId: string) {
  return queryOptions({
    queryKey: qk.taskSession.byId(sessionId),
    queryFn: async ({ signal }) => {
      const response = await fetchTaskSession(sessionId, withSignal(signal));
      return response.session;
    },
    enabled: Boolean(sessionId),
  });
}

export function taskPlanQueryOptions(taskId: string, enabled = true) {
  return queryOptions({
    queryKey: qk.taskPlan.detail(taskId),
    queryFn: () => getTaskPlan(taskId),
    enabled: Boolean(taskId && enabled),
    staleTime: 60_000,
  });
}

export function taskPlanRevisionsQueryOptions(taskId: string, enabled = true) {
  return queryOptions({
    queryKey: qk.taskPlan.revisions(taskId),
    queryFn: () => listPlanRevisions(taskId),
    enabled: Boolean(taskId && enabled),
    staleTime: 60_000,
  });
}

export function planRevisionQueryOptions(taskId: string, revisionId: string, enabled = true) {
  return queryOptions({
    queryKey: qk.taskPlan.revision(taskId, revisionId),
    queryFn: () => getPlanRevision(revisionId, taskId),
    enabled: Boolean(taskId && revisionId && enabled),
    staleTime: 60_000,
  });
}

export function queueStatusQueryOptions(sessionId: string, enabled = true) {
  return queryOptions({
    queryKey: qk.session.queue(sessionId),
    queryFn: () => getQueueStatus(sessionId),
    enabled: Boolean(sessionId && enabled),
    staleTime: 30_000,
  });
}

export function sessionMessagesLatestQueryOptions(sessionId: string, limit = 100) {
  return queryOptions({
    queryKey: qk.session.messages(sessionId),
    queryFn: async ({ signal }): Promise<SessionMessagesLatestData> => {
      const response = await listTaskSessionMessages(
        sessionId,
        { limit, sort: "desc" },
        withSignal(signal),
      );
      const messages = [...(response.messages ?? [])].reverse();
      return {
        messages,
        hasMore: response.has_more ?? false,
        oldestCursor: messages[0]?.id ?? response.cursor ?? null,
      };
    },
    enabled: Boolean(sessionId),
    staleTime: 60_000,
  });
}

export function sessionMessagesQueryOptions(
  sessionId: string,
  params: { limit?: number; before?: string; after?: string; sort?: "asc" | "desc" } = {},
) {
  return queryOptions({
    queryKey: qk.session.messagesPage(sessionId, params),
    queryFn: ({ signal }) => listTaskSessionMessages(sessionId, params, withSignal(signal)),
    enabled: Boolean(sessionId),
    staleTime: 60_000,
  });
}

export function sessionMessagesInfiniteQueryOptions(
  sessionId: string,
  params: { limit?: number; sort?: "asc" | "desc" } = {},
) {
  return infiniteQueryOptions({
    queryKey: qk.session.messagesInfinite(sessionId, params),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listTaskSessionMessages(sessionId, { ...params, before: pageParam }, withSignal(signal)),
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.cursor || lastPage.messages[0]?.id : undefined,
    enabled: Boolean(sessionId),
    staleTime: 60_000,
  });
}

export function sessionTurnsQueryOptions(sessionId: string) {
  return queryOptions({
    queryKey: qk.session.turns(sessionId),
    queryFn: async ({ signal }): Promise<SessionTurnsData> => {
      const response = await listSessionTurns(sessionId, withSignal(signal));
      return { ...response, activeTurnId: null };
    },
    enabled: Boolean(sessionId),
    staleTime: 60_000,
  });
}

export function sessionSearchQueryOptions(sessionId: string, query: string, limit = 50) {
  return queryOptions({
    queryKey: qk.session.search(sessionId, query, limit),
    queryFn: () => searchSessionMessages(sessionId, query, limit),
    enabled: Boolean(sessionId && query.trim()),
  });
}
