import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";

const mockRequest = vi.fn();
const mockMergeMessages = vi.fn();
const mockSetMessages = vi.fn();
const mockListSessionTurns = vi.fn();
const SESSION_ID = "sess-1";
const MESSAGE_LIST = "message.list";

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    getSessionSubscriptionReadiness: () => Promise.resolve(),
    request: mockRequest,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
  useAppStoreApi: () => ({
    getState: () => ({
      messages: { bySession: {} },
      taskSessions: { items: { [SESSION_ID]: { state: "RUNNING" } } },
      turns: {
        bySession: {},
        activeBySession: {},
        loadedBySession: {},
        reconcileEpochBySession: {},
        settledBoundaryBySession: {},
      },
      mergeMessages: mockMergeMessages,
      setMessages: mockSetMessages,
      addTurn: vi.fn(),
      markTurnsLoaded: vi.fn(),
      reconcileActiveTurnAfterHydration: vi.fn(),
    }),
  }),
}));

import { useVisibilityBackfill } from "./use-session-messages";

function setVisibility(value: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

describe("useVisibilityBackfill", () => {
  let store: { getState: () => unknown };

  beforeEach(() => {
    vi.clearAllMocks();
    mockRequest.mockResolvedValue({ messages: [], has_more: false });
    mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });
    store = {
      getState: () => ({
        messages: { bySession: {} },
        taskSessions: { items: { [SESSION_ID]: { state: "RUNNING" } } },
        turns: {
          bySession: {},
          activeBySession: {},
          loadedBySession: {},
          reconcileEpochBySession: {},
          settledBoundaryBySession: {},
        },
        mergeMessages: mockMergeMessages,
        setMessages: mockSetMessages,
        addTurn: vi.fn(),
        markTurnsLoaded: vi.fn(),
        reconcileActiveTurnAfterHydration: vi.fn(),
      }),
    };
  });

  afterEach(() => {
    cleanup();
  });

  it("fetches when the tab becomes visible", async () => {
    renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));
    await act(async () => {
      setVisibility("visible");
      await Promise.resolve();
    });
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockRequest).toHaveBeenCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: SESSION_ID }),
      expect.any(Number),
    );
  });

  it("fetches when the Kandev window regains focus", async () => {
    renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      await Promise.resolve();
    });

    expect(mockRequest).toHaveBeenCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: SESSION_ID }),
      expect.any(Number),
    );
  });

  it("does not fetch when the tab becomes hidden", () => {
    renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));
    setVisibility("hidden");
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("does nothing when sessionId is null", () => {
    renderHook(() => useVisibilityBackfill(null, store as never));
    setVisibility("visible");
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("removes the listener on unmount", () => {
    const { unmount } = renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));
    unmount();
    setVisibility("visible");
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("re-registers when sessionId changes", async () => {
    const { rerender } = renderHook(
      ({ id }: { id: string | null }) => useVisibilityBackfill(id, store as never),
      { initialProps: { id: SESSION_ID } },
    );
    await act(async () => {
      setVisibility("visible");
      await Promise.resolve();
    });
    expect(mockRequest).toHaveBeenLastCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: SESSION_ID }),
      expect.any(Number),
    );

    rerender({ id: "sess-2" });
    await act(async () => {
      setVisibility("visible");
      await Promise.resolve();
    });
    expect(mockRequest).toHaveBeenLastCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: "sess-2" }),
      expect.any(Number),
    );
  });
});
