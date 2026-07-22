import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useGitLabStatus } from "./use-gitlab-status";

const fetchGitLabStatusMock = vi.fn();
const setStatus = vi.fn();
const setStatusLoading = vi.fn();
const workspaceA = "workspace-a";
const workspaceB = "workspace-b";
const gitLabAHost = "https://gitlab-a.example";

let activeWorkspaceId: string | null = workspaceA;
let cachedStatus = {
  workspaceId: workspaceA as string | null,
  data: null as { host: string } | null,
  loading: false,
  loadedAt: null as number | null,
};

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  fetchGitLabStatus: (...args: unknown[]) => fetchGitLabStatusMock(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      workspaces: { activeId: activeWorkspaceId },
      gitlabStatus: cachedStatus,
      setGitLabStatus: setStatus,
      setGitLabStatusLoading: setStatusLoading,
    }),
}));

describe("useGitLabStatus", () => {
  beforeEach(() => {
    activeWorkspaceId = workspaceA;
    cachedStatus = { workspaceId: workspaceA, data: null, loading: false, loadedAt: null };
    fetchGitLabStatusMock.mockReset().mockResolvedValue({ host: gitLabAHost });
    setStatus.mockReset();
    setStatusLoading.mockReset();
  });

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
    expect(setStatus).toHaveBeenLastCalledWith(workspaceB, null);

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
    cachedStatus = {
      workspaceId: workspaceA,
      data: { host: gitLabAHost },
      loading: false,
      loadedAt: 1,
    };
    fetchGitLabStatusMock.mockReset().mockResolvedValue({ host: gitLabAHost });
    setStatus.mockReset();
    setStatusLoading.mockReset();
    const { result, rerender } = renderHook(() => useGitLabStatus());
    expect(result.current.status).toEqual({ host: gitLabAHost });

    activeWorkspaceId = workspaceB;
    rerender();

    expect(result.current.status).toBeNull();
  });
});
