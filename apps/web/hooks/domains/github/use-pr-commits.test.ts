import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { PRCommitInfo } from "@/lib/types/github";
import { createPRCommitsResource, type PRCommitsRequest } from "./pr-commits-resource";
import { resolvePRCommitsView, usePRCommits, type KeyedPRCommitsState } from "./use-pr-commits";

const requestMock = vi.hoisted(() => vi.fn());
let websocketClient: { request: typeof requestMock } | null = { request: requestMock };
const SHARED_COMMIT_SHA = "shared-sha";
const RETRY_COMMIT_SHA = "retry-sha";
const REFRESHED_COMMIT_SHA = "refreshed-sha";
const STABLE_COMMIT_SHA = "stable-sha";
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => websocketClient,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  requestMock.mockReset();
  websocketClient = { request: requestMock };
});

afterEach(() => {
  cleanup();
});

function wrapper({ children }: { children: ReactNode }) {
  const initialState = {
    workspaces: { activeId: "workspace-1" },
  } as unknown as Partial<AppState>;
  return createElement(StateProvider, { initialState, children });
}

function commit(sha: string): PRCommitInfo {
  return {
    sha,
    message: sha,
    author_login: "octocat",
    author_date: "2026-08-04T12:00:00Z",
    additions: 1,
    deletions: 0,
    files_changed: 1,
    stats_available: false,
  };
}

function resourceRequest(sourceKey: string, prNumber = 1): PRCommitsRequest {
  return {
    workspaceId: "workspace-1",
    owner: "acme",
    repo: "app",
    prNumber,
    sourceKey,
  };
}

