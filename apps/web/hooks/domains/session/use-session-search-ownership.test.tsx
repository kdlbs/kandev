import { StrictMode, type PropsWithChildren } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { useSessionSearch, type SessionSearchHook } from "./use-session-search";

type WireRequest = { id: string; action: string; payload: unknown };

// Only the wire transport is substituted. The hook, API and protocol client are real.
class DeferredSocket {
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static current: DeferredSocket;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  requests: WireRequest[] = [];
  settled = new Set<string>();

  constructor() {
    DeferredSocket.current = this;
  }

  send(data: string) {
    const request = JSON.parse(data) as WireRequest;
    if (request.action === "message.search") this.requests.push(request);
  }

  close() {
    this.readyState = DeferredSocket.CLOSED;
  }

  settle(index: number, type: "response" | "error", payload: unknown) {
    const request = this.requests[index];
    expect(request).toBeDefined();
    expect(this.settled.has(request.id)).toBe(false);
    this.settled.add(request.id);
    this.onmessage?.({ data: JSON.stringify({ id: request.id, type, payload }) });
  }
}

let client: WebSocketClient;
let previousClient: WebSocketClient | null;
let errors: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  vi.useFakeTimers();
  errors = vi.spyOn(console, "error").mockImplementation(() => {});
  previousClient = getWebSocketClient();
  vi.stubGlobal("WebSocket", DeferredSocket);
  client = new WebSocketClient("ws://session-search-test", undefined, { enabled: false });
  client.connect();
  DeferredSocket.current.readyState = DeferredSocket.OPEN;
  DeferredSocket.current.onopen?.();
  setWebSocketClient(client);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    DeferredSocket.current.requests.forEach((request, index) => {
      if (!DeferredSocket.current.settled.has(request.id)) {
        DeferredSocket.current.settle(index, "response", { hits: [], total: 0 });
      }
    });
  });
  client.disconnect();
  setWebSocketClient(previousClient);
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function advance(ms = 180) {
  await act(async () => vi.advanceTimersByTimeAsync(ms));
}

async function begin(search: SessionSearchHook, query = "alpha") {
  act(() => {
    search.open();
    search.setQuery(query);
  });
  await advance();
}

async function settle(index: number, outcome: "response" | "error", id = "current-hit") {
  await act(async () => {
    const payload =
      outcome === "error"
        ? { code: "search_failed", message: "Search failed" }
        : {
            hits: [
              {
                id,
                author_type: "agent",
                type: "text",
                snippet: id,
                created_at: "2026-10-06T00:00:00Z",
              },
            ],
            total: 1,
          };
    DeferredSocket.current.settle(index, outcome, payload);
  });
}

describe("query ownership through the real protocol", () => {
  // @covers AC-UI-SESSION-SEARCH-OWNERSHIP-001.1
  it.each(["", "   "])("retires a pending reply immediately when cleared to %j", async (query) => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    await begin(result.current);
    act(() => result.current.setQuery(query));
    await settle(0, "response", "retired-hit");
    expect(result.current.hits).toEqual([]);
    expect(result.current.isSearching).toBe(false);
    expect(result.current.query).toBe(query);
    await advance();
    expect(DeferredSocket.current.requests).toHaveLength(1);
  });

  it("rejects the previous query's reply before the replacement debounce", async () => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    await begin(result.current);
    act(() => result.current.setQuery("beta"));
    await settle(0, "response", "retired-alpha");
    expect(result.current.query).toBe("beta");
    expect(result.current.hits).toEqual([]);
    expect(DeferredSocket.current.requests).toHaveLength(1);
    await advance();
    await settle(1, "response", "beta-hit");
    expect(result.current.hits.map((hit) => hit.id)).toEqual(["beta-hit"]);
  });

  it("clears displayed results and selection at query edit", async () => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    await begin(result.current);
    await settle(0, "response", "alpha-hit");
    await act(async () => result.current.setActiveHit("alpha-hit"));
    act(() => result.current.setQuery("beta"));
    expect(result.current.hits).toEqual([]);
    expect(result.current.activeHitId).toBeNull();
  });

  it.each(["response", "error"] as const)(
    "ignores retired %s and finalization while the current request is pending",
    async (outcome) => {
      const { result } = renderHook(() => useSessionSearch("session-a"));
      await begin(result.current);
      act(() => result.current.setQuery("beta"));
      await advance();
      await settle(0, outcome, "retired-alpha");
      expect(result.current.isSearching).toBe(true);
      expect(result.current.hits).toEqual([]);
      expect(errors).not.toHaveBeenCalled();
      await settle(1, "response", "beta-hit");
      expect(result.current.isSearching).toBe(false);
      expect(result.current.hits.map((hit) => hit.id)).toEqual(["beta-hit"]);
    },
  );
});

