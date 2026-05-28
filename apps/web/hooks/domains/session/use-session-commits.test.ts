import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";

const mockRequest = vi.fn();
const mockSetSessionCommits = vi.fn();
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
      expect(mockSetSessionCommits).toHaveBeenCalledWith("sess-1", [
        { commit_sha: "abc", insertions: 10, deletions: 2 },
      ]);
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
      expect(mockSetSessionCommits).toHaveBeenCalledWith("sess-1", [
        { commit_sha: "abc", insertions: 5, deletions: 1 },
      ]);
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

  it("refetches when refetchTrigger bumps without nulling the visible list", async () => {
    // Seed the store with existing commits so we can assert they survive the
    // bump. The bug we're guarding against is: a bump clears the list to
    // undefined, the hook returns `[]`, the Changes panel renders its empty
    // state, then the new list arrives — a visible flicker.
    storeState.sessionCommits = {
      byEnvironmentId: {
        "sess-1": [{ commit_sha: "old", insertions: 1, deletions: 0 }],
      },
      loading: {},
      refetchTrigger: { "sess-1": 0 },
    };
    mockRequest.mockResolvedValueOnce({
      commits: [{ commit_sha: "new", insertions: 2, deletions: 1 }],
      ready: true,
    });

    const { rerender } = renderHook(() => useSessionCommits("sess-1"));
    // Initial mount: commits is already populated, so no fetch should fire.
    expect(mockRequest).not.toHaveBeenCalled();

    // Simulate the WS handler bumping the trigger (commits_reset /
    // branch_switched). Critically, byEnvironmentId stays populated.
    (storeState.sessionCommits as { refetchTrigger: Record<string, number> }).refetchTrigger = {
      "sess-1": 1,
    };
    rerender();

    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    await waitFor(() => {
      expect(mockSetSessionCommits).toHaveBeenCalledWith("sess-1", [
        { commit_sha: "new", insertions: 2, deletions: 1 },
      ]);
    });

    // The old list must still be in the store throughout — the hook never
    // wiped it. Without stale-while-revalidate, the panel would flicker.
    const byEnv = (storeState.sessionCommits as { byEnvironmentId: Record<string, unknown> })
      .byEnvironmentId;
    expect(byEnv["sess-1"]).toEqual([{ commit_sha: "old", insertions: 1, deletions: 0 }]);
  });
});
