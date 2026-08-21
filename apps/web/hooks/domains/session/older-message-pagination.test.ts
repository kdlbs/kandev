import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { useAppStoreApi } from "@/components/state-provider";
import { joinOlderMessages, requestOlderMessages } from "./older-message-pagination";

const listTaskSessionMessages = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", () => ({ listTaskSessionMessages }));

type Meta = {
  isLoading: boolean;
  isLoadingMore: boolean;
  hasMore: boolean;
  oldestCursor: string | null;
};

type StoreLike = ReturnType<typeof useAppStoreApi>;

function makeStore(initialMeta?: Partial<Meta>) {
  const meta: Record<string, Meta> = {};
  // Like the real store, sessions with pagination state already have message
  // entries; the purge guard keys on bySession absence, so seed the sessions
  // the tests paginate.
  const bySession: Record<string, Array<{ id: string; created_at: string }>> = {
    s1: [],
    s2: [],
  };
  let prependCalls = 0;
  const setMeta = (sessionId: string, patch: Partial<Meta>) => {
    const defaults: Meta = {
      isLoading: false,
      isLoadingMore: false,
      hasMore: false,
      oldestCursor: null,
      ...initialMeta,
    };
    meta[sessionId] = { ...defaults, ...meta[sessionId], ...patch };
  };
  const store = {
    getState: () => ({
      messages: {
        bySession,
        metaBySession: meta,
      },
      setMessagesMetadata: (sessionId: string, patch: { isLoadingMore?: boolean }) => {
        setMeta(sessionId, patch);
      },
      prependMessages: (
        sessionId: string,
        messages: Array<{ id: string; created_at: string }>,
        patch: { hasMore: boolean; oldestCursor: string | null },
      ) => {
        prependCalls += 1;
        bySession[sessionId] = [...messages, ...(bySession[sessionId] ?? [])];
        setMeta(sessionId, patch);
      },
    }),
  } as unknown as StoreLike;
  return {
    store,
    meta,
    bySession,
    prependCalls: () => prependCalls,
  };
}

function pageResponse(ids: string[], hasMore: boolean) {
  // The API returns newest-first (sort=desc); ids are given newest-first.
  return {
    messages: ids.map((id, i) => ({ id, created_at: `2026-01-01T00:00:0${i}.000Z` })),
    has_more: hasMore,
  };
}

