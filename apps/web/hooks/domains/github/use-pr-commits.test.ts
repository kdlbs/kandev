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
      const unsubscribe = resource.subscribe(request.sourceKey, vi.fn());

      const failed = resource.load(request);
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(10);
      await expect(failed).resolves.toMatchObject({
        commits: [],
        providerHead: null,
        providerCommitsComplete: false,
        error: "final provider failure",
      });
      expect(resource.getSnapshot(request.sourceKey).error).toBe("final provider failure");

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
      providerHead: "first-sha",
      providerCommitsComplete: true,
      loading: false,
      error: null,
    };

    expect(resolvePRCommitsView(staleState, "workspace-1/acme/other-app/2/new-refresh")).toEqual({
      commits: [],
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
