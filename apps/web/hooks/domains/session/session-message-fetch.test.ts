import { beforeEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import {
  sessionMessagesLatestQueryOptions,
  type SessionMessagesLatestData,
} from "@/lib/query/query-options";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import type { Message } from "@/lib/types/http";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import {
  commitFetchSeq,
  fetchAndStoreMessages,
  INITIAL_FETCH_LIMIT,
} from "./session-message-fetch";

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listTaskSessions: vi.fn(),
  searchSessionMessages: vi.fn(),
}));

const TASK_ID = toTaskId("task-1");

function makeMessage(id: string, createdAt: string): Message {
  return {
    id,
    task_id: TASK_ID,
    session_id: toSessionId("session-1"),
    author_type: "agent",
    content: id,
    type: "message",
    created_at: createdAt,
  };
}

function makeStore(sessionId: string, messages: Message[]) {
  const state: {
    messages: {
      bySession: Record<string, Message[]>;
      metaBySession: Record<
        string,
        { isLoading: boolean; hasMore: boolean; oldestCursor: string | null }
      >;
    };
    mergeMessages: (
      nextSessionId: string,
      nextMessages: Message[],
      meta?: { hasMore?: boolean; oldestCursor?: string | null },
    ) => void;
  } = {
    messages: {
      bySession: { [sessionId]: messages },
      metaBySession: {},
    },
    mergeMessages: vi.fn(
      (
        nextSessionId: string,
        nextMessages: Message[],
        meta?: { hasMore?: boolean; oldestCursor?: string | null },
      ) => {
        state.messages.bySession[nextSessionId] = nextMessages;
        state.messages.metaBySession[nextSessionId] = {
          isLoading: false,
          hasMore: meta?.hasMore ?? false,
          oldestCursor: meta?.oldestCursor ?? null,
        };
      },
    ),
  };
  return { getState: () => state };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("fetchAndStoreMessages", () => {
  it("writes the merged store-preserved snapshot back to the latest messages query cache", async () => {
    const sessionId = "session-1";
    const fetched = makeMessage("fetched", "2026-01-01T00:00:01Z");
    const wsOnly = makeMessage("ws-only", "2026-01-01T00:00:02Z");
    const queryClient = makeQueryClient();
    const store = makeStore(sessionId, [wsOnly]);

    vi.mocked(listTaskSessionMessages).mockResolvedValueOnce({
      messages: [fetched],
      total: 1,
      has_more: true,
      cursor: "older",
    });

    const result = await fetchAndStoreMessages(sessionId, store as never, queryClient);
    const cached = queryClient.getQueryData<SessionMessagesLatestData>(
      sessionMessagesLatestQueryOptions(sessionId, INITIAL_FETCH_LIMIT).queryKey,
    );

    expect(result.map((message) => message.id)).toEqual(["fetched", "ws-only"]);
    expect(cached?.messages.map((message) => message.id)).toEqual(["fetched", "ws-only"]);
    expect(store.getState().mergeMessages).toHaveBeenCalledWith(
      sessionId,
      expect.arrayContaining([fetched, wsOnly]),
      expect.objectContaining({ hasMore: true, oldestCursor: "fetched" }),
    );
  });

  it("restores the current store snapshot into the query cache when a fetch sequence is stale", async () => {
    const sessionId = "session-stale";
    const fetched = {
      ...makeMessage("fetched", "2026-01-01T00:00:01Z"),
      session_id: toSessionId(sessionId),
    };
    const current = {
      ...makeMessage("current", "2026-01-01T00:00:02Z"),
      session_id: toSessionId(sessionId),
    };
    const queryClient = makeQueryClient();
    const store = makeStore(sessionId, [current]);
    commitFetchSeq(sessionId, 999);

    vi.mocked(listTaskSessionMessages).mockResolvedValueOnce({
      messages: [fetched],
      total: 1,
      has_more: false,
      cursor: "",
    });

    const result = await fetchAndStoreMessages(sessionId, store as never, queryClient);
    const cached = queryClient.getQueryData<SessionMessagesLatestData>(
      sessionMessagesLatestQueryOptions(sessionId, INITIAL_FETCH_LIMIT).queryKey,
    );

    expect(result.map((message) => message.id)).toEqual(["current"]);
    expect(cached?.messages.map((message) => message.id)).toEqual(["current"]);
    expect(store.getState().mergeMessages).not.toHaveBeenCalled();
  });
});