describe("usePRCommits request ownership", () => {
  it("shares one in-flight request across concurrent consumers", async () => {
    const pending = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    requestMock.mockReturnValue(pending.promise);

    const first = renderHook(() => usePRCommits("acme", "app", 1, "concurrent"), { wrapper });
    const second = renderHook(() => usePRCommits("acme", "app", 1, "concurrent"), { wrapper });

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));

    await act(async () => {
      pending.resolve({
        commits: [commit(SHARED_COMMIT_SHA)],
        head_sha: SHARED_COMMIT_SHA,
        complete: true,
      });
      await pending.promise;
    });

    await waitFor(() => expect(first.result.current.providerHead).toBe(SHARED_COMMIT_SHA));
    expect(second.result.current.providerHead).toBe(SHARED_COMMIT_SHA);
  });

  it("retries one failed request and exposes the successful retry", async () => {
    requestMock
      .mockRejectedValueOnce(new Error("temporary provider failure"))
      .mockResolvedValueOnce({
        commits: [commit(RETRY_COMMIT_SHA)],
        head_sha: RETRY_COMMIT_SHA,
        complete: true,
      });

    const { result } = renderHook(() => usePRCommits("acme", "app", 1, "retry"), { wrapper });

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.providerHead).toBe(RETRY_COMMIT_SHA));
  });

  it("keeps a final error internal and retries it for a later consumer", async () => {
    vi.useFakeTimers();
    try {
      const requester = vi
        .fn()
        .mockRejectedValueOnce(new Error("first provider failure"))
        .mockRejectedValueOnce(new Error("final provider failure"))
        .mockResolvedValueOnce({
          commits: [commit("recovered-sha")],
          head_sha: "recovered-sha",
          complete: true,
        });
      const resource = createPRCommitsResource(requester, { retryDelayMs: 10 });
      const request = resourceRequest("final-error");
      const unsubscribe = resource.subscribe(request, vi.fn());

      const failed = resource.load(request);
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(10);
      await expect(failed).resolves.toMatchObject({
        commits: [],
        providerHead: null,
        providerCommitsComplete: false,
        error: "final provider failure",
      });
      expect(resource.getSnapshot(request).error).toBe("final provider failure");

      const recovered = resource.load(request);
      await expect(recovered).resolves.toMatchObject({ providerHead: "recovered-sha" });
      expect(requester).toHaveBeenCalledTimes(3);
      unsubscribe();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("usePRCommits manual refresh", () => {
  it("shares a manual refresh and publishes it to every consumer", async () => {
    const refreshed = deferred<{
      commits: PRCommitInfo[];
      head_sha: string;
      complete: boolean;
    }>();
    requestMock
      .mockResolvedValueOnce({
        commits: [commit("initial-sha")],
        head_sha: "initial-sha",
        complete: true,
      })
      .mockReturnValueOnce(refreshed.promise);

    const first = renderHook(() => usePRCommits("acme", "app", 1, "manual"), { wrapper });
    const second = renderHook(() => usePRCommits("acme", "app", 1, "manual"), { wrapper });
    await waitFor(() => expect(first.result.current.providerHead).toBe("initial-sha"));

    let firstRefresh: Promise<unknown> | undefined;
    let secondRefresh: Promise<unknown> | undefined;
    await act(async () => {
      firstRefresh = first.result.current.refresh();
      secondRefresh = second.result.current.refresh();
      await Promise.resolve();
    });
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
    expect(firstRefresh).toBeDefined();
    expect(secondRefresh).toBeDefined();

    await act(async () => {
      refreshed.resolve({
        commits: [commit(REFRESHED_COMMIT_SHA)],
        head_sha: REFRESHED_COMMIT_SHA,
        complete: true,
      });
      await refreshed.promise;
    });
    await waitFor(() => expect(first.result.current.providerHead).toBe(REFRESHED_COMMIT_SHA));
    expect(second.result.current.providerHead).toBe(REFRESHED_COMMIT_SHA);
  });
});

describe("usePRCommits retained evidence", () => {
  // @covers AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.7
  it("retains resolved display commits while a same-PR sync-version refresh is pending", async () => {
    const refresh = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    requestMock
      .mockResolvedValueOnce({
        commits: [commit(STABLE_COMMIT_SHA)],
        head_sha: STABLE_COMMIT_SHA,
        complete: true,
      })
      .mockReturnValueOnce(refresh.promise);

    const { result, rerender } = renderHook(
      ({ syncVersion }) => usePRCommits("acme", "app", 1, syncVersion),
      { initialProps: { syncVersion: "retention-old-sync" }, wrapper },
    );
    await waitFor(() => expect(result.current.commits[0]?.sha).toBe(STABLE_COMMIT_SHA));

    rerender({ syncVersion: "retention-new-sync" });
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));

    expect(result.current.commits[0]?.sha).toBe(STABLE_COMMIT_SHA);
    expect(result.current.authoritativeCommits).toEqual([]);
    expect(result.current.loading).toBe(true);

    await act(async () => {
      refresh.resolve({
        commits: [commit(STABLE_COMMIT_SHA)],
        head_sha: STABLE_COMMIT_SHA,
        complete: true,
      });
      await refresh.promise;
    });
  });

  it("retains resolved display commits when a same-PR refresh fails", async () => {
    const requester = vi
      .fn()
      .mockResolvedValueOnce({
        commits: [commit(STABLE_COMMIT_SHA)],
        head_sha: STABLE_COMMIT_SHA,
        complete: true,
      })
      .mockRejectedValueOnce(new Error("temporary refresh failure"))
      .mockRejectedValueOnce(new Error("final refresh failure"));
    const resource = createPRCommitsResource(requester, { retryDelayMs: 0 });

    await resource.load(resourceRequest("failure-old-version"));
    const failed = await resource.load(resourceRequest("failure-new-version"));

    expect(failed).toMatchObject({
      commits: [commit(STABLE_COMMIT_SHA)],
      authoritativeCommits: [],
      providerHead: null,
      providerCommitsComplete: false,
      loading: false,
      error: "final refresh failure",
    });
  });

  it("replaces retained display commits after a same-PR refresh succeeds", async () => {
    requestMock
      .mockResolvedValueOnce({
        commits: [commit(STABLE_COMMIT_SHA)],
        head_sha: STABLE_COMMIT_SHA,
        complete: true,
      })
      .mockResolvedValueOnce({
        commits: [commit(REFRESHED_COMMIT_SHA)],
        head_sha: REFRESHED_COMMIT_SHA,
        complete: true,
      });

    const { result, rerender } = renderHook(
      ({ syncVersion }) => usePRCommits("acme", "app", 1, syncVersion),
      { initialProps: { syncVersion: "success-old-version" }, wrapper },
    );
    await waitFor(() => expect(result.current.commits[0]?.sha).toBe(STABLE_COMMIT_SHA));

    rerender({ syncVersion: "success-new-version" });

    await waitFor(() => expect(result.current.commits[0]?.sha).toBe(REFRESHED_COMMIT_SHA));
    expect(result.current.authoritativeCommits[0]?.sha).toBe(REFRESHED_COMMIT_SHA);
    expect(result.current.loading).toBe(false);
  });

  it("does not retain commits across different PR identities", async () => {
    const pending = deferred<{ commits: PRCommitInfo[] }>();
    const requester = vi
      .fn()
      .mockResolvedValueOnce({
        commits: [commit("first-pr-sha")],
        head_sha: "first-pr-sha",
        complete: true,
      })
      .mockReturnValueOnce(pending.promise);
    const resource = createPRCommitsResource(requester);
    const secondPR = resourceRequest("second-pr-version", 2);

    await resource.load(resourceRequest("first-pr-version", 1));
    const secondLoad = resource.load(secondPR);

    expect(resource.getSnapshot(secondPR).commits).toEqual([]);
    pending.resolve({ commits: [] });
    await secondLoad;
  });
});

describe("usePRCommits unavailable client", () => {
  it("retains resolved display commits when the WebSocket client is unavailable", async () => {
    requestMock.mockResolvedValueOnce({
      commits: [commit(STABLE_COMMIT_SHA)],
      head_sha: STABLE_COMMIT_SHA,
      complete: true,
    });
    const resource = createPRCommitsResource(undefined, { retryDelayMs: 0 });
    const initialRequest = resourceRequest("null-old-version");
    const refreshedRequest = resourceRequest("null-new-version");

    await resource.load(initialRequest);
    websocketClient = null;

    const failed = await resource.load(refreshedRequest);

    expect(failed.commits).toEqual([commit(STABLE_COMMIT_SHA)]);
    expect(failed.authoritativeCommits).toEqual([]);
    expect(failed.error).not.toBeNull();
    expect(requestMock).toHaveBeenCalledTimes(1);
  });
});

describe("usePRCommits stale results and eviction", () => {
  it("does not let an older sync-version response replace the active result", async () => {
    const first = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    const second = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    requestMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      ({ syncVersion }) => usePRCommits("acme", "app", 1, syncVersion),
      { initialProps: { syncVersion: "old-sync" }, wrapper },
    );
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    rerender({ syncVersion: "new-sync" });
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));

    await act(async () => {
      second.resolve({ commits: [commit("new-sha")], head_sha: "new-sha", complete: true });
      await second.promise;
    });
    await waitFor(() => expect(result.current.providerHead).toBe("new-sha"));

    await act(async () => {
      first.resolve({ commits: [commit("old-sha")], head_sha: "old-sha", complete: true });
      await first.promise;
    });
    expect(result.current.providerHead).toBe("new-sha");
  });

  it("retains the latest selected version after an older request resolves last", async () => {
    const first = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    const second = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    const third = deferred<{ commits: PRCommitInfo[]; head_sha: string; complete: boolean }>();
    const requester = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
      .mockReturnValueOnce(third.promise);
    const resource = createPRCommitsResource(requester);
    const firstRequest = resourceRequest("retained-old-version");
    const secondRequest = resourceRequest("retained-current-version");
    const thirdRequest = resourceRequest("retained-next-version");

    const firstLoad = resource.load(firstRequest);
    const secondLoad = resource.load(secondRequest);
    second.resolve({ commits: [commit("new-sha")], head_sha: "new-sha", complete: true });
    await secondLoad;
    first.resolve({ commits: [commit("old-sha")], head_sha: "old-sha", complete: true });
    await firstLoad;

    const thirdLoad = resource.load(thirdRequest);

    expect(resource.getSnapshot(thirdRequest).commits[0]?.sha).toBe("new-sha");
    third.resolve({ commits: [commit("third-sha")], head_sha: "third-sha", complete: true });
    await thirdLoad;
  });

  it("evicts superseded and old cache entries", async () => {
    const requester = vi.fn().mockImplementation(async (request: PRCommitsRequest) => ({
      commits: [commit(request.sourceKey)],
      head_sha: request.sourceKey,
      complete: true,
    }));
    const resource = createPRCommitsResource(requester, { maxEntries: 2 });

    await resource.load(resourceRequest("sync-a"));
    await resource.load(resourceRequest("sync-b"));
    requester.mockClear();
    await resource.load(resourceRequest("sync-a"));
    expect(requester).toHaveBeenCalledTimes(1);

    await resource.load(resourceRequest("other-pr", 2));
    await resource.load(resourceRequest("third-pr", 3));
    requester.mockClear();
    await resource.load(resourceRequest("sync-a"));
    expect(requester).toHaveBeenCalledTimes(1);
  });
});

