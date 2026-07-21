import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  listTaskSessionMessages: vi.fn(),
  prependMessages: vi.fn(),
  setMessagesMetadata: vi.fn(),
  state: {
    messages: {
      metaBySession: {
        "session-1": { hasMore: true, oldestCursor: "cursor-1", isLoading: false },
      },
    },
  },
}));

vi.mock("@/lib/api", () => ({
  listTaskSessionMessages: mocks.listTaskSessionMessages,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      ...mocks.state,
      prependMessages: mocks.prependMessages,
      setMessagesMetadata: mocks.setMessagesMetadata,
    }),
}));

import { useLazyLoadMessages } from "./use-lazy-load-messages";

describe("useLazyLoadMessages", () => {
  beforeEach(() => {
    mocks.state.messages.metaBySession["session-1"].isLoading = false;
    mocks.listTaskSessionMessages.mockReset();
    mocks.prependMessages.mockReset();
    mocks.setMessagesMetadata.mockReset();
  });

  it("shares an in-flight page request between automatic and explicit navigation callers", async () => {
    let resolvePage!: (value: { messages: unknown[]; has_more: boolean }) => void;
    mocks.listTaskSessionMessages.mockReturnValue(
      new Promise((resolve) => {
        resolvePage = resolve;
      }),
    );
    const { result } = renderHook(() => useLazyLoadMessages("session-1"));

    let automaticLoad!: Promise<number>;
    let navigationLoad!: Promise<number>;
    act(() => {
      automaticLoad = result.current.loadMore();
      navigationLoad = result.current.loadMore();
    });

    expect(mocks.listTaskSessionMessages).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolvePage({
        messages: [
          {
            id: "older-1",
            created_at: "2026-07-21T00:00:00Z",
            author_type: "agent",
            type: "message",
          },
        ],
        has_more: true,
      });
    });

    await expect(automaticLoad).resolves.toBe(1);
    await expect(navigationLoad).resolves.toBe(1);
    expect(mocks.prependMessages).toHaveBeenCalledTimes(1);
  });

  it("does not let a stale store loading flag block a cursor-backed page request", async () => {
    mocks.state.messages.metaBySession["session-1"].isLoading = true;
    mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    const { result } = renderHook(() => useLazyLoadMessages("session-1"));

    await act(() => result.current.loadMore());

    expect(mocks.listTaskSessionMessages).toHaveBeenCalledTimes(1);
    mocks.state.messages.metaBySession["session-1"].isLoading = false;
  });
});
