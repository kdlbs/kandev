/* eslint-disable max-lines -- session history race regressions share one lifecycle test harness. */

import { act, renderHook, waitFor } from "@testing-library/react";
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
const mockStore = { getState: () => mockState };

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockWebSocketClient,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
  useAppStoreApi: () => mockStore,
}));

import { taskId, sessionId } from "@/lib/types/ids";
import { WebSocketRequestTimeoutError } from "@/lib/ws/client";

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
  vi.useRealTimers();
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
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function configureSession(
  id: string,
  state: TaskSessionState,
  messages: Message[],
  historyInitialized = true,
) {
  const messagesBySession = mockState.messages.bySession as Record<string, Message[]>;
  const metaBySession = mockState.messages.metaBySession as Record<
    string,
    {
      historyInitialized: boolean;
      hasMore: boolean;
      oldestCursor: string | null;
      isLoading: boolean;
      isLoadingMore: boolean;
    }
  >;
  const sessions = mockState.taskSessions.items as Record<string, { state: TaskSessionState }>;
  const turnsBySession = mockState.turns.bySession as Record<string, unknown[]>;
  const activeTurns = mockState.turns.activeBySession as Record<string, string | null>;
  messagesBySession[id] = messages;
  metaBySession[id] = {
    historyInitialized,
    hasMore: false,
    oldestCursor: null,
    isLoading: false,
    isLoadingMore: false,
  };
  sessions[id] = { state };
  turnsBySession[id] = [];
  activeTurns[id] = null;
}

describe("cached session entry history readiness", () => {
  // @covers AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11
  it("keeps cached-entry history placement pending until its background refresh settles", async () => {
    const readiness = deferred<void>();
    const response = deferred<{ messages: Message[]; has_more: boolean }>();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness.promise);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness.promise,
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request.mockReturnValue(response.promise);
    mockState.messages.bySession["sess-1"] = [makeMessage({ id: "cached" })];

    const { result, rerender, unmount } = renderHook(() => useSessionMessages("sess-1"));

    expect(result.current.historyRefreshPending).toBe(true);

    await act(async () => {
      readiness.resolve();
      await readiness.promise;
    });
    expect(result.current.historyRefreshPending).toBe(true);

    // A live row can arrive while the cached refresh is still in flight.
    // That changes the message count and reruns the entry effect before the
    // refresh response settles.
    mockState.messages.bySession["sess-1"] = [
      makeMessage({ id: "cached" }),
      makeMessage({ id: "live" }),
    ];
    rerender();

    await act(async () => {
      response.resolve({ messages: [], has_more: false });
      await response.promise;
    });
    expect(result.current.historyRefreshPending).toBe(false);
    unmount();
  });
});

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

describe("history retry state", () => {
  it("retries a timed out history request and marks the snapshot ready", async () => {
    vi.useFakeTimers();
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(Promise.resolve());
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: Promise.resolve(),
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request
      .mockRejectedValueOnce(new WebSocketRequestTimeoutError("message.list"))
      .mockResolvedValueOnce({ messages: [], has_more: false });

    const { result, unmount } = renderHook(() => useSessionMessages("sess-1"));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(mockWebSocketClient.request).toHaveBeenCalledTimes(2);
    expect(result.current.historyStatus).toBe("ready");
    unmount();
  });

  it("keeps a failed history snapshot unavailable without retrying non-timeout errors", async () => {
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(Promise.resolve());
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: Promise.resolve(),
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request.mockRejectedValue(new Error("permission denied"));
    vi.spyOn(console, "error").mockImplementation(() => {});

    const { result, unmount } = renderHook(() => useSessionMessages("sess-1"));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockWebSocketClient.request).toHaveBeenCalledTimes(1);
    expect(result.current.historyStatus).toBe("unavailable");
    expect(result.current.historyError).toEqual(new Error("permission denied"));
    unmount();
  });
});

