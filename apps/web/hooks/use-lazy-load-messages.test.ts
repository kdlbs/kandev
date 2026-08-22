import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const listTaskSessionMessages = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", () => ({ listTaskSessionMessages }));

const storeMock = vi.hoisted(() => ({
  meta: {
    hasMore: true,
    oldestCursor: "m3" as string | null,
    isLoading: false,
    isLoadingMore: false,
  },
  prepended: [] as Array<{ id: string }>,
  setMessagesMetadata: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      messages: {
        metaBySession: { s1: storeMock.meta },
        bySession: { s1: [] },
      },
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      messages: {
        bySession: { s1: [] as Array<{ id: string }> },
        metaBySession: { s1: storeMock.meta },
      },
      setMessagesMetadata: storeMock.setMessagesMetadata,
      prependMessages: (_sessionId: string, messages: Array<{ id: string }>, _meta: unknown) => {
        storeMock.prepended = [...messages, ...storeMock.prepended];
      },
    }),
  }),
}));

import { useLazyLoadMessages } from "./use-lazy-load-messages";

beforeEach(() => {
  listTaskSessionMessages.mockReset();
  storeMock.setMessagesMetadata.mockReset();
  storeMock.meta = { hasMore: true, oldestCursor: "m3", isLoading: false, isLoadingMore: false };
  storeMock.prepended = [];
});

afterEach(() => {
  vi.restoreAllMocks();
});

function wireResponse(ids: string[], hasMore: boolean) {
  return {
    messages: ids.map((id) => ({ id, created_at: "2026-01-01T00:00:00.000000Z" })),
    has_more: hasMore,
  };
}

describe("useLazyLoadMessages loadMore", () => {
  it("joins an in-flight request for the same cursor instead of skipping", async () => {
    let resolvePage: (value: unknown) => void = () => {};
    listTaskSessionMessages.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePage = resolve;
      }),
    );
    const { result } = renderHook(() => useLazyLoadMessages("s1"));

    // First caller starts the request (in flight).
    const first = result.current.loadMore();
    // The store now reports isLoadingMore (the coordinator raised it).
    storeMock.meta.isLoadingMore = true;
    storeMock.meta.oldestCursor = "m3";

    // A concurrent caller (e.g. the panel sentinel with join enabled) calls
    // loadMore while the request is in flight: it must JOIN the shared
    // promise (same count, no second HTTP request), not return 0.
    const second = result.current.loadMore();

    await act(async () => {
      resolvePage(wireResponse(["m3", "m2", "m1"], false));
    });
    const [firstCount, secondCount] = await Promise.all([first, second]);

    expect(firstCount).toBe(3);
    expect(secondCount).toBe(3);
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
    expect(storeMock.prepended.map((m) => m.id)).toEqual(["m1", "m2", "m3"]);
  });

  it("starts a fresh request when nothing is in flight", async () => {
    listTaskSessionMessages.mockResolvedValueOnce(wireResponse(["m3", "m2", "m1"], false));
    const { result } = renderHook(() => useLazyLoadMessages("s1"));

    await act(async () => {
      await result.current.loadMore();
    });

    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
    expect(listTaskSessionMessages).toHaveBeenCalledWith("s1", {
      limit: 20,
      before: "m3",
      sort: "desc",
    });
    expect(storeMock.prepended.map((m) => m.id)).toEqual(["m1", "m2", "m3"]);
  });

  it("skips when there is no cursor or no more pages", async () => {
    storeMock.meta = { hasMore: false, oldestCursor: null, isLoading: false, isLoadingMore: false };
    const { result } = renderHook(() => useLazyLoadMessages("s1"));
    await act(async () => {
      await result.current.loadMore();
    });
    expect(listTaskSessionMessages).not.toHaveBeenCalled();
  });
});
