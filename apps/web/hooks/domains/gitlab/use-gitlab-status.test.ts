import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useGitLabStatus } from "./use-gitlab-status";

const fetchGitLabStatusMock = vi.fn();
const setStatus = vi.fn();
const setStatusLoading = vi.fn();
const resetStatus = vi.fn();
const workspaceA = "workspace-a";
const workspaceB = "workspace-b";
const gitLabAHost = "https://gitlab-a.example";

let activeWorkspaceId: string | null = workspaceA;
let cachedStatuses: Record<
  string,
  { data: { host: string } | null; loading: boolean; loadedAt: number | null }
> = {};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function statusEntry(workspaceId: string) {
  return (cachedStatuses[workspaceId] ??= { data: null, loading: false, loadedAt: null });
}

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  fetchGitLabStatus: (...args: unknown[]) => fetchGitLabStatusMock(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      workspaces: { activeId: activeWorkspaceId },
      gitlabStatus: { byWorkspaceId: cachedStatuses },
      setGitLabStatus: setStatus,
      setGitLabStatusLoading: setStatusLoading,
      resetGitLabStatus: resetStatus,
    }),
}));

beforeEach(() => {
  activeWorkspaceId = workspaceA;
  cachedStatuses = {};
  fetchGitLabStatusMock.mockReset().mockResolvedValue({ host: gitLabAHost });
  setStatus.mockReset();
  setStatusLoading.mockReset();
  resetStatus.mockReset();
  setStatus.mockImplementation((workspaceId: string, status: { host: string } | null) => {
    const entry = statusEntry(workspaceId);
    entry.data = status;
    entry.loadedAt = Date.now();
  });
  setStatusLoading.mockImplementation((workspaceId: string, loading: boolean) => {
    statusEntry(workspaceId).loading = loading;
  });
  resetStatus.mockImplementation((workspaceId: string) => {
    cachedStatuses[workspaceId] = { data: null, loading: false, loadedAt: null };
  });
});

describe("useGitLabStatus", () => {
  it("refetches status when the active workspace changes", async () => {
    const { rerender } = renderHook(() => useGitLabStatus());

    await waitFor(() =>
      expect(fetchGitLabStatusMock).toHaveBeenCalledWith({
        cache: "no-store",
        workspaceId: workspaceA,
      }),
    );

    activeWorkspaceId = workspaceB;
    rerender();

    await waitFor(() =>
      expect(fetchGitLabStatusMock).toHaveBeenLastCalledWith({
        cache: "no-store",
        workspaceId: workspaceB,
      }),
    );
  });
});

describe("useGitLabStatus stale workspace responses", () => {
  it("clears the previous status and ignores a stale workspace response", async () => {
    let resolveA: (value: { host: string }) => void = () => undefined;
    let resolveB: (value: { host: string }) => void = () => undefined;
    fetchGitLabStatusMock.mockImplementation(
      ({ workspaceId }: { workspaceId: string }) =>
        new Promise<{ host: string }>((resolve) => {
          if (workspaceId === workspaceA) resolveA = resolve;
          else resolveB = resolve;
        }),
    );
    const { rerender } = renderHook(() => useGitLabStatus());
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(1));

    activeWorkspaceId = workspaceB;
    rerender();
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(2));
    expect(resetStatus).toHaveBeenCalledWith(workspaceB);

    resolveA({ host: "https://stale.example" });
    await Promise.resolve();
    expect(setStatus).not.toHaveBeenCalledWith(workspaceA, { host: "https://stale.example" });

    resolveB({ host: "https://current.example" });
    await waitFor(() =>
      expect(setStatus).toHaveBeenLastCalledWith(workspaceB, {
        host: "https://current.example",
      }),
    );
  });

  it("keeps an imperative refresh from overwriting a newly selected workspace", async () => {
    const initialA = Promise.resolve({ host: "https://initial-a.example" });
    let resolveRefreshA: (value: { host: string }) => void = () => undefined;
    let resolveB: (value: { host: string }) => void = () => undefined;
    const refreshA = new Promise<{ host: string }>((resolve) => {
      resolveRefreshA = resolve;
    });
    const fetchB = new Promise<{ host: string }>((resolve) => {
      resolveB = resolve;
    });
    fetchGitLabStatusMock
      .mockImplementationOnce(() => initialA)
      .mockImplementationOnce(() => refreshA)
      .mockImplementationOnce(() => fetchB);

    const { result, rerender } = renderHook(() => useGitLabStatus());
    await waitFor(() =>
      expect(setStatus).toHaveBeenCalledWith(workspaceA, {
        host: "https://initial-a.example",
      }),
    );
    setStatus.mockClear();
    setStatusLoading.mockClear();

    const pendingRefresh = result.current.refresh();
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(2));
    activeWorkspaceId = workspaceB;
    rerender();
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(3));

    resolveRefreshA({ host: "https://stale-refresh-a.example" });
    await pendingRefresh;
    expect(
      setStatus.mock.calls.some(([, value]) => value?.host === "https://stale-refresh-a.example"),
    ).toBe(false);
    expect(setStatusLoading).not.toHaveBeenCalledWith(workspaceA, false);

    resolveB({ host: "https://current-b.example" });
    await waitFor(() =>
      expect(setStatus).toHaveBeenLastCalledWith(workspaceB, {
        host: "https://current-b.example",
      }),
    );
    expect(setStatusLoading).toHaveBeenLastCalledWith(workspaceB, false);
  });
});

