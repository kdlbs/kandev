/**
 * Tests for useLazyLoadMessages — verifies the hook reads its
 * live state (hasMore / oldestCursor) from the TanStack Query
 * cache at qk.session.messages(sid), and that loadMore writes
 * fetched older messages back into the same cache key.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { qk } from "@/lib/query/keys";
import type { MessagesData } from "@/lib/query/query-options/session";
import type { Message } from "@/lib/types/http";

const mockListTaskSessionMessages = vi.fn();

vi.mock("@/lib/api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListTaskSessionMessages(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ prependMessages: vi.fn(), setMessagesMetadata: vi.fn() }),
}));

import { useLazyLoadMessages } from "./use-lazy-load-messages";

function makeMessage(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    task_id: "task-1",
    session_id: "sess-1",
    author_type: "user",
    content: "hello",
    type: "message",
    created_at: "2024-01-01T00:00:00Z",
    ...overrides,
  } as Message;
}

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

describe("useLazyLoadMessages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("reads hasMore from the TanStack Query cache", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData<MessagesData>(qk.session.messages("sess-1"), {
      messages: [makeMessage({ id: "m1" })],
      hasMore: true,
      oldestCursor: "m1",
    });
    const { result } = renderHook(() => useLazyLoadMessages("sess-1"), {
      wrapper: makeWrapper(qc),
    });
    expect(result.current.hasMore).toBe(true);
  });

  it("returns hasMore=false when no cache entry exists", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useLazyLoadMessages("sess-1"), {
      wrapper: makeWrapper(qc),
    });
    expect(result.current.hasMore).toBe(false);
    expect(result.current.isLoading).toBe(false);
  });

  it("loadMore writes fetched older messages into the TQ cache", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData<MessagesData>(qk.session.messages("sess-1"), {
      messages: [makeMessage({ id: "m2", created_at: "2024-01-02T00:00:00Z" })],
      hasMore: true,
      oldestCursor: "m2",
    });
    mockListTaskSessionMessages.mockResolvedValue({
      messages: [
        // backend returns desc order — hook reverses, so this becomes the oldest
        makeMessage({ id: "m1", created_at: "2024-01-01T00:00:00Z" }),
      ],
      has_more: false,
    });
    const { result } = renderHook(() => useLazyLoadMessages("sess-1"), {
      wrapper: makeWrapper(qc),
    });
    await act(async () => {
      await result.current.loadMore();
    });
    const cached = qc.getQueryData<MessagesData>(qk.session.messages("sess-1"));
    expect(cached?.messages.map((m) => m.id)).toEqual(["m1", "m2"]);
    expect(cached?.hasMore).toBe(false);
    expect(cached?.oldestCursor).toBe("m1");
  });

  it("loadMore is a no-op when hasMore is false", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData<MessagesData>(qk.session.messages("sess-1"), {
      messages: [makeMessage({ id: "m1" })],
      hasMore: false,
      oldestCursor: "m1",
    });
    const { result } = renderHook(() => useLazyLoadMessages("sess-1"), {
      wrapper: makeWrapper(qc),
    });
    let returned = -1;
    await act(async () => {
      returned = await result.current.loadMore();
    });
    expect(returned).toBe(0);
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("loadMore is a no-op when sessionId is null", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useLazyLoadMessages(null), {
      wrapper: makeWrapper(qc),
    });
    let returned = -1;
    await act(async () => {
      returned = await result.current.loadMore();
    });
    expect(returned).toBe(0);
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });
});

describe("useLazyLoadMessages — cache subscription", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("ignores non-'updated' cache events (infinite-render-loop regression)", () => {
    // Regression for commit 84843ff: the cache subscription must filter to
    // `event.type === "updated"`. Without it, `observerOptionsUpdated` events —
    // which fire on every render that passes a fresh options object to useQuery —
    // call setTick on every render and trip an infinite loop (React #185).
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData<MessagesData>(qk.session.messages("sess-1"), {
      messages: [makeMessage({ id: "m1" })],
      hasMore: true,
      oldestCursor: "m1",
    });

    let renderCount = 0;
    renderHook(
      () => {
        renderCount++;
        return useLazyLoadMessages("sess-1");
      },
      { wrapper: makeWrapper(qc) },
    );

    const rendersAfterMount = renderCount;
    const query = qc.getQueryCache().find({ queryKey: qk.session.messages("sess-1") });
    expect(query).toBeDefined();

    // Fire a non-"updated" cache notification for the same query key. The
    // subscriber must short-circuit on `event.type !== "updated"`, so no tick
    // (and therefore no re-render) is scheduled.
    act(() => {
      qc.getQueryCache().notify({
        type: "observerOptionsUpdated",
        query: query!,
      } as never);
    });

    expect(renderCount).toBe(rendersAfterMount);
  });
});
