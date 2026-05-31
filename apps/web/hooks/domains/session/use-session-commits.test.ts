import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";

const mockRequest = vi.fn();
// setSessionCommits is the only mock whose default behaviour matters: the
// trigger-bump regression tests assert what the store looks like before
// and after the refetch resolves. With a pure `vi.fn()` mock those
// assertions would be vacuous. Mirror the real action's anti-race guard
// (skip writes of `[]` over a populated list unless `allowEmpty` is set)
// so the tests cover the actual end-to-end behaviour, including the
// authoritative-empty path used after a `commits_reset` bump.
const mockSetSessionCommits = vi.fn(
  (sessionId: string, commits: unknown[], opts?: { allowEmpty?: boolean }) => {
    const sc = storeState.sessionCommits as { byEnvironmentId: Record<string, unknown[]> };
    const existing = sc.byEnvironmentId[sessionId];
    if (!opts?.allowEmpty && commits.length === 0 && existing && existing.length > 0) {
      return;
    }
    sc.byEnvironmentId[sessionId] = commits;
  },
);
const mockSetSessionCommitsLoading = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

let storeState: Record<string, unknown> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
}));

import { useSessionCommits } from "./use-session-commits";

function setStore(connectionStatus: "connected" | "disconnected" = "connected") {
  storeState = {
    environmentIdBySessionId: {} as Record<string, string>,
    sessionCommits: {
      byEnvironmentId: {} as Record<string, unknown>,
      loading: {} as Record<string, boolean>,
      refetchTrigger: {} as Record<string, number>,
    },
    connection: { status: connectionStatus },
    setSessionCommits: mockSetSessionCommits,
    setSessionCommitsLoading: mockSetSessionCommitsLoading,
  };
}

describe("useSessionCommits", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setStore();
  });

  afterEach(() => {
    cleanup();
  });

  it("stores commits when the backend returns a populated list", async () => {
    mockRequest.mockResolvedValueOnce({
      commits: [{ commit_sha: "abc", insertions: 10, deletions: 2 }],
    });

    renderHook(() => useSessionCommits("sess-1"));

    await waitFor(() => {
      expect(mockSetSessionCommits).toHaveBeenCalledWith(
        "sess-1",
        [{ commit_sha: "abc", insertions: 10, deletions: 2 }],
        undefined,
      );
    });
  });

  it("retries when the backend signals ready:false instead of overwriting with []", async () => {
    mockRequest.mockResolvedValueOnce({ commits: [], ready: false }).mockResolvedValueOnce({
      commits: [{ commit_sha: "abc", insertions: 5, deletions: 1 }],
    });

    renderHook(() => useSessionCommits("sess-1"));

    // First request fires immediately.
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    // The store must NOT be filled with the empty list — that would mask the
    // missing data and prevent any future load.
    expect(mockSetSessionCommits).not.toHaveBeenCalled();

    // The hook's setTimeout retry kicks in after ~2s; waitFor polls until it
    // does. Bump the timeout above the retry delay.
    await waitFor(
      () => {
        expect(mockRequest).toHaveBeenCalledTimes(2);
      },
      { timeout: 4000 },
    );
    await waitFor(() => {
      expect(mockSetSessionCommits).toHaveBeenCalledWith(
        "sess-1",
        [{ commit_sha: "abc", insertions: 5, deletions: 1 }],
        undefined,
      );
    });
  });

  it("keeps loading:true while a retry is scheduled", async () => {
    mockRequest.mockResolvedValueOnce({ commits: [], ready: false }).mockResolvedValueOnce({
      commits: [{ commit_sha: "abc" }],
    });

    renderHook(() => useSessionCommits("sess-1"));

    // First request resolves with ready:false — the hook should set loading
    // to true at the start, then leave it as-is (no setLoading(false) call)
    // until the retry path eventually succeeds.
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(mockSetSessionCommitsLoading).toHaveBeenCalledWith("sess-1", true));
    // Critical: setLoading(false) must NOT have been called yet — flipping
    // it during the retry window leaves consumers seeing { loading: false,
    // commits: [] } which is the "loaded but empty" lie this hook avoids.
    expect(
      mockSetSessionCommitsLoading.mock.calls.filter(([, value]) => value === false),
    ).toHaveLength(0);

    // Once the retry succeeds, loading flips to false on the success path.
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(2), { timeout: 4000 });
    await waitFor(() => expect(mockSetSessionCommitsLoading).toHaveBeenCalledWith("sess-1", false));
  });

  it("does not retry when ready is true (default success path)", async () => {
    mockRequest.mockResolvedValueOnce({
      commits: [{ commit_sha: "abc" }],
      ready: true,
    });

    renderHook(() => useSessionCommits("sess-1"));

    await waitFor(() => expect(mockSetSessionCommits).toHaveBeenCalledTimes(1));

    // Wait past the retry window — no second request should fire.
    await new Promise((resolve) => setTimeout(resolve, 2500));
    expect(mockRequest).toHaveBeenCalledTimes(1);
  });

  it("does not fetch when disconnected", () => {
    setStore("disconnected");
    renderHook(() => useSessionCommits("sess-1"));
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("does not fetch when sessionId is null", () => {
    renderHook(() => useSessionCommits(null));
    expect(mockRequest).not.toHaveBeenCalled();
  });
});

