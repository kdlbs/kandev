import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { Message } from "@/lib/types/http";

const mockListTaskSessionMessages = vi.fn();

vi.mock("@/lib/api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListTaskSessionMessages(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
  useAppStoreApi: () => null,
}));

import { taskId, sessionId } from "@/lib/types/ids";

beforeEach(() => {
  vi.clearAllMocks();
  mockListTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
});

afterEach(() => {
  vi.restoreAllMocks();
});
import {
  hasUserOrAgentMessage,
  runBackfillRound,
  autoBackfillUntilUserMessage,
} from "./use-session-messages";

function makeMessage(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    task_id: taskId("task-1"),
    session_id: sessionId("sess-1"),
    author_type: "user",
    content: "hello",
    type: "message",
    created_at: "2024-01-01T00:00:00Z",
    ...overrides,
  } as Message;
}

/** Stateful store mock — prependMessages actually updates the stored messages. */
function makeStore(options: {
  messages?: Message[];
  hasMore?: boolean;
  oldestCursor?: string | null;
}) {
  let messages: Message[] = options.messages ?? [];
  let meta = {
    hasMore: options.hasMore ?? false,
    oldestCursor: options.oldestCursor ?? null,
    isLoading: false,
  };
  const prependMessages = vi.fn(
    (
      _sessionId: string,
      newMsgs: Message[],
      newMeta: { hasMore: boolean; oldestCursor: string | null },
    ) => {
      messages = [...newMsgs, ...messages];
      meta = { ...meta, ...newMeta };
    },
  );
  return {
    getState: () => ({
      messages: {
        bySession: { "sess-1": messages },
        metaBySession: { "sess-1": meta },
      },
      prependMessages,
    }),
    _prependMessages: prependMessages,
  };
}

describe("hasUserOrAgentMessage", () => {
  it("returns true for a user message", () => {
    expect(hasUserOrAgentMessage([makeMessage({ type: "message", author_type: "user" })])).toBe(
      true,
    );
  });

  it("returns true for an agent message", () => {
    expect(hasUserOrAgentMessage([makeMessage({ type: "message", author_type: "agent" })])).toBe(
      true,
    );
  });

  it("returns false for tool_call only", () => {
    expect(hasUserOrAgentMessage([makeMessage({ type: "tool_call", author_type: "agent" })])).toBe(
      false,
    );
  });

  it("returns false for empty array", () => {
    expect(hasUserOrAgentMessage([])).toBe(false);
  });

  it("returns true when mixed messages include a user message", () => {
    const msgs = [
      makeMessage({ id: "t1", type: "tool_call", author_type: "agent" }),
      makeMessage({ id: "u1", type: "message", author_type: "user" }),
    ];
    expect(hasUserOrAgentMessage(msgs)).toBe(true);
  });
});

describe("runBackfillRound", () => {
  it("returns 'stop' when a user/agent message already exists in the store", async () => {
    const store = makeStore({
      messages: [makeMessage({ type: "message", author_type: "user" })],
      hasMore: true,
      oldestCursor: "msg-1",
    });
    const result = await runBackfillRound("sess-1", store as never, 0);
    expect(result).toBe("stop");
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("returns 'stop' when hasMore is false", async () => {
    const store = makeStore({ messages: [], hasMore: false, oldestCursor: "msg-1" });
    const result = await runBackfillRound("sess-1", store as never, 0);
    expect(result).toBe("stop");
  });

  it("returns 'stop' when oldestCursor is null", async () => {
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: null });
    const result = await runBackfillRound("sess-1", store as never, 0);
    expect(result).toBe("stop");
  });

  it("returns 'continue' and calls prependMessages when older messages are fetched", async () => {
    mockListTaskSessionMessages.mockResolvedValue({
      messages: [makeMessage({ id: "old-1" })],
      has_more: true,
    });
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: "msg-1" });
    const result = await runBackfillRound("sess-1", store as never, 0);
    expect(result).toBe("continue");
    expect(store._prependMessages).toHaveBeenCalled();
  });

  it("returns 'stop' when fetched batch is empty", async () => {
    mockListTaskSessionMessages.mockResolvedValue({ messages: [], has_more: true });
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: "msg-1" });
    const result = await runBackfillRound("sess-1", store as never, 0);
    expect(result).toBe("stop");
  });

  it("returns 'stop' on fetch error", async () => {
    mockListTaskSessionMessages.mockRejectedValue(new Error("network error"));
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: "msg-1" });
    const result = await runBackfillRound("sess-1", store as never, 0);
    expect(result).toBe("stop");
  });
});

describe("autoBackfillUntilUserMessage", () => {
  it("continues past three pages until a user/agent message is found", async () => {
    mockListTaskSessionMessages
      .mockResolvedValueOnce({
        messages: [makeMessage({ id: "tool-1", type: "tool_call", author_type: "agent" })],
        has_more: true,
      })
      .mockResolvedValueOnce({
        messages: [makeMessage({ id: "tool-2", type: "tool_call", author_type: "agent" })],
        has_more: true,
      })
      .mockResolvedValueOnce({
        messages: [makeMessage({ id: "tool-3", type: "tool_call", author_type: "agent" })],
        has_more: true,
      })
      .mockResolvedValueOnce({
        messages: [makeMessage({ id: "tool-4", type: "tool_call", author_type: "agent" })],
        has_more: true,
      })
      .mockResolvedValueOnce({
        messages: [makeMessage({ id: "user-1", type: "message", author_type: "user" })],
        has_more: true,
      });
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: "cursor-0" });

    await autoBackfillUntilUserMessage("sess-1", store as never);

    expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(5);
  });

  it("stops after the auto-backfill page budget without finding a user/agent message", async () => {
    mockListTaskSessionMessages.mockResolvedValue({
      messages: [makeMessage({ id: "t1", type: "tool_call", author_type: "agent" })],
      has_more: true,
    });
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: "cursor-0" });
    await autoBackfillUntilUserMessage("sess-1", store as never);
    expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(10);
  });

  it("stops after round 1 once a user message is prepended", async () => {
    mockListTaskSessionMessages.mockResolvedValue({
      messages: [makeMessage({ id: "u1", type: "message", author_type: "user" })],
      has_more: true,
    });
    const store = makeStore({ messages: [], hasMore: true, oldestCursor: "cursor-0" });
    await autoBackfillUntilUserMessage("sess-1", store as never);
    // After round 0, prependMessages adds the user message; round 1 finds it and stops.
    expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(1);
  });
});