describe("usePRCommits hook view", () => {
  it("masks state unless the complete workspace and PR source key matches", () => {
    const staleState: KeyedPRCommitsState = {
      sourceKey: "workspace-1/acme/app/1/old-refresh",
      commits: [commit("first-sha")],
      authoritativeCommits: [commit("first-sha")],
      providerHead: "first-sha",
      providerCommitsComplete: true,
      loading: false,
      error: null,
    };

    expect(resolvePRCommitsView(staleState, "workspace-1/acme/other-app/2/new-refresh")).toEqual({
      commits: [],
      authoritativeCommits: [],
      providerHead: null,
      providerCommitsComplete: false,
      loading: true,
      error: null,
    });
  });

  it("masks the previous PR while a rapid switch is loading", async () => {
    const first = deferred<{ commits: PRCommitInfo[] }>();
    const second = deferred<{ commits: PRCommitInfo[] }>();
    requestMock.mockImplementation((_action: string, payload: { number: number }) =>
      payload.number === 1 ? first.promise : second.promise,
    );

    const { result, rerender } = renderHook(
      ({ number }) => usePRCommits("acme", "app", number, "switch"),
      { initialProps: { number: 1 }, wrapper },
    );

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    rerender({ number: 2 });

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
    await act(async () => {
      second.resolve({ commits: [commit("second-sha")] });
      await second.promise;
    });
    await waitFor(() => expect(result.current.commits[0]?.sha).toBe("second-sha"));

    await act(async () => {
      first.resolve({ commits: [commit("late-first-sha")] });
      await first.promise;
    });

    expect(result.current.commits[0]?.sha).toBe("second-sha");
  });

  it("exposes the provider head and commit-list completeness", async () => {
    requestMock.mockResolvedValue({
      commits: [commit("provider-head")],
      head_sha: "provider-head",
      complete: true,
    });

    const { result } = renderHook(() => usePRCommits("acme", "app", 1, "head"), { wrapper });

    await waitFor(() => expect(result.current.providerHead).toBe("provider-head"));
    expect(result.current.providerCommitsComplete).toBe(true);
  });

  it("refreshes provider evidence on demand", async () => {
    requestMock
      .mockResolvedValueOnce({
        commits: [commit("old-provider-head")],
        head_sha: "old-provider-head",
        complete: true,
      })
      .mockResolvedValueOnce({
        commits: [commit("new-provider-head")],
        head_sha: "new-provider-head",
        complete: true,
      });

    const { result } = renderHook(() => usePRCommits("acme", "app", 1, "refresh"), { wrapper });
    await waitFor(() => expect(result.current.providerHead).toBe("old-provider-head"));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.providerHead).toBe("new-provider-head");
    expect(requestMock).toHaveBeenCalledTimes(2);
  });
});