// Helper: seed store with one existing commit and a deferred request so we
// can observe the store mid-refetch (stale-while-revalidate test).
async function seedAndDeferRefetch(sessionId: string) {
  storeState.sessionCommits = {
    byEnvironmentId: {
      [sessionId]: [{ commit_sha: "old", insertions: 1, deletions: 0 }],
    },
    loading: {},
    refetchTrigger: { [sessionId]: 0 },
  };
  let resolveRequest!: (value: unknown) => void;
  mockRequest.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveRequest = resolve;
    }),
  );
  return resolveRequest;
}

function bumpTrigger(sessionId: string, value: number) {
  (storeState.sessionCommits as { refetchTrigger: Record<string, number> }).refetchTrigger = {
    [sessionId]: value,
  };
}

// Lives in its own describe so the outer block stays under the 100-line
// max-lines-per-function limit.
describe("useSessionCommits — stale-while-revalidate on trigger bump", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setStore();
  });

  afterEach(() => {
    cleanup();
  });

  it("refetches when refetchTrigger bumps without nulling the visible list", async () => {
    const resolveRequest = await seedAndDeferRefetch("sess-1");
    const { rerender } = renderHook(() => useSessionCommits("sess-1"));
    expect(mockRequest).not.toHaveBeenCalled();

    bumpTrigger("sess-1", 1);
    rerender();

    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    // During mid-refetch the store must still hold the OLD commits.
    const midRefetch = (storeState.sessionCommits as { byEnvironmentId: Record<string, unknown> })
      .byEnvironmentId;
    expect(midRefetch["sess-1"]).toEqual([{ commit_sha: "old", insertions: 1, deletions: 0 }]);

    resolveRequest({ commits: [{ commit_sha: "new", insertions: 2, deletions: 1 }], ready: true });
    await waitFor(() => {
      const after = (storeState.sessionCommits as { byEnvironmentId: Record<string, unknown[]> })
        .byEnvironmentId;
      expect(after["sess-1"]).toEqual([{ commit_sha: "new", insertions: 2, deletions: 1 }]);
    });
  });

  it("accepts an authoritative empty response on trigger bump", async () => {
    // After a `git reset`, the refetch legitimately returns []. Without
    // `allowEmpty: true`, the default guard in `setSessionCommits` would
    // silently drop that response and the panel would keep showing stale data.
    storeState.sessionCommits = {
      byEnvironmentId: {
        "sess-1": [
          { commit_sha: "a", insertions: 0, deletions: 0 },
          { commit_sha: "b", insertions: 0, deletions: 0 },
        ],
      },
      loading: {},
      refetchTrigger: { "sess-1": 0 },
    };
    mockRequest.mockResolvedValueOnce({ commits: [], ready: true });

    const { rerender } = renderHook(() => useSessionCommits("sess-1"));
    bumpTrigger("sess-1", 1);
    rerender();

    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    await waitFor(() => {
      expect(mockSetSessionCommits).toHaveBeenCalledWith("sess-1", [], { allowEmpty: true });
    });
    await waitFor(() => {
      const after = (storeState.sessionCommits as { byEnvironmentId: Record<string, unknown[]> })
        .byEnvironmentId;
      expect(after["sess-1"]).toEqual([]);
    });
  });

  it("drops a stale response when a newer fetch already started", async () => {
    storeState.sessionCommits = {
      byEnvironmentId: { "sess-1": [{ commit_sha: "initial" }] },
      loading: {},
      refetchTrigger: { "sess-1": 0 },
    };
    let resolveFirst!: (value: unknown) => void;
    let resolveSecond!: (value: unknown) => void;
    mockRequest
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveSecond = resolve;
        }),
      );

    const { rerender } = renderHook(() => useSessionCommits("sess-1"));

    bumpTrigger("sess-1", 1);
    rerender();
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));

    bumpTrigger("sess-1", 2);
    rerender();
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(2));

    resolveFirst({ commits: [{ commit_sha: "stale" }], ready: true });
    resolveSecond({ commits: [{ commit_sha: "fresh" }], ready: true });

    await waitFor(() => {
      const after = (storeState.sessionCommits as { byEnvironmentId: Record<string, unknown[]> })
        .byEnvironmentId;
      expect(after["sess-1"]).toEqual([{ commit_sha: "fresh" }]);
    });

    const winningCalls = mockSetSessionCommits.mock.calls.filter(
      ([, commits]) => Array.isArray(commits) && commits.length > 0,
    );
    expect(winningCalls).toHaveLength(1);
  });
});