describe("closed search lifetime through the real protocol", () => {
  // @covers AC-UI-SESSION-SEARCH-OWNERSHIP-001.2
  it.each(["response", "error"] as const)(
    "does not revive closed search on a late %s after reopen",
    async (outcome) => {
      const { result } = renderHook(() => useSessionSearch("session-a"));
      await begin(result.current);
      act(() => {
        result.current.close();
        result.current.open();
      });
      await settle(0, outcome, "closed-hit");
      expect(result.current.isOpen).toBe(true);
      expect(result.current.query).toBe("");
      expect(result.current.hits).toEqual([]);
      expect(result.current.isSearching).toBe(false);
      expect(errors).not.toHaveBeenCalled();
      await begin(result.current, "fresh");
      await settle(1, "response", "fresh-hit");
      expect(result.current.hits.map((hit) => hit.id)).toEqual(["fresh-hit"]);
    },
  );

  it("cancels queued work and rejects a retained query callback after close/reopen", async () => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    act(() => result.current.open());
    const retainedSetQuery = result.current.setQuery;
    act(() => {
      result.current.setQuery("queued");
      result.current.close();
      result.current.open();
    });
    act(() => retainedSetQuery("retired-callback"));
    await advance();
    expect(DeferredSocket.current.requests).toHaveLength(0);
    expect(result.current.query).toBe("");
    await begin(result.current, "fresh");
    await settle(0, "response");
    expect(result.current.hits).toHaveLength(1);
  });
});

describe("committed session ownership through the real protocol", () => {
  // @covers AC-UI-SESSION-SEARCH-OWNERSHIP-001.3
  it.each([
    ["session-b"],
    [null, "session-a"],
    [undefined, "session-a"],
    ["session-b", "session-a"],
  ])("retires issued work across committed session changes %j", async (...sessions) => {
    const { result, rerender } = renderHook(({ sessionId }) => useSessionSearch(sessionId), {
      initialProps: { sessionId: "session-a" as string | null | undefined },
    });
    await begin(result.current);
    sessions.forEach((sessionId) => rerender({ sessionId }));
    expect(result.current.query).toBe("");
    expect(result.current.isSearching).toBe(false);
    await settle(0, "response", "old-session-hit");
    expect(result.current.hits).toEqual([]);
    await begin(result.current, "current");
    expect(DeferredSocket.current.requests[1].payload).toEqual({
      session_id: sessions.at(-1),
      query: "current",
      limit: 50,
    });
    await settle(1, "response");
    expect(result.current.hits).toHaveLength(1);
  });

  it("rejects queued work and retained callbacks from an earlier session lifetime", async () => {
    const navigate = vi.fn(() => null);
    const loadOlder = vi.fn(async () => 0);
    const { result, rerender } = renderHook(
      ({ sessionId }) => useSessionSearch(sessionId, loadOlder, navigate),
      { initialProps: { sessionId: "session-a" } },
    );
    act(() => {
      result.current.open();
      result.current.setQuery("queued");
    });
    const retained = result.current;
    rerender({ sessionId: "session-b" });
    rerender({ sessionId: "session-a" });
    await act(async () => {
      retained.close();
      retained.setQuery("retired");
      retained.setActiveHit("retired");
    });
    await advance();
    expect(result.current.isOpen).toBe(true);
    expect(result.current.query).toBe("");
    expect(result.current.activeHitId).toBeNull();
    expect(DeferredSocket.current.requests).toHaveLength(0);
    expect(navigate).not.toHaveBeenCalled();
    expect(loadOlder).not.toHaveBeenCalled();
    act(() => result.current.close());
    act(() => retained.open());
    expect(result.current.isOpen).toBe(false);
  });
});

