import { QueryClient } from "@tanstack/react-query";
import { useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  sessionMessagesLatestQueryOptions,
  sessionMessagesQueryOptions,
} from "@/lib/query/query-options";
import type { Message, TaskSessionState } from "@/lib/types/http";

export const INITIAL_FETCH_LIMIT = 100;
const BACKFILL_PAGE_LIMIT = 100;
export const MAX_AUTO_BACKFILL_PAGES = 10;

const standaloneMessageQueryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

const lastAppliedFetchSeq = new Map<string, number>();
const debug = createDebugLogger("messages:fetch");
let fetchSeqCounter = 0;

export function nextFetchSeq(): number {
  fetchSeqCounter += 1;
  return fetchSeqCounter;
}

export function commitFetchSeq(sessionId: string, seq: number): boolean {
  const last = lastAppliedFetchSeq.get(sessionId) ?? 0;
  if (seq < last) return false;
  lastAppliedFetchSeq.set(sessionId, seq);
  return true;
}

export function hasUserOrAgentMessage(messages: Message[]): boolean {
  return messages.some(
    (message) =>
      message.type === "message" &&
      (message.author_type === "user" || message.author_type === "agent"),
  );
}

const ACTIVE_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set(["STARTING", "RUNNING"]);

const SETTLED_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

export function isTurnSettleTransition(
  previousState: TaskSessionState | null,
  nextState: TaskSessionState | null,
): boolean {
  return (
    !!previousState &&
    !!nextState &&
    ACTIVE_SESSION_STATES.has(previousState) &&
    SETTLED_SESSION_STATES.has(nextState)
  );
}

export function hasUserPromptInActiveTurn(messages: Message[], activeTurnId: string | null) {
  return (
    !!activeTurnId &&
    messages.some(
      (message) =>
        message.turn_id === activeTurnId &&
        message.type === "message" &&
        message.author_type === "user",
    )
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
    params.activeTurnId !== null
  );
}

type MessageSummary = {
  count: number;
  byType: Record<string, number>;
  userMessageCount: number;
  agentMessageCount: number;
  oldestCreatedAt: string | null;
  newestCreatedAt: string | null;
};

function summarizeMessages(messages: Message[]): MessageSummary {
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

type MessageListResponse = { messages: Message[]; has_more?: boolean; cursor?: string };

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
      hint: "The fetch limit may be too small for this session's last turn — user prompt and agent replies live further back. Paginate or raise the limit to see them.",
    });
  }
}

export async function fetchAndStoreMessages(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
  queryClient: QueryClient = standaloneMessageQueryClient,
): Promise<Message[]> {
  const seq = nextFetchSeq();
  const requestParams = {
    limit: INITIAL_FETCH_LIMIT,
    sort: "desc" as const,
  };
  debug("message.list request", { session_id: sessionId, ...requestParams });
  const response = await queryClient.fetchQuery({
    ...sessionMessagesLatestQueryOptions(sessionId, requestParams.limit),
    staleTime: 0,
  });
  const fetched = response.messages ?? [];
  logFetchSummary(
    sessionId,
    fetched,
    {
      messages: fetched,
      has_more: response.hasMore,
      cursor: response.oldestCursor ?? undefined,
    },
    requestParams.limit,
  );
  const existing = store.getState().messages.bySession[sessionId] ?? [];
  const fetchedIds = new Set(fetched.map((m) => m.id));
  const extras = existing.filter((m) => !fetchedIds.has(m.id));
  const merged =
    extras.length > 0
      ? [...fetched, ...extras].sort(
          (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
        )
      : fetched;

  if (!commitFetchSeq(sessionId, seq)) {
    debug("message.list stale fetch skipped", { sessionId, seq });
    return store.getState().messages.bySession[sessionId] ?? merged;
  }

  store.getState().mergeMessages(sessionId, merged, {
    hasMore: response.hasMore,
    oldestCursor: merged[0]?.id ?? response.oldestCursor,
  });
  return merged;
}

export type BackfillStep = "continue" | "stop";

async function fetchAndPrependOlder(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
  oldestCursor: string,
  queryClient: QueryClient = standaloneMessageQueryClient,
): Promise<number> {
  const response = await queryClient.fetchQuery({
    ...sessionMessagesQueryOptions(sessionId, {
      limit: BACKFILL_PAGE_LIMIT,
      before: oldestCursor,
      sort: "desc",
    }),
    staleTime: 0,
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
  queryClient: QueryClient = standaloneMessageQueryClient,
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
  store: ReturnType<typeof useAppStoreApi>,
  queryClient: QueryClient = standaloneMessageQueryClient,
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