describe("useGitLabStatus workspace ownership", () => {
  it("hides workspace A's cached status on the first render for workspace B", () => {
    activeWorkspaceId = workspaceA;
    cachedStatuses = {
      [workspaceA]: { data: { host: gitLabAHost }, loading: false, loadedAt: 1 },
    };
    fetchGitLabStatusMock.mockReset().mockResolvedValue({ host: gitLabAHost });
    setStatus.mockClear();
    setStatusLoading.mockClear();
    resetStatus.mockClear();
    const { result, rerender } = renderHook(() => useGitLabStatus());
    expect(result.current.status).toEqual({ host: gitLabAHost });

    activeWorkspaceId = workspaceB;
    rerender();

    expect(result.current.status).toBeNull();
  });
});

describe("useGitLabStatus requested workspace", () => {
  beforeEach(() => {
    activeWorkspaceId = workspaceA;
    cachedStatuses = {};
    fetchGitLabStatusMock.mockReset().mockResolvedValue({ host: gitLabAHost });
    setStatus.mockClear();
    setStatusLoading.mockClear();
    resetStatus.mockClear();
  });

  it("uses the requested workspace instead of the active workspace", async () => {
    renderHook(() => useGitLabStatus(workspaceB));

    await waitFor(() =>
      expect(fetchGitLabStatusMock).toHaveBeenCalledWith({
        cache: "no-store",
        workspaceId: workspaceB,
      }),
    );
    expect(resetStatus).toHaveBeenCalledWith(workspaceB);
  });

  it("keeps simultaneous workspace status requests isolated", async () => {
    const { rerender: rerenderA } = renderHook(() => useGitLabStatus(workspaceA));
    const { rerender: rerenderB } = renderHook(() => useGitLabStatus(workspaceB));

    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(2));
    rerenderA();
    rerenderB();

    expect(setStatus).not.toHaveBeenCalledWith(null, null);
    expect(setStatusLoading).not.toHaveBeenCalledWith(null, false);
    expect(resetStatus).toHaveBeenCalledWith(workspaceA);
    expect(resetStatus).toHaveBeenCalledWith(workspaceB);
  });
});

describe("useGitLabStatus shared request ownership", () => {
  beforeEach(() => {
    activeWorkspaceId = workspaceA;
    cachedStatuses = {
      [workspaceA]: { data: { host: gitLabAHost }, loading: false, loadedAt: 1 },
    };
    fetchGitLabStatusMock.mockReset().mockResolvedValue({ host: gitLabAHost });
    setStatus.mockClear();
    setStatusLoading.mockClear();
    resetStatus.mockClear();
  });

  it("completes a refresh after the consumer that started it unmounts", async () => {
    const initial = renderHook(() => useGitLabStatus());
    await waitFor(() => expect(cachedStatuses[workspaceA]?.data).toEqual({ host: gitLabAHost }));
    const refreshResponse = deferred<{ host: string }>();
    fetchGitLabStatusMock.mockReset().mockReturnValue(refreshResponse.promise);

    const repositoryConsumer = renderHook(() => useGitLabStatus(workspaceA));
    expect(cachedStatuses[workspaceA]).toEqual({
      data: { host: gitLabAHost },
      loading: false,
      loadedAt: 1,
    });

    act(() => {
      void repositoryConsumer.result.current.refresh();
    });
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(1));

    repositoryConsumer.unmount();
    await act(async () => {
      refreshResponse.resolve({ host: "https://gitlab-refreshed.example" });
      await refreshResponse.promise;
    });

    expect(cachedStatuses[workspaceA]).toMatchObject({
      data: { host: "https://gitlab-refreshed.example" },
      loading: false,
    });
    initial.unmount();
  });

  it("lets the latest same-workspace refresh win", async () => {
    const firstResponse = deferred<{ host: string }>();
    const secondResponse = deferred<{ host: string }>();
    fetchGitLabStatusMock.mockResolvedValue({ host: gitLabAHost });
    const { result, unmount } = renderHook(() => useGitLabStatus(workspaceA));
    await waitFor(() => expect(cachedStatuses[workspaceA]?.data).toEqual({ host: gitLabAHost }));
    fetchGitLabStatusMock
      .mockReset()
      .mockReturnValueOnce(firstResponse.promise)
      .mockReturnValueOnce(secondResponse.promise);

    const firstRefresh = result.current.refresh();
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(1));
    const secondRefresh = result.current.refresh();
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(2));

    await act(async () => {
      firstResponse.resolve({ host: "https://gitlab-stale.example" });
      await firstRefresh;
    });
    expect(cachedStatuses[workspaceA]?.data).toEqual({ host: gitLabAHost });
    expect(cachedStatuses[workspaceA]?.loading).toBe(true);

    await act(async () => {
      secondResponse.resolve({ host: "https://gitlab-current.example" });
      await secondRefresh;
    });
    expect(cachedStatuses[workspaceA]).toMatchObject({
      data: { host: "https://gitlab-current.example" },
      loading: false,
    });
    unmount();
  });
});
