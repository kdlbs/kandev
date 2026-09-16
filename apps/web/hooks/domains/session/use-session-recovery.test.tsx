import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message, Turn } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";

const listSessionTurns = vi.fn();
const recoveryHandlers = new Map<string, () => Promise<boolean>>();
const webSocketClient = {
  getSessionSubscriptionReadiness: vi.fn(() => Promise.resolve()),
  request: vi.fn(),
  subscribeSessionWithReady: vi.fn(() => ({
    ready: Promise.resolve(),
    unsubscribe: vi.fn(),
  })),
  registerCoreSessionRecovery: vi.fn((id: string, handler: () => Promise<boolean>) => {
    recoveryHandlers.set(id, handler);
    return () => recoveryHandlers.delete(id);
  }),
  retryCoreSessionRecovery: vi.fn(() => undefined),
};

const staleMessage = {
  id: "message-stale",
  task_id: taskId("task-1"),
  session_id: sessionId("sess-1"),
  author_type: "agent",
  content: "old content",
  type: "message",
  created_at: "2026-09-14T12:00:00Z",
} as Message;
const repairedMessage = { ...staleMessage, content: "repaired content" };
const repairedTurn = {
  id: "turn-1",
  task_id: taskId("task-1"),
  session_id: sessionId("sess-1"),
  started_at: "2026-09-14T11:59:00Z",
  completed_at: "2026-09-14T12:01:00Z",
  created_at: "2026-09-14T11:59:00Z",
  updated_at: "2026-09-14T12:01:00Z",
  metadata: { model: "repaired" },
} as Turn;

const state = {
  messages: {
    bySession: { "sess-1": [staleMessage] as Message[] },
    metaBySession: {
      "sess-1": {
        historyInitialized: true,
        hasMore: false,
        oldestCursor: staleMessage.id,
        isLoading: false,
        isLoadingMore: false,
      },
    },
  },
  taskSessions: { items: { "sess-1": { id: "sess-1", state: "IDLE" } } },
  turns: {
    bySession: { "sess-1": [] as Turn[] },
    activeBySession: { "sess-1": null as string | null },
    loadedBySession: { "sess-1": true },
    reconcileEpochBySession: { "sess-1": 0 },
    settledBoundaryBySession: {},
  },
  connection: { status: "connected" },
  mergeMessages: vi.fn(),
  setMessagesLoading: vi.fn(),
  mergeTurnsSnapshot: vi.fn(),
  addTurn: vi.fn(),
  markTurnsLoaded: vi.fn(),
  reconcileActiveTurnAfterHydration: vi.fn(),
  setActiveTurn: vi.fn(),
};
const store = { getState: () => state };

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => listSessionTurns(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => webSocketClient,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => store,
}));

import { useSessionMessages } from "./use-session-messages";

beforeEach(() => {
  vi.clearAllMocks();
  recoveryHandlers.clear();
  state.messages.bySession["sess-1"] = [staleMessage];
  state.messages.metaBySession["sess-1"] = {
    historyInitialized: true,
    hasMore: false,
    oldestCursor: staleMessage.id,
    isLoading: false,
    isLoadingMore: false,
  };
  state.turns.bySession["sess-1"] = [];
  state.turns.loadedBySession["sess-1"] = true;
  state.turns.activeBySession["sess-1"] = null;
  state.connection.status = "connected";
  listSessionTurns.mockImplementation(() =>
    Promise.resolve({ turns: repairedTurn.id === "turn-1" ? [repairedTurn] : [] }),
  );
  let messageRequest = 0;
  webSocketClient.request.mockImplementation(() => {
    messageRequest += 1;
    return Promise.resolve({
      messages: messageRequest === 1 ? [staleMessage] : [repairedMessage],
      has_more: false,
    });
  });
  state.mergeMessages.mockImplementation(
    (sessionId: string, messages: Message[], meta?: Record<string, unknown>) => {
      const messagesBySession = state.messages.bySession as Record<string, Message[]>;
      const metaBySession = state.messages.metaBySession as Record<string, Record<string, unknown>>;
      messagesBySession[sessionId] = messages;
      Object.assign(metaBySession[sessionId], meta);
    },
  );
  state.mergeTurnsSnapshot.mockImplementation(
    (sessionId: string, turns: Turn[], _epoch: number, options?: { replace?: boolean }) => {
      const turnsBySession = state.turns.bySession as Record<string, Turn[]>;
      turnsBySession[sessionId] = options?.replace
        ? turns
        : [...turnsBySession[sessionId], ...turns];
      (state.turns.loadedBySession as Record<string, boolean>)[sessionId] = true;
    },
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("core session recovery", () => {
  it("rehydrates both core snapshots before resuming after invalid resume", async () => {
    const { unmount } = renderHook(() => useSessionMessages("sess-1"));

    await waitFor(() => expect(recoveryHandlers.has("sess-1")).toBe(true));
    const recover = recoveryHandlers.get("sess-1");
    if (!recover) throw new Error("Recovery owner was not registered");

    await act(async () => {
      await expect(recover()).resolves.toBe(true);
    });

    expect(state.messages.bySession["sess-1"]).toEqual([repairedMessage]);
    expect(state.turns.bySession["sess-1"]).toEqual([repairedTurn]);
    expect(state.mergeMessages).toHaveBeenCalledWith(
      "sess-1",
      [repairedMessage],
      expect.objectContaining({ historyInitialized: true }),
    );
    expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith("sess-1", [repairedTurn], 0, {
      replace: true,
    });

    unmount();
  });
});
