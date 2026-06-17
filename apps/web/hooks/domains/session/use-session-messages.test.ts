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
  hasUserPromptInActiveTurn,
  hasUserOrAgentMessage,
  isTurnSettleTransition,
  shouldRunMessageBackfill,
  shouldRetryUnknownSessionSubscription,
  runBackfillRound,
  autoBackfillUntilUserMessage,
  nextFetchSeq,
  commitFetchSeq,
  MAX_AUTO_BACKFILL_PAGES,
} from "./use-session-messages";
import type { TaskSessionState } from "@/lib/types/http";

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

describe("isTurnSettleTransition", () => {
  const settled: TaskSessionState[] = [
    "IDLE",
    "WAITING_FOR_INPUT",
    "COMPLETED",
    "FAILED",
    "CANCELLED",
  ];

  it("is true when leaving RUNNING for a settled state (resume turn ends)", () => {
    for (const next of settled) {
      expect(isTurnSettleTransition("RUNNING", next)).toBe(true);
    }
  });

  it("is true when leaving STARTING for a settled state (resume boots with no turn)", () => {
    expect(isTurnSettleTransition("STARTING", "WAITING_FOR_INPUT")).toBe(true);
  });

  it("is false for the active-phase transition STARTING -> RUNNING", () => {
    expect(isTurnSettleTransition("STARTING", "RUNNING")).toBe(false);
  });

  it("is false when staying in a settled state (no churn on WAITING -> WAITING)", () => {
    expect(isTurnSettleTransition("WAITING_FOR_INPUT", "WAITING_FOR_INPUT")).toBe(false);
  });

  it("is false when entering an active state", () => {
    expect(isTurnSettleTransition("WAITING_FOR_INPUT", "RUNNING")).toBe(false);
    expect(isTurnSettleTransition("IDLE", "STARTING")).toBe(false);
  });

  it("is false when there is no previous state (initial render)", () => {
    expect(isTurnSettleTransition(null, "WAITING_FOR_INPUT")).toBe(false);
  });

  it("is false when the next state is unknown", () => {
    expect(isTurnSettleTransition("RUNNING", null)).toBe(false);
  });
});

describe("running message backfill guards", () => {
  it("detects a user prompt in the active turn", () => {
    expect(
      hasUserPromptInActiveTurn(
        [makeMessage({ id: "u1", turn_id: "turn-1", author_type: "user" })],
        "turn-1",
      ),
    ).toBe(true);
  });

  it("ignores script output and old-turn prompts", () => {
    expect(
      hasUserPromptInActiveTurn(
        [makeMessage({ id: "s1", turn_id: "turn-1", type: "script_execution" })],
        "turn-1",
      ),
    ).toBe(false);
    expect(
      hasUserPromptInActiveTurn(
        [makeMessage({ id: "u1", turn_id: "old-turn", author_type: "user" })],
        "turn-1",
      ),
    ).toBe(false);
  });

  it("runs only for a connected RUNNING session with an active-turn user prompt", () => {
    const messages = [makeMessage({ id: "u1", turn_id: "turn-1", author_type: "user" })];
    expect(
      shouldRunMessageBackfill({
        taskSessionState: "RUNNING",
        connectionStatus: "connected",
        activeTurnId: "turn-1",
        messages,
      }),
    ).toBe(true);
    expect(
      shouldRunMessageBackfill({
        taskSessionState: "WAITING_FOR_INPUT",
        connectionStatus: "connected",
        activeTurnId: "turn-1",
        messages,
      }),
    ).toBe(false);
    expect(
      shouldRunMessageBackfill({
        taskSessionState: "RUNNING",
        connectionStatus: "connecting",
        activeTurnId: "turn-1",
        messages,
      }),
    ).toBe(false);
    expect(
      shouldRunMessageBackfill({
        taskSessionState: "RUNNING",
        connectionStatus: "connected",
        activeTurnId: null,
        messages,
      }),
    ).toBe(false);
  });
});

describe("unknown session subscription retry guard", () => {
  it("retries only while a connected session id has no session state", () => {
    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: "sess-1",
        taskSessionState: null,
        connectionStatus: "connected",
      }),
    ).toBe(true);

    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: "sess-1",
        taskSessionState: "STARTING",
        connectionStatus: "connected",
      }),
    ).toBe(false);
    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: null,
        taskSessionState: null,
        connectionStatus: "connected",
      }),
    ).toBe(false);
    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: "sess-1",
        taskSessionState: null,
        connectionStatus: "connecting",
      }),
    ).toBe(false);
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

describe("stale concurrent fetch guard", () => {
  it("rejects an older fetch that completes after a newer one merged", () => {
    const sid = "guard-sess-a";
    const older = nextFetchSeq();
    const newer = nextFetchSeq();
    // The newer fetch finishes first and merges.
    expect(commitFetchSeq(sid, newer)).toBe(true);
    // The older fetch finishing late must be skipped.
    expect(commitFetchSeq(sid, older)).toBe(false);
  });

  it("applies fetches that complete in order", () => {
    const sid = "guard-sess-b";
    expect(commitFetchSeq(sid, nextFetchSeq())).toBe(true);
    expect(commitFetchSeq(sid, nextFetchSeq())).toBe(true);
  });

  it("tracks the applied sequence independently per session", () => {
    const older = nextFetchSeq();
    const newer = nextFetchSeq();
    expect(commitFetchSeq("guard-sess-c", newer)).toBe(true);
    // A different session is unaffected by another session's higher applied seq.
    expect(commitFetchSeq("guard-sess-d", older)).toBe(true);
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
    expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(MAX_AUTO_BACKFILL_PAGES);
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
