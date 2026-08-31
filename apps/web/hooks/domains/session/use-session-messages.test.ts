import { act, renderHook } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { Message } from "@/lib/types/http";

const mockListSessionTurns = vi.fn();
const mockWebSocketClient = {
  getSessionSubscriptionReadiness: vi.fn(),
  request: vi.fn(),
  subscribeSession: vi.fn(),
  subscribeSessionWithReady: vi.fn(),
};

const mockState = {
  messages: {
    bySession: { "sess-1": [] as Message[] },
    metaBySession: {
      "sess-1": {
        historyInitialized: false,
        hasMore: false,
        oldestCursor: null,
        isLoading: false,
        isLoadingMore: false,
      },
    },
  },
  taskSessions: { items: { "sess-1": { state: "RUNNING" } } },
  turns: {
    bySession: { "sess-1": [] as unknown[] },
    activeBySession: { "sess-1": null },
    loadedBySession: {} as Record<string, boolean>,
    reconcileEpochBySession: {} as Record<string, number>,
    settledBoundaryBySession: {} as Record<string, string>,
  },
  connection: { status: "connected" },
  mergeMessages: vi.fn(),
  setMessagesLoading: vi.fn(),
  setMessages: vi.fn(),
  prependMessages: vi.fn(),
  addTurn: vi.fn(),
  mergeTurnsSnapshot: vi.fn(),
  markTurnsLoaded: vi.fn((sessionId: string) => {
    mockState.turns.loadedBySession[sessionId] = true;
  }),
  setActiveTurn: vi.fn(),
  reconcileActiveTurnAfterHydration: vi.fn(),
};
const mockStoreApi = { getState: () => mockState };

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockWebSocketClient,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
  useAppStoreApi: () => mockStoreApi,
}));

import { taskId, sessionId } from "@/lib/types/ids";

beforeEach(() => {
  vi.clearAllMocks();
  mockState.mergeMessages.mockReset();
  mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });
  mockWebSocketClient.request.mockResolvedValue({ messages: [], has_more: false });
  mockWebSocketClient.subscribeSession.mockReturnValue(vi.fn());
  mockState.messages.bySession["sess-1"] = [];
  mockState.messages.metaBySession["sess-1"] = {
    historyInitialized: false,
    hasMore: false,
    oldestCursor: null,
    isLoading: false,
    isLoadingMore: false,
  };
  mockState.connection.status = "connected";
  mockState.taskSessions.items["sess-1"] = { state: "RUNNING" };
  mockState.turns.bySession["sess-1"] = [];
  mockState.turns.activeBySession["sess-1"] = null;
  mockState.turns.loadedBySession = {};
  mockState.mergeTurnsSnapshot.mockImplementation(
    (_sessionId: string, turns: unknown[], hydrationEpoch: number) => {
      turns.forEach((turn) => mockState.addTurn(turn));
      mockState.reconcileActiveTurnAfterHydration("sess-1", hydrationEpoch);
      mockState.markTurnsLoaded("sess-1");
    },
  );
});

afterEach(() => {
  vi.restoreAllMocks();
});
import {
  hasUserPromptInActiveTurn,
  isTurnSettleTransition,
  shouldRunMessageBackfill,
  shouldRetryUnknownSessionSubscription,
  nextFetchSeq,
  commitFetchSeq,
  useSessionMessages,
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

  it("runs for a connected RUNNING session with an active turn", () => {
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
        taskSessionState: "RUNNING",
        connectionStatus: "connected",
        activeTurnId: "turn-1",
        messages: [],
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

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("session subscription hydration ordering", () => {
  // @covers AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
  it("replaces a disjoint stale cache while preserving rows received during the fetch", async () => {
    const readiness = deferred<void>();
    const response = deferred<{ messages: Message[]; has_more: boolean }>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request.mockReturnValue(response.promise);

    const staleOldest = makeMessage({
      id: "stale-1",
      created_at: "2026-08-30T09:00:00Z",
    });
    const staleNewest = makeMessage({
      id: "stale-2",
      created_at: "2026-08-30T09:01:00Z",
    });
    mockState.messages.bySession["sess-1"] = [staleOldest, staleNewest];

    const { unmount } = renderHook(() => useSessionMessages("sess-1"));

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });

    const liveDuringFetch = makeMessage({
      id: "live-5",
      author_type: "agent",
      created_at: "2026-08-30T09:05:00Z",
    });
    mockState.messages.bySession["sess-1"] = [staleOldest, staleNewest, liveDuringFetch];

    const fetchedNewest = makeMessage({
      id: "fetched-4",
      author_type: "agent",
      created_at: "2026-08-30T09:04:00Z",
    });
    const fetchedOldest = makeMessage({
      id: "fetched-3",
      author_type: "agent",
      created_at: "2026-08-30T09:03:00Z",
    });
    await act(async () => {
      response.resolve({ messages: [fetchedNewest, fetchedOldest], has_more: true });
      await response.promise;
    });

    expect(mockState.mergeMessages).toHaveBeenCalledWith(
      "sess-1",
      [fetchedOldest, fetchedNewest, liveDuringFetch],
      { historyInitialized: true, hasMore: true, oldestCursor: "fetched-3" },
    );
    unmount();
  });

  it("does not request messages before subscription acknowledgement", async () => {
    const readiness = deferred<void>();
    const response = deferred<{ messages: Message[]; has_more: boolean }>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request.mockReturnValue(response.promise);

    const { unmount } = renderHook(() => useSessionMessages("sess-1"));

    expect(mockWebSocketClient.request).not.toHaveBeenCalled();
    expect(mockListSessionTurns).not.toHaveBeenCalled();

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });

    expect(mockWebSocketClient.subscribeSessionWithReady).toHaveBeenCalledTimes(1);
    expect(mockWebSocketClient.request).toHaveBeenCalledWith(
      "message.list",
      expect.objectContaining({ session_id: "sess-1" }),
      10000,
    );
    expect(mockWebSocketClient.request).toHaveBeenCalledTimes(1);

    await act(async () => {
      response.resolve({ messages: [], has_more: false });
      await response.promise;
    });
    unmount();
  });

  it("does not start hydration when acknowledgement resolves after cleanup", async () => {
    const readiness = deferred<void>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });

    const { unmount } = renderHook(() => useSessionMessages("sess-1"));
    unmount();

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });

    expect(mockWebSocketClient.request).not.toHaveBeenCalled();
    expect(mockState.setMessagesLoading).toHaveBeenLastCalledWith("sess-1", false);
  });
});

