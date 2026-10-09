import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

const { listTaskSessionMessages, state, storeApi } = vi.hoisted(() => {
  const state = {
    messagePrompts: {
      bySession: { session: [] as Message[] },
      metaBySession: {
        session: {
          isLoading: false,
          isLoadingMore: false,
          hasMore: false,
          oldestCursor: null as string | null,
        },
      },
      generationBySession: { session: 0 },
      refreshGenerationBySession: { session: 0 },
      authoritativeBySession: {} as Record<string, true>,
      observedBySession: {},
      deletedIdsBySession: {},
    },
    connection: { status: "connected" },
    setPromptMessagesLoading: vi.fn(),
    replacePromptMessages: vi.fn(),
    installAuthoritativePromptMessages: vi.fn(),
  };
  return {
    listTaskSessionMessages: vi.fn(),
    state,
    storeApi: { getState: () => state },
  };
});

const readinessLease = { subscribers: 0, ready: Promise.resolve() as Promise<void> };

vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => storeApi,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    subscribeSessionWithReady: () => {
      if (readinessLease.subscribers++ === 0) readinessLease.ready = Promise.resolve();
      return {
        ready: readinessLease.ready,
        unsubscribe: () => {
          readinessLease.subscribers--;
        },
      };
    },
  }),
}));
import { useSessionPrompts } from "./use-session-prompts";

beforeEach(() => {
  vi.clearAllMocks();
  listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false, cursor: null });
  state.connection.status = "connected";
  state.messagePrompts.refreshGenerationBySession.session = 0;
  state.messagePrompts.authoritativeBySession = {};
  state.installAuthoritativePromptMessages.mockReset();
  state.messagePrompts.bySession.session = [];
  state.messagePrompts.metaBySession.session.oldestCursor = null;
  state.messagePrompts.generationBySession.session = 0;
});

describe("useSessionPrompts authority", () => {
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
  it("reads through a non-authoritative cache without replacing paginated rows", async () => {
    const pending = Promise.withResolvers<{ messages: Message[] }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);
    state.messagePrompts.bySession.session = [
      { id: "older", session_id: "session", author_type: "user" } as Message,
    ];
    state.messagePrompts.metaBySession.session.oldestCursor = "older";
    renderHook(() => useSessionPrompts("session", { firstLoadOnly: true }));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    await act(async () => pending.resolve({ messages: [] }));
    expect(state.installAuthoritativePromptMessages).toHaveBeenCalledWith("session", [], {
      hasMore: false,
      oldestCursor: null,
    });
    expect(state.replacePromptMessages).not.toHaveBeenCalled();
  });

  it("skips every read and prompt-slice write when the projection is authoritative", async () => {
    state.messagePrompts.authoritativeBySession.session = true;
    renderHook(() => useSessionPrompts("session", { firstLoadOnly: true }));

    await act(async () => Promise.resolve());
    expect(listTaskSessionMessages).not.toHaveBeenCalled();
    expect(state.setPromptMessagesLoading).not.toHaveBeenCalled();
    expect(state.installAuthoritativePromptMessages).not.toHaveBeenCalled();
  });
});

describe("useSessionPrompts requests", () => {
  it("does not retry a failed authority read until the connection status changes", async () => {
    listTaskSessionMessages.mockRejectedValueOnce(new Error("offline"));
    const { result, rerender } = renderHook(() =>
      useSessionPrompts("session", { firstLoadOnly: true }),
    );
    await waitFor(() => expect(result.current.fetchFailed).toBe(true));
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);

    state.connection.status = "disconnected";
    rerender();
    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(2));
    expect(state.setPromptMessagesLoading).not.toHaveBeenCalled();
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
    await waitFor(() =>
      expect(state.replacePromptMessages).toHaveBeenCalledWith("session", [], {
        hasMore: false,
        oldestCursor: null,
      }),
    );
  });

  it("retries after a failed prompt fetch", async () => {
    listTaskSessionMessages.mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useSessionPrompts("session"));
    await waitFor(() => expect(result.current.fetchFailed).toBe(true));

    listTaskSessionMessages.mockResolvedValueOnce({ messages: [], has_more: false, cursor: null });
    await act(async () => result.current.retryPrompts());

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.fetchFailed).toBe(false));
  });

  it("does not refetch when a live prompt mutation advances the refresh revision", async () => {
    const { rerender } = renderHook(() => useSessionPrompts("session"));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    state.messagePrompts.refreshGenerationBySession.session = 1;
    rerender();

    await Promise.resolve();
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
  });
});
