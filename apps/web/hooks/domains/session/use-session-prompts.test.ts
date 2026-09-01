import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

const { listTaskSessionMessages, state, storeApi } = vi.hoisted(() => {
  const state = {
    messagePrompts: {
      bySession: { session: [] as Message[] },
      metaBySession: {
        session: { isLoading: false, isLoadingMore: false, hasMore: false, oldestCursor: null },
      },
      generationBySession: { session: 0 },
    },
    connection: { status: "connected" },
    setPromptMessagesLoading: vi.fn(),
    replacePromptMessages: vi.fn(),
  };
  return {
    listTaskSessionMessages: vi.fn(),
    state,
    storeApi: { getState: () => state },
  };
});

vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => storeApi,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    subscribeSessionWithReady: () => ({ ready: Promise.resolve(), unsubscribe: vi.fn() }),
  }),
}));
import { useSessionPrompts } from "./use-session-prompts";

describe("useSessionPrompts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.connection.status = "connected";
    listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false, cursor: null });
  });

  it("requests only user-authored prompt messages", async () => {
    renderHook(() => useSessionPrompts("session"));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    expect(listTaskSessionMessages).toHaveBeenCalledWith("session", {
      author_type: "user",
      limit: 20,
      sort: "desc",
    });
  });

  it("exposes a terminal fetch failure without keeping loading true", async () => {
    listTaskSessionMessages.mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useSessionPrompts("session"));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.fetchFailed).toBe(true));

    expect(result.current.isLoading).toBe(false);
  });

  it("shares the initial request across concurrent consumers", async () => {
    const pending = Promise.withResolvers<{ messages: Message[] }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);

    renderHook(() => useSessionPrompts("session"));
    renderHook(() => useSessionPrompts("session"));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    pending.resolve({ messages: [] });
    await waitFor(() =>
      expect(state.setPromptMessagesLoading).toHaveBeenCalledWith("session", false),
    );
  });

  it("hydrates prompts even while the websocket is disconnected", async () => {
    state.connection.status = "disconnected";
    renderHook(() => useSessionPrompts("session"));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    expect(state.replacePromptMessages).toHaveBeenCalledWith("session", [], {
      hasMore: false,
      oldestCursor: null,
    });
  });

  it("retries after a failed prompt fetch", async () => {
    listTaskSessionMessages.mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useSessionPrompts("session"));
    await waitFor(() => expect(result.current.fetchFailed).toBe(true));

    listTaskSessionMessages.mockResolvedValueOnce({ messages: [], has_more: false, cursor: null });
    await act(async () => result.current.retryPrompts());

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(2));
    expect(result.current.fetchFailed).toBe(false);
  });
});
