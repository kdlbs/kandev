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
