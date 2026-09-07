import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

const { listTaskSessionMessages, state, storeApi } = vi.hoisted(() => {
  const state = {
    messagePrompts: {
      bySession: {
        session: [{ id: "existing" } as Message],
      } as Record<string, Message[]>,
      metaBySession: {
        session: {
          isLoading: false,
          isLoadingMore: false,
          hasMore: true,
          oldestCursor: "cursor",
        },
      } as Record<
        string,
        {
          isLoading: boolean;
          isLoadingMore: boolean;
          hasMore: boolean;
          oldestCursor: string | null;
        }
      >,
      generationBySession: { session: 0 } as Record<string, number>,
      refreshGenerationBySession: { session: 0 } as Record<string, number>,
    },
    setPromptMessagesLoadingMore: vi.fn(),
    prependPromptMessages: vi.fn(),
  };
  return { listTaskSessionMessages: vi.fn(), state, storeApi: { getState: () => state } };
});

vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => storeApi,
}));

import { useLazyLoadPrompts } from "./use-lazy-load-prompts";

// eslint-disable-next-line max-lines-per-function -- pagination race cases share one deterministic harness.
describe("useLazyLoadPrompts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.messagePrompts.bySession.session = [{ id: "existing" } as Message];
    state.messagePrompts.metaBySession.session = {
      isLoading: false,
      isLoadingMore: false,
      hasMore: true,
      oldestCursor: "cursor",
    };
    state.messagePrompts.refreshGenerationBySession.session = 0;
    state.messagePrompts.generationBySession.session = 0;
  });

  it("does not resurrect prompt state after session removal during an older-page request", async () => {
    const pending = Promise.withResolvers<{
      messages: Message[];
      has_more: boolean;
      cursor: string;
    }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useLazyLoadPrompts("session"));

    const load = result.current.loadMore();
    delete state.messagePrompts.bySession.session;
    delete state.messagePrompts.metaBySession.session;
    state.messagePrompts.generationBySession.session += 1;
    // A recreated session can immediately install fresh empty state. The
    // generation guard must still reject the old response.
    state.messagePrompts.bySession.session = [];
    state.messagePrompts.metaBySession.session = {
      isLoading: false,
      isLoadingMore: true,
      hasMore: true,
      oldestCursor: "new-cursor",
    };
    pending.resolve({ messages: [{ id: "stale" } as Message], has_more: false, cursor: "stale" });
    await act(async () => load);

    expect(state.prependPromptMessages).not.toHaveBeenCalled();
    expect(state.setPromptMessagesLoadingMore).toHaveBeenCalledWith("session", true);
  });

  it("reports an older-page request as stale after session generation changes", async () => {
    const pending = Promise.withResolvers<{
      messages: Message[];
      has_more: boolean;
      cursor: string;
    }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useLazyLoadPrompts("session"));

    const load = result.current.loadMore();
    expect(result.current.isRequestCurrent()).toBe(true);
    state.messagePrompts.generationBySession.session += 1;
    expect(result.current.isRequestCurrent()).toBe(false);

    pending.resolve({ messages: [], has_more: false, cursor: "next" });
    await act(async () => load);
  });

  it("keeps the request marker available until the sentinel consumes completion", async () => {
    const pending = Promise.withResolvers<{
      messages: Message[];
      has_more: boolean;
      cursor: string;
    }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useLazyLoadPrompts("session"));

    const load = result.current.loadMore();
    pending.resolve({ messages: [], has_more: false, cursor: "next" });
    await act(async () => load);

    expect(result.current.isRequestCurrent()).toBe(true);
  });

  it("rejects an older page after a newer initial refresh completes", async () => {
    const pending = Promise.withResolvers<{
      messages: Message[];
      has_more: boolean;
      cursor: string;
    }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useLazyLoadPrompts("session"));

    const load = result.current.loadMore();
    state.messagePrompts.refreshGenerationBySession.session += 1;
    expect(result.current.isRequestCurrent()).toBe(false);
    state.messagePrompts.metaBySession.session.isLoading = false;
    pending.resolve({ messages: [{ id: "stale" } as Message], has_more: false, cursor: "stale" });
    await act(async () => load);

    expect(state.prependPromptMessages).not.toHaveBeenCalled();
  });

  it("keeps the replacement session request current when the old request settles first", async () => {
    const pendingA = Promise.withResolvers<{
      messages: Message[];
      has_more: boolean;
      cursor: string;
    }>();
    const pendingB = Promise.withResolvers<{
      messages: Message[];
      has_more: boolean;
      cursor: string;
    }>();
    listTaskSessionMessages
      .mockReturnValueOnce(pendingA.promise)
      .mockReturnValueOnce(pendingB.promise);
    state.messagePrompts.bySession.other = [{ id: "other-existing" } as Message];
    state.messagePrompts.metaBySession.other = {
      isLoading: false,
      isLoadingMore: false,
      hasMore: true,
      oldestCursor: "other-cursor",
    };
    state.messagePrompts.generationBySession.other = 0;
    state.messagePrompts.refreshGenerationBySession.other = 0;
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useLazyLoadPrompts(sessionId),
      { initialProps: { sessionId: "session" } },
    );

    const loadA = result.current.loadMore();
    rerender({ sessionId: "other" });
    const loadB = result.current.loadMore();
    expect(listTaskSessionMessages).toHaveBeenNthCalledWith(2, "other", expect.anything());

    pendingA.resolve({ messages: [], has_more: true, cursor: "a-next" });
    await act(async () => loadA);
    expect(result.current.isRequestCurrent()).toBe(true);

    pendingB.resolve({ messages: [], has_more: false, cursor: "b-next" });
    await act(async () => loadB);
    expect(result.current.isRequestCurrent()).toBe(true);
  });

  it("clears the loading state when an older-page request rejects", async () => {
    const error = new Error("request failed");
    listTaskSessionMessages.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useLazyLoadPrompts("session"));

    await expect(result.current.loadMore()).rejects.toThrow(error);

    expect(state.setPromptMessagesLoadingMore).toHaveBeenCalledWith("session", false);
  });
});
