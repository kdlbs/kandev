import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AzureDevOpsPullRequest } from "@/lib/types/azure-devops";
import { invalidateIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";

const apiMocks = vi.hoisted(() => ({
  config: vi.fn(),
  feedback: vi.fn(),
  pullRequests: vi.fn(),
  workItems: vi.fn(),
}));

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsConfig: apiMocks.config,
  getAzureDevOpsPullRequestFeedback: apiMocks.feedback,
  listAzureDevOpsPullRequests: apiMocks.pullRequests,
  searchAzureDevOpsWorkItems: apiMocks.workItems,
}));

import {
  useAzureDevOpsConnection,
  useAzureDevOpsPullRequestFeedback,
  useAzureDevOpsPullRequestSearch,
  useAzureDevOpsWorkItemSearch,
} from "./use-azure-devops-browse";

const WORKSPACE_A = "workspace-a";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((next, fail) => {
    resolve = next;
    reject = fail;
  });
  return { promise, resolve, reject };
}

const pullRequest = {
  id: 42,
  projectId: "project-1",
  repositoryId: "repo-1",
} as AzureDevOpsPullRequest;

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(cleanup);

describe("Azure DevOps connection hook", () => {
  it("clears connection data while switching workspaces", async () => {
    const nextWorkspace = deferred<{ hasSecret: boolean; organizationUrl: string }>();
    apiMocks.config
      .mockResolvedValueOnce({ hasSecret: true, organizationUrl: "https://dev.azure.com/first" })
      .mockReturnValueOnce(nextWorkspace.promise);
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useAzureDevOpsConnection(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    await waitFor(() => expect(result.current.data?.organizationUrl).toContain("first"));

    rerender({ workspaceId: "workspace-b" });
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
  });

  it("reloads connection data on refresh", async () => {
    apiMocks.config.mockResolvedValue({ hasSecret: true, lastOk: true });
    const { result } = renderHook(() => useAzureDevOpsConnection(WORKSPACE_A));

    await waitFor(() => expect(apiMocks.config).toHaveBeenCalledTimes(1));
    act(() => result.current.refresh());

    await waitFor(() => expect(apiMocks.config).toHaveBeenCalledTimes(2));
  });

  it("reloads connection data when integration availability is invalidated", async () => {
    apiMocks.config.mockResolvedValue({ hasSecret: true, lastOk: true });
    renderHook(() => useAzureDevOpsConnection(WORKSPACE_A));

    await waitFor(() => expect(apiMocks.config).toHaveBeenCalledTimes(1));
    act(() => invalidateIntegrationAvailability());

    await waitFor(() => expect(apiMocks.config).toHaveBeenCalledTimes(2));
  });

  it("keeps a connected state usable during the scheduled background refresh", async () => {
    vi.useFakeTimers();
    try {
      const refresh = deferred<{ hasSecret: boolean; lastOk: boolean }>();
      apiMocks.config
        .mockResolvedValueOnce({ hasSecret: true, lastOk: true })
        .mockReturnValueOnce(refresh.promise);
      const { result } = renderHook(() => useAzureDevOpsConnection(WORKSPACE_A));

      await vi.waitFor(() => expect(result.current.data?.hasSecret).toBe(true));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(INTEGRATION_STATUS_REFRESH_MS);
      });

      expect(apiMocks.config).toHaveBeenCalledTimes(2);
      expect(result.current.data?.hasSecret).toBe(true);
      expect(result.current.loading).toBe(false);
      expect(result.current.refreshing).toBe(true);

      await act(async () => {
        refresh.resolve({ hasSecret: true, lastOk: true });
        await refresh.promise;
      });
      expect(result.current.refreshing).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("preserves a connected state during and after a failed background refresh", async () => {
    const refresh = deferred<{ hasSecret: boolean; lastOk: boolean }>();
    apiMocks.config
      .mockResolvedValueOnce({ hasSecret: true, lastOk: true })
      .mockReturnValueOnce(refresh.promise);
    const { result } = renderHook(() => useAzureDevOpsConnection(WORKSPACE_A));

    await waitFor(() => expect(result.current.data?.hasSecret).toBe(true));
    act(() => invalidateIntegrationAvailability());
    await waitFor(() => expect(apiMocks.config).toHaveBeenCalledTimes(2));

    expect(result.current.data?.hasSecret).toBe(true);
    expect(result.current.loading).toBe(false);

    await act(async () => refresh.reject(new Error("Azure temporarily unavailable")));
    await waitFor(() => expect(result.current.error).toContain("temporarily unavailable"));
    expect(result.current.data?.hasSecret).toBe(true);
    expect(result.current.loading).toBe(false);
  });

  it("removes eligibility when a background refresh confirms no connection", async () => {
    apiMocks.config
      .mockResolvedValueOnce({ hasSecret: true, lastOk: true })
      .mockResolvedValueOnce(null);
    const { result } = renderHook(() => useAzureDevOpsConnection(WORKSPACE_A));

    await waitFor(() => expect(result.current.data?.hasSecret).toBe(true));
    act(() => invalidateIntegrationAvailability());

    await waitFor(() => expect(result.current.data).toBeNull());
    expect(result.current.loading).toBe(false);
  });
});

describe("Azure DevOps browse hooks", () => {
  it("ignores a work-item response from the previous workspace", async () => {
    const stale = deferred<{ items: Array<{ id: number }> }>();
    apiMocks.workItems.mockReturnValueOnce(stale.promise);
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useAzureDevOpsWorkItemSearch(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    act(() => void result.current.search({ project: "project-a", wiql: "SELECT" }));
    rerender({ workspaceId: "workspace-b" });
    await act(async () => stale.resolve({ items: [{ id: 1 }] }));

    expect(result.current.data).toEqual([]);
    expect(result.current.loading).toBe(false);
  });

  it("keeps feedback cleared when an older request finishes", async () => {
    const pending = deferred<{ reviewState: string }>();
    apiMocks.feedback.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useAzureDevOpsPullRequestFeedback(WORKSPACE_A));
    act(() => void result.current.load(pullRequest));
    act(() => result.current.clear());
    await act(async () => pending.resolve({ reviewState: "approved" }));

    expect(result.current.data).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("surfaces pull-request search errors", async () => {
    apiMocks.pullRequests.mockRejectedValueOnce(new Error("Azure unavailable"));
    const { result } = renderHook(() => useAzureDevOpsPullRequestSearch(WORKSPACE_A));
    await act(async () => {
      await result.current.search({ project: "project-a", repository: "repo-a" });
    });
    expect(result.current.error).toContain("Azure unavailable");
  });
});