describe("mounted search ownership through the real protocol", () => {
  it("never admits a retained callback or logs a retired failure after unmount", async () => {
    const { result, unmount } = renderHook(() => useSessionSearch("session-a"));
    await begin(result.current);
    const retained = result.current;
    unmount();
    act(() => {
      retained.open();
      retained.setQuery("retired");
    });
    await advance();
    await settle(0, "error");
    expect(DeferredSocket.current.requests).toHaveLength(1);
    expect(errors).not.toHaveBeenCalled();
  });

  it("keeps current callbacks and settlement live through rerender and StrictMode replay", async () => {
    const wrapper = ({ children }: PropsWithChildren) => <StrictMode>{children}</StrictMode>;
    const { result, rerender } = renderHook(() => useSessionSearch("session-a"), { wrapper });
    await begin(result.current);
    const retainedSetQuery = result.current.setQuery;
    rerender();
    await settle(0, "response");
    expect(result.current.hits).toHaveLength(1);
    act(() => retainedSetQuery("still-current"));
    await advance();
    await settle(1, "response", "rerender-hit");
    expect(result.current.hits.map((hit) => hit.id)).toEqual(["rerender-hit"]);
  });

  it("retires one instance without invalidating another instance's pending search", async () => {
    const first = renderHook(() => useSessionSearch("session-a"));
    const second = renderHook(() => useSessionSearch("session-a"));
    await begin(first.result.current, "first");
    await begin(second.result.current, "second");
    act(() => first.result.current.close());
    await settle(0, "response", "retired-first");
    expect(second.result.current.isSearching).toBe(true);
    await settle(1, "response", "second-hit");
    expect(first.result.current.hits).toEqual([]);
    expect(second.result.current.hits.map((hit) => hit.id)).toEqual(["second-hit"]);
  });
});

describe("current searches and navigation through the real protocol", () => {
  // @covers AC-UI-SESSION-SEARCH-OWNERSHIP-001.4
  it("admits only the latest query at 180ms and preserves exact trimmed wire identity", async () => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    act(() => {
      result.current.open();
      result.current.setQuery("a");
      result.current.setQuery(" current ");
    });
    await advance(179);
    expect(DeferredSocket.current.requests).toHaveLength(0);
    await advance(1);
    expect(DeferredSocket.current.requests).toHaveLength(1);
    expect(DeferredSocket.current.requests[0].payload).toEqual({
      session_id: "session-a",
      query: "current",
      limit: 50,
    });
    expect(result.current.isSearching).toBe(true);
    await settle(0, "response");
    expect(result.current.hits.map((hit) => hit.id)).toEqual(["current-hit"]);
    expect(result.current.isSearching).toBe(false);
  });

  it("preserves newer results when the older request later fails", async () => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    await begin(result.current);
    act(() => result.current.setQuery("beta"));
    await advance();
    await settle(1, "response", "beta-hit");
    await settle(0, "error");
    expect(result.current.hits.map((hit) => hit.id)).toEqual(["beta-hit"]);
    expect(result.current.isSearching).toBe(false);
    expect(errors).not.toHaveBeenCalled();
  });

  it("preserves current failure diagnostics and settled empty state", async () => {
    const { result } = renderHook(() => useSessionSearch("session-a"));
    await begin(result.current);
    await settle(0, "error");
    expect(result.current.hits).toEqual([]);
    expect(result.current.isSearching).toBe(false);
    expect(errors).toHaveBeenCalledExactlyOnceWith("Session search failed:", expect.any(Error));
  });

  it("does not admit searches while the selected session is unavailable", async () => {
    const { result } = renderHook(() => useSessionSearch(null));
    await begin(result.current);
    expect(DeferredSocket.current.requests).toHaveLength(0);
    expect(result.current.isSearching).toBe(false);
  });

  it("preserves bounded backfill and cancellation of navigation after close", async () => {
    const navigate = vi.fn(() => null);
    const loadOlder = vi.fn(async () => 1);
    const { result } = renderHook(() => useSessionSearch("session-a", loadOlder, navigate));
    await act(async () => result.current.setActiveHit("missing"));
    expect(loadOlder).toHaveBeenCalledTimes(40);
    let finishPage: (count: number) => void = () => {};
    loadOlder.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishPage = resolve;
        }),
    );
    act(() => {
      result.current.setActiveHit("other");
    });
    act(() => result.current.close());
    await act(async () => finishPage(1));
    expect(loadOlder).toHaveBeenCalledTimes(41);
    expect(result.current.activeHitId).toBeNull();
  });
});