describe("turn loading for sessions without hydrated turns", () => {
  it("fetches and merges turns when the session has none in the store", async () => {
    const readiness = deferred<void>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockListSessionTurns.mockResolvedValue({
      turns: [
        {
          id: "turn-1",
          session_id: "sess-1",
          task_id: "task-1",
          started_at: "2026-08-10T10:00:00Z",
          completed_at: "2026-08-10T10:05:00Z",
          metadata: { runtime_config_snapshot: { model: "deepseek/deepseek-v4-flash" } },
          created_at: "2026-08-10T10:00:00Z",
          updated_at: "2026-08-10T10:05:00Z",
        },
      ],
      total: 1,
    });
    mockState.turns.bySession["sess-1"] = [];

    const { unmount } = renderHook(() => useSessionMessages("sess-1"));

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });
    // Flush the chained message/turn fetch microtasks.
    await act(async () => {});

    expect(mockListSessionTurns).toHaveBeenCalledWith("sess-1", expect.anything());
    expect(mockState.addTurn).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "turn-1",
        metadata: { runtime_config_snapshot: { model: "deepseek/deepseek-v4-flash" } },
      }),
    );
    expect(mockState.markTurnsLoaded).toHaveBeenCalledWith("sess-1");
    unmount();
  });

  it("still fetches the full history when WS-seeded turns exist but no marker", async () => {
    // WS `session.turn.*` events seed individual live turns without the full
    // history; array presence must NOT suppress the REST hydration, or older
    // messages keep resolving to `turn = null` (the reported regression).
    const readiness = deferred<void>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockState.turns.bySession["sess-1"] = [{ id: "turn-live" }];
    mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });

    const { unmount } = renderHook(() => useSessionMessages("sess-1"));

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });
    await act(async () => {});

    // The REST hydration must run despite the partial live turn (the loaded
    // marker is the gate, not array presence). At least one full fetch is
    // required; the exact count is environment-dependent (multiple message
    // fetch paths race at mount), and single-flight semantics are pinned in
    // use-session-turns-hydration.test.ts.
    expect(mockListSessionTurns).toHaveBeenCalled();
    unmount();
  });

  it("refreshes the turn snapshot for the current subscription generation", async () => {
    const readiness = deferred<void>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockState.turns.loadedBySession["sess-1"] = true;

    const { unmount } = renderHook(() => useSessionMessages("sess-1"));

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });
    await act(async () => {});

    expect(mockListSessionTurns).toHaveBeenCalledWith("sess-1", expect.anything());
    unmount();
  });
});

describe("deduplicated message request baselines", () => {
  it("uses the first caller's cache baseline for a concurrent fetch", async () => {
    const readiness = deferred<void>();
    const response = deferred<{ messages: Message[]; has_more: boolean }>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request.mockReturnValue(response.promise);

    const staleOldest = makeMessage({
      id: "stale-1",
      created_at: "2026-08-30T09:00:00Z",
    });
    const staleNewest = makeMessage({
      id: "stale-2",
      created_at: "2026-08-30T09:01:00Z",
    });
    const liveDuringFetch = makeMessage({
      id: "live-5",
      author_type: "agent",
      created_at: "2026-08-30T09:05:00Z",
    });
    mockState.messages.bySession["sess-1"] = [staleOldest, staleNewest];
    mockState.mergeMessages.mockImplementation((_sessionId, messages) => {
      mockState.messages.bySession["sess-1"] = messages;
    });

    const first = renderHook(() => useSessionMessages("sess-1"));
    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });
    expect(mockWebSocketClient.request).toHaveBeenCalledTimes(1);

    mockState.messages.bySession["sess-1"] = [staleOldest, staleNewest, liveDuringFetch];
    const second = renderHook(() => useSessionMessages("sess-1"));

    const fetchedOldest = makeMessage({
      id: "fetched-3",
      author_type: "agent",
      created_at: "2026-08-30T09:03:00Z",
    });
    const fetchedNewest = makeMessage({
      id: "fetched-4",
      author_type: "agent",
      created_at: "2026-08-30T09:04:00Z",
    });
    await act(async () => {
      response.resolve({ messages: [fetchedNewest, fetchedOldest], has_more: true });
      await response.promise;
      await Promise.resolve();
    });

    expect(mockState.mergeMessages).toHaveBeenLastCalledWith(
      "sess-1",
      [fetchedOldest, fetchedNewest, liveDuringFetch],
      { historyInitialized: true, hasMore: true, oldestCursor: "fetched-3" },
    );
    first.unmount();
    second.unmount();
  });
});
