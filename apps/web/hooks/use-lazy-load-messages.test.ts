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
  bySession: [] as Array<{ id: string; author_type?: string; prompt_index?: number }>,
  setMessagesMetadata: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      messages: {
        metaBySession: { s1: storeMock.meta },
        bySession: { s1: storeMock.bySession },
      },
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      messages: {
        bySession: { s1: storeMock.bySession },
        metaBySession: { s1: storeMock.meta },
      },
      setMessagesMetadata: storeMock.setMessagesMetadata,
      prependMessages: (_sessionId: string, messages: Array<{ id: string }>, _meta: unknown) => {
        storeMock.prepended = [...messages, ...storeMock.prepended];
        storeMock.bySession = [...messages, ...storeMock.bySession];
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
  storeMock.bySession = [];
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

function wireTypedResponse(
  entries: Array<{ id: string; author_type?: string; prompt_index?: number }>,
  hasMore: boolean,
) {
  return {
    messages: entries.map((entry) => ({
      id: entry.id,
      author_type: entry.author_type ?? "agent",
      prompt_index: entry.prompt_index,
      created_at: "2026-01-01T00:00:00.000000Z",
    })),
    has_more: hasMore,
  };
}

describe("useLazyLoadMessages loadMore", () => {
  it("hides older pages once the first user prompt is loaded", async () => {
    storeMock.bySession = [{ id: "first", author_type: "user", prompt_index: 1 }];
    const { result } = renderHook(() => useLazyLoadMessages("s1"));

    expect(result.current.hasMore).toBe(false);
    await act(async () => {
      await result.current.loadMore();
    });
    expect(listTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("keeps raw backfill available after the visible prompt boundary", async () => {
    storeMock.bySession = [{ id: "first", author_type: "user", prompt_index: 1 }];
    listTaskSessionMessages.mockResolvedValueOnce(wireResponse(["hidden"], false));
    const { result } = renderHook(() => useLazyLoadMessages("s1"));

    expect(result.current.hasMore).toBe(false);
    expect(result.current.rawHasMore).toBe(true);
    await act(async () => {
      await result.current.loadMoreRaw();
    });
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
  });

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

  it("keeps raw pagination when loaded messages have no prompt ordinal", () => {
    storeMock.bySession = [{ id: "legacy-user", author_type: "user" }];
    const { result } = renderHook(() => useLazyLoadMessages("s1"));

    expect(result.current.hasMore).toBe(true);
  });
});

describe("useLazyLoadMessages minUserPromptsPerLoad", () => {
  it("loads multiple pages until at least the threshold of user prompts arrives", async () => {
    listTaskSessionMessages
      .mockResolvedValueOnce(
        wireTypedResponse(
          [
            { id: "m4", author_type: "user" },
            { id: "m3", author_type: "agent" },
            { id: "m2", author_type: "user" },
          ],
          true,
        ),
      )
      .mockResolvedValueOnce(wireTypedResponse([{ id: "m1", author_type: "user" }], false));
    const { result } = renderHook(() => useLazyLoadMessages("s1", { minUserPromptsPerLoad: 3 }));

    await act(async () => {
      await result.current.loadMore();
    });

    // Page 1 carries only 2 user prompts (below the threshold of 3), so a
    // second page is fetched; page 2 brings the total to 3 and the loop stops.
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(2);
    expect(listTaskSessionMessages).toHaveBeenNthCalledWith(2, "s1", {
      limit: 20,
      before: "m2",
      sort: "desc",
    });
    expect(storeMock.bySession.filter((m) => m.author_type === "user")).toHaveLength(3);
  });

  it("continues the accumulation loop after joining a request once it settles", async () => {
    const { promise: pagePromise, resolve: resolvePage } = Promise.withResolvers<unknown>();
    listTaskSessionMessages.mockReturnValueOnce(pagePromise);
    listTaskSessionMessages.mockResolvedValueOnce(
      wireTypedResponse([{ id: "m1", author_type: "user" }], false),
    );
    const { result } = renderHook(() => useLazyLoadMessages("s1", { minUserPromptsPerLoad: 3 }));

    // A transcript-owned request is in flight for the current cursor; the
    // panel's loadMore joins it.
    storeMock.meta.isLoadingMore = true;
    const loadPromise = result.current.loadMore();
    // The joined flight is the session's LAST: the coordinator clears the
    // store's flag before the join promise resolves.
    await act(async () => {
      storeMock.meta.isLoadingMore = false;
      resolvePage(
        wireTypedResponse(
          [
            { id: "m4", author_type: "user" },
            { id: "m3", author_type: "agent" },
          ],
          true,
        ),
      );
    });
    await act(async () => {
      await loadPromise;
    });

    // The loop continued past the joined page (page 2 fetched) instead of
    // stopping on the stale in-flight flag.
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(2);
    expect(listTaskSessionMessages).toHaveBeenNthCalledWith(2, "s1", {
      limit: 20,
      before: "m3",
      sort: "desc",
    });
  });

  it("stops after a zero-result page even when the threshold is unmet", async () => {
    listTaskSessionMessages
      .mockResolvedValueOnce(wireTypedResponse([{ id: "m1", author_type: "user" }], true))
      .mockResolvedValueOnce(wireTypedResponse([], true));
    const { result } = renderHook(() => useLazyLoadMessages("s1", { minUserPromptsPerLoad: 3 }));

    await act(async () => {
      await result.current.loadMore();
    });

    expect(listTaskSessionMessages).toHaveBeenCalledTimes(2);
  });

  it("stops accumulating when a page loads prompt #1", async () => {
    listTaskSessionMessages
      .mockResolvedValueOnce(
        wireTypedResponse(
          [
            { id: "m3", author_type: "agent" },
            { id: "m2", author_type: "user", prompt_index: 2 },
          ],
          true,
        ),
      )
      .mockResolvedValueOnce(
        wireTypedResponse([{ id: "m1", author_type: "user", prompt_index: 1 }], true),
      )
      .mockResolvedValueOnce(wireTypedResponse([{ id: "hidden" }], false));
    const { result } = renderHook(() => useLazyLoadMessages("s1", { minUserPromptsPerLoad: 3 }));

    await act(async () => {
      await result.current.loadMore();
    });

    expect(listTaskSessionMessages).toHaveBeenCalledTimes(2);
    expect(storeMock.bySession.some((message) => message.prompt_index === 1)).toBe(true);
  });

  it("fetches a single page when no threshold is set (transcript behavior)", async () => {
    listTaskSessionMessages.mockResolvedValueOnce(
      wireTypedResponse([{ id: "m1", author_type: "user" }], false),
    );
    const { result } = renderHook(() => useLazyLoadMessages("s1"));

    await act(async () => {
      await result.current.loadMore();
    });

    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
  });
});