// eslint-disable-next-line max-lines-per-function -- deferred session transitions are easiest to audit as complete scenarios.
describe("history feedback generation", () => {
  it("ignores a terminal fetch rejection from session A after switching to session B", async () => {
    const aReadiness = deferred<void>();
    const bReadiness = deferred<void>();
    const aInitial = deferred<{ messages: Message[]; has_more: boolean }>();
    const aTerminal = deferred<{ messages: Message[]; has_more: boolean }>();
    const bInitial = deferred<{ messages: Message[]; has_more: boolean }>();
    const readinessBySession = new Map([
      ["sess-a", aReadiness.promise],
      ["sess-b", bReadiness.promise],
    ]);
    const aUser = makeMessage({ id: "a-user", session_id: sessionId("sess-a") });
    const bUser = makeMessage({ id: "b-user", session_id: sessionId("sess-b") });
    configureSession("sess-a", "RUNNING", [aUser]);
    configureSession("sess-b", "RUNNING", [bUser]);
    mockState.mergeMessages.mockImplementation((id, messages, metadata) => {
      (mockState.messages.bySession as Record<string, Message[]>)[id] = messages;
      (
        mockState.messages.metaBySession as Record<
          string,
          (typeof mockState.messages.metaBySession)["sess-1"]
        >
      )[id] = {
        ...mockState.messages.metaBySession["sess-1"],
        ...metadata,
      };
    });
    mockWebSocketClient.getSessionSubscriptionReadiness.mockImplementation(
      (id: string) => readinessBySession.get(id) ?? Promise.resolve(),
    );
    mockWebSocketClient.subscribeSessionWithReady.mockImplementation((id: string) => ({
      ready: readinessBySession.get(id) ?? Promise.resolve(),
      unsubscribe: vi.fn(),
    }));
    mockWebSocketClient.request
      .mockImplementationOnce(() => aInitial.promise)
      .mockImplementationOnce(() => aTerminal.promise)
      .mockImplementationOnce(() => bInitial.promise);

    const { result, rerender, unmount } = renderHook(
      ({ activeSessionId }: { activeSessionId: string }) => useSessionMessages(activeSessionId),
      { initialProps: { activeSessionId: "sess-a" } },
    );

    await act(async () => {
      aReadiness.resolve();
      await aReadiness.promise;
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(1));
    await act(async () => {
      aInitial.resolve({ messages: [aUser], has_more: false });
      await aInitial.promise;
    });

    (mockState.taskSessions.items as Record<string, { state: TaskSessionState }>)["sess-a"].state =
      "WAITING_FOR_INPUT";
    rerender({ activeSessionId: "sess-a" });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(2));

    rerender({ activeSessionId: "sess-b" });
    await act(async () => {
      bReadiness.resolve();
      await bReadiness.promise;
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(3));
    await act(async () => {
      bInitial.resolve({ messages: [bUser], has_more: false });
      await bInitial.promise;
    });
    await waitFor(() => expect(result.current.historyStatus).toBe("ready"));

    await act(async () => {
      aTerminal.reject(new Error("session A history failed"));
      await aTerminal.promise.catch(() => undefined);
    });

    expect(result.current.historyStatus).toBe("ready");
    expect(result.current.historyError).toBeNull();
    unmount();
  });

  it("ignores an obsolete manual retry after leaving and re-entering session A", async () => {
    const aReadiness = deferred<void>();
    const bReadiness = deferred<void>();
    const aReentryReadiness = deferred<void>();
    const aInitial = deferred<{ messages: Message[]; has_more: boolean }>();
    const aRetry = deferred<{ messages: Message[]; has_more: boolean }>();
    const bInitial = deferred<{ messages: Message[]; has_more: boolean }>();
    const aReentry = deferred<{ messages: Message[]; has_more: boolean }>();
    const readinessBySession = new Map<string, Promise<void>>([["sess-a", aReadiness.promise]]);
    const aUser = makeMessage({ id: "a-user", session_id: sessionId("sess-a") });
    const bUser = makeMessage({ id: "b-user", session_id: sessionId("sess-b") });
    const aNewUser = makeMessage({ id: "a-new-user", session_id: sessionId("sess-a") });
    configureSession("sess-a", "RUNNING", [aUser]);
    configureSession("sess-b", "RUNNING", [bUser]);
    mockState.mergeMessages.mockImplementation((id, messages, metadata) => {
      (mockState.messages.bySession as Record<string, Message[]>)[id] = messages;
      (
        mockState.messages.metaBySession as Record<
          string,
          (typeof mockState.messages.metaBySession)["sess-1"]
        >
      )[id] = {
        ...mockState.messages.metaBySession["sess-1"],
        ...metadata,
      };
    });
    mockWebSocketClient.getSessionSubscriptionReadiness.mockImplementation(
      (id: string) => readinessBySession.get(id) ?? Promise.resolve(),
    );
    mockWebSocketClient.subscribeSessionWithReady.mockImplementation((id: string) => ({
      ready: readinessBySession.get(id) ?? Promise.resolve(),
      unsubscribe: vi.fn(),
    }));
    mockWebSocketClient.request
      .mockImplementationOnce(() => aInitial.promise)
      .mockImplementationOnce(() => aRetry.promise)
      .mockImplementationOnce(() => bInitial.promise)
      .mockImplementationOnce(() => aReentry.promise);

    const { result, rerender, unmount } = renderHook(
      ({ activeSessionId }: { activeSessionId: string }) => useSessionMessages(activeSessionId),
      { initialProps: { activeSessionId: "sess-a" } },
    );
    await act(async () => {
      aReadiness.resolve();
      await aReadiness.promise;
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(1));
    await act(async () => {
      aInitial.resolve({ messages: [aUser], has_more: false });
      await aInitial.promise;
    });

    await act(async () => {
      result.current.retryHistory();
      await Promise.resolve();
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(2));

    readinessBySession.set("sess-b", bReadiness.promise);
    rerender({ activeSessionId: "sess-b" });
    await act(async () => {
      bReadiness.resolve();
      await bReadiness.promise;
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(3));
    await act(async () => {
      bInitial.resolve({ messages: [bUser], has_more: false });
      await bInitial.promise;
    });

    readinessBySession.set("sess-a", aReentryReadiness.promise);
    rerender({ activeSessionId: "sess-a" });
    await act(async () => {
      aReentryReadiness.resolve();
      await aReentryReadiness.promise;
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(4));
    await act(async () => {
      aReentry.resolve({ messages: [aNewUser], has_more: false });
      await aReentry.promise;
    });
    await waitFor(() => expect(result.current.historyStatus).toBe("ready"));

    await act(async () => {
      aRetry.reject(new Error("obsolete retry failed"));
      await aRetry.promise.catch(() => undefined);
    });

    expect(result.current.historyStatus).toBe("ready");
    expect(result.current.historyError).toBeNull();
    expect((mockState.messages.bySession as Record<string, Message[]>)["sess-a"]).toEqual([
      aNewUser,
    ]);
    unmount();
  });

  it("forces a fresh message snapshot when Retry follows a failed refresh", async () => {
    const readiness = Promise.resolve();
    const initial = deferred<{ messages: Message[]; has_more: boolean }>();
    const failedRefresh = deferred<{ messages: Message[]; has_more: boolean }>();
    const retried = deferred<{ messages: Message[]; has_more: boolean }>();
    const oldUser = makeMessage({ id: "old-user", session_id: sessionId("sess-1") });
    const newUser = makeMessage({ id: "new-user", session_id: sessionId("sess-1") });
    configureSession("sess-1", "RUNNING", [oldUser]);
    mockState.mergeMessages.mockImplementation((id, messages, metadata) => {
      (mockState.messages.bySession as Record<string, Message[]>)[id] = messages;
      (
        mockState.messages.metaBySession as Record<
          string,
          (typeof mockState.messages.metaBySession)["sess-1"]
        >
      )[id] = {
        ...mockState.messages.metaBySession["sess-1"],
        ...metadata,
      };
    });
    mockWebSocketClient.getSessionSubscriptionReadiness.mockReturnValue(readiness);
    mockWebSocketClient.subscribeSessionWithReady.mockReturnValue({
      ready: readiness,
      unsubscribe: vi.fn(),
    });
    mockWebSocketClient.request
      .mockImplementationOnce(() => initial.promise)
      .mockImplementationOnce(() => failedRefresh.promise)
      .mockImplementationOnce(() => retried.promise);

    const { result, rerender, unmount } = renderHook(() => useSessionMessages("sess-1"));
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(1));
    await act(async () => {
      initial.resolve({ messages: [oldUser], has_more: false });
      await initial.promise;
    });
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 0));
    });

    (mockState.taskSessions.items as Record<string, { state: TaskSessionState }>)["sess-1"].state =
      "WAITING_FOR_INPUT";
    rerender();
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(2));
    await act(async () => {
      failedRefresh.reject(new Error("refresh failed"));
      await failedRefresh.promise.catch(() => undefined);
    });
    await waitFor(() => expect(result.current.historyStatus).toBe("unavailable"));

    await act(async () => {
      result.current.retryHistory();
      await Promise.resolve();
    });
    await waitFor(() => expect(mockWebSocketClient.request).toHaveBeenCalledTimes(3));
    await act(async () => {
      retried.resolve({ messages: [newUser], has_more: false });
      await retried.promise;
    });

    await waitFor(() => expect(result.current.historyStatus).toBe("ready"));
    expect(mockState.messages.bySession["sess-1"]).toEqual([newUser]);
    unmount();
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
