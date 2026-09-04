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
    expect(state.setPromptMessagesLoadingMore).toHaveBeenCalledTimes(1);
  });
});