beforeEach(() => {
  listTaskSessionMessages.mockReset();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("requestOlderMessages", () => {
  it("dedups concurrent callers for one (session, cursor) with first-request-wins limit", async () => {
    const { store, meta, bySession, prependCalls } = makeStore();
    listTaskSessionMessages.mockResolvedValue(pageResponse(["m3", "m2", "m1"], true));

    // Panel, native transcript, automatic backfill, and last-prompt preload
    // all request the same cursor with different limits, synchronously.
    const panel = requestOlderMessages({ sessionId: "s1", cursor: "m3", limit: 20, store });
    const transcript = requestOlderMessages({ sessionId: "s1", cursor: "m3", limit: 20, store });
    const backfill = requestOlderMessages({ sessionId: "s1", cursor: "m3", limit: 100, store });
    const preload = requestOlderMessages({ sessionId: "s1", cursor: "m3", limit: 50, store });

    const [panelResult, transcriptResult, backfillResult, preloadResult] = await Promise.all([
      panel,
      transcript,
      backfill,
      preload,
    ]);

    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
    expect(prependCalls()).toBe(1);
    // Every follower receives the same response and effective limit.
    expect(panelResult.count).toBe(3);
    expect(transcriptResult).toEqual(panelResult);
    expect(backfillResult).toEqual(panelResult);
    expect(preloadResult).toEqual(panelResult);
    expect(panelResult.effectiveLimit).toBe(20); // first caller (panel) won
    // Monotonic cursor metadata after the merge.
    expect(meta.s1.oldestCursor).toBe("m1");
    expect(meta.s1.hasMore).toBe(true);
    expect(meta.s1.isLoadingMore).toBe(false);
    expect(meta.s1.isLoading).toBe(false);
    expect(bySession.s1.map((m) => m.id)).toEqual(["m1", "m2", "m3"]);
  });

  it("clears isLoadingMore on error without touching initial isLoading", async () => {
    const { store, meta } = makeStore();
    listTaskSessionMessages.mockRejectedValue(new Error("network"));

    await expect(
      requestOlderMessages({ sessionId: "s1", cursor: "m9", limit: 20, store }),
    ).rejects.toThrow("network");

    expect(meta.s1.isLoadingMore).toBe(false);
    expect(meta.s1.isLoading).toBe(false);
    expect(meta.s1.oldestCursor).toBeNull(); // no merge happened
  });

  it("does not interfere across sessions", async () => {
    const { store, meta } = makeStore();
    listTaskSessionMessages.mockImplementation((sessionId: string) =>
      Promise.resolve(pageResponse(sessionId === "s1" ? ["a1"] : ["b1"], false)),
    );

    const first = requestOlderMessages({ sessionId: "s1", cursor: "a1", limit: 20, store });
    const second = requestOlderMessages({ sessionId: "s2", cursor: "b1", limit: 100, store });
    await Promise.all([first, second]);

    expect(listTaskSessionMessages).toHaveBeenCalledTimes(2);
    expect(meta.s1.isLoadingMore).toBe(false);
    expect(meta.s2.isLoadingMore).toBe(false);
    expect(meta.s1.oldestCursor).toBe("a1");
    expect(meta.s2.oldestCursor).toBe("b1");
  });

  it("retains the cursor on a zero-row response so a retry re-requests the same page", async () => {
    const { store, meta, prependCalls } = makeStore();
    listTaskSessionMessages
      .mockResolvedValueOnce({ messages: [], has_more: true })
      .mockResolvedValueOnce(pageResponse(["m2", "m1"], false));

    const first = await requestOlderMessages({ sessionId: "s1", cursor: "m2", limit: 20, store });
    expect(first.count).toBe(0);
    expect(first.effectiveLimit).toBe(20);
    expect(first.hasMore).toBe(true);
    // The cursor is retained (not advanced to null) so the next attempt
    // re-requests the same page.
    expect(meta.s1.oldestCursor).toBe("m2");
    expect(meta.s1.hasMore).toBe(true);
    expect(prependCalls()).toBe(1);

    // A later retry for the retained cursor makes progress.
    const second = await requestOlderMessages({ sessionId: "s1", cursor: "m2", limit: 20, store });
    expect(second.count).toBe(2);
    expect(meta.s1.oldestCursor).toBe("m1");
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(2);
  });
});

describe("requestOlderMessages — joins and purged-session lifecycle", () => {
  it("exposes the in-flight promise via joinOlderMessages and shares its result", async () => {
    const { store, bySession } = makeStore();
    let resolvePage: (value: { messages: unknown[]; has_more: boolean }) => void = () => {};
    listTaskSessionMessages.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePage = resolve;
      }),
    );

    const starter = requestOlderMessages({ sessionId: "s1", cursor: "m3", limit: 20, store });
    // A concurrent caller asks the coordinator for the same cursor BEFORE its
    // local loading guards: it must receive the in-flight promise, not null.
    const joined = joinOlderMessages("s1", "m3");
    expect(joined).not.toBeNull();
    expect(joined).toBe(starter);

    resolvePage(pageResponse(["m3", "m2", "m1"], false));
    const [starterResult, joinedResult] = await Promise.all([starter, joined!]);
    expect(joinedResult).toEqual(starterResult);
    expect(joinedResult.count).toBe(3);
    expect(bySession.s1.map((m) => m.id)).toEqual(["m1", "m2", "m3"]);
    expect(listTaskSessionMessages).toHaveBeenCalledTimes(1);
  });

  it("returns null from joinOlderMessages when no request is in flight", async () => {
    const { store } = makeStore();
    expect(joinOlderMessages("s1", "m9")).toBeNull();
    listTaskSessionMessages.mockResolvedValueOnce({ messages: [], has_more: false });
    const done = await requestOlderMessages({ sessionId: "s1", cursor: "m9", limit: 20, store });
    expect(done.count).toBe(0);
    expect(joinOlderMessages("s1", "m9")).toBeNull(); // settled flight is gone
  });

  it("does not resurrect purged session state when the response lands after deletion", async () => {
    const { store, bySession, meta, prependCalls } = makeStore();
    let resolvePage: (value: { messages: unknown[]; has_more: boolean }) => void = () => {};
    listTaskSessionMessages.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePage = resolve;
      }),
    );

    const pending = requestOlderMessages({ sessionId: "s1", cursor: "m3", limit: 20, store });
    // The session is deleted while the request is in flight: the store purge
    // removes its bySession/metaBySession entries.
    delete bySession.s1;
    delete meta.s1;

    resolvePage(pageResponse(["m3", "m2", "m1"], false));
    const result = await pending;

    expect(result.count).toBe(3);
    // The prepend must NOT have recreated the purged session state.
    expect(prependCalls()).toBe(0);
    expect(bySession.s1).toBeUndefined();
    expect(meta.s1).toBeUndefined();
  });
});
