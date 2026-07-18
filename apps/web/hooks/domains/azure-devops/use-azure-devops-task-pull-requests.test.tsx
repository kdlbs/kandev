import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

const apiMocks = vi.hoisted(() => ({
  list: vi.fn(),
  sync: vi.fn(),
}));

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listWorkspaceAzureDevOpsTaskPullRequests: apiMocks.list,
  syncAzureDevOpsTaskPullRequest: apiMocks.sync,
}));

import {
  cacheAzureDevOpsTaskPullRequest,
  useAzureDevOpsTaskPullRequests,
} from "./use-azure-devops-task-pull-requests";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function taskPullRequest(
  overrides: Partial<AzureDevOpsTaskPullRequest> = {},
): AzureDevOpsTaskPullRequest {
  const now = new Date().toISOString();
  return {
    id: "link-1",
    taskId: "task-1",
    repositoryId: "repo-1",
    organizationUrl: "https://dev.azure.com/acme",
    projectId: "project-1",
    azureRepositoryId: "azure-repo-1",
    pullRequestId: 42,
    pullRequestUrl: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
    title: "Initial title",
    sourceBranch: "feature/azure",
    targetBranch: "main",
    authorId: "author-1",
    authorName: "Ada",
    status: "active",
    isDraft: false,
    lastSyncedAt: now,
    createdAt: now,
    updatedAt: now,
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((next, fail) => {
    resolve = next;
    reject = fail;
  });
  return { promise, reject, resolve };
}

beforeEach(() => {
  apiMocks.list.mockReset();
  apiMocks.sync.mockReset();
});

afterEach(cleanup);

describe("useAzureDevOpsTaskPullRequests", () => {
  it("preserves a new association in the workspace snapshot across remounts", async () => {
    apiMocks.list.mockResolvedValue({ taskPrs: {} });
    const first = renderHook(() => useAzureDevOpsTaskPullRequests("workspace-cache", "task-1"), {
      wrapper,
    });
    await waitFor(() => expect(apiMocks.list).toHaveBeenCalledTimes(1));

    const linked = taskPullRequest();
    act(() => cacheAzureDevOpsTaskPullRequest("workspace-cache", "task-1", linked));
    first.unmount();

    const second = renderHook(() => useAzureDevOpsTaskPullRequests("workspace-cache", "task-1"), {
      wrapper,
    });
    await waitFor(() => expect(second.result.current).toEqual([linked]));
    expect(apiMocks.list).toHaveBeenCalledTimes(1);
  });

  it("refreshes a stale association when its task chip is displayed", async () => {
    const stale = taskPullRequest({
      id: "link-stale",
      title: "Stale title",
      lastSyncedAt: "2020-01-01T00:00:00Z",
    });
    const refreshed = taskPullRequest({
      id: "link-stale",
      title: "Current title",
      lastSyncedAt: new Date().toISOString(),
    });
    apiMocks.list.mockResolvedValue({ taskPrs: { "task-1": [stale] } });
    apiMocks.sync.mockResolvedValue(refreshed);

    const { result } = renderHook(
      () => useAzureDevOpsTaskPullRequests("workspace-refresh", "task-1"),
      { wrapper },
    );

    await waitFor(() => expect(result.current[0]?.title).toBe("Current title"));
    expect(apiMocks.sync).toHaveBeenCalledWith("workspace-refresh", "task-1", {
      repositoryId: "repo-1",
      pullRequestId: 42,
    });
  });

  it("deduplicates concurrent workspace loads", async () => {
    const pending = deferred<{ taskPrs: Record<string, AzureDevOpsTaskPullRequest[]> }>();
    apiMocks.list.mockReturnValue(pending.promise);
    const first = taskPullRequest({ id: "link-one", taskId: "task-1" });
    const second = taskPullRequest({ id: "link-two", taskId: "task-2" });
    const { result } = renderHook(
      () => ({
        first: useAzureDevOpsTaskPullRequests("workspace-concurrent", "task-1"),
        second: useAzureDevOpsTaskPullRequests("workspace-concurrent", "task-2"),
      }),
      { wrapper },
    );
    await waitFor(() => expect(apiMocks.list).toHaveBeenCalledTimes(1));
    await act(async () => pending.resolve({ taskPrs: { "task-1": [first], "task-2": [second] } }));
    await waitFor(() => expect(result.current).toEqual({ first: [first], second: [second] }));
  });

  it("retries a failed workspace load after remount", async () => {
    apiMocks.list.mockRejectedValueOnce(new Error("offline"));
    const first = renderHook(() => useAzureDevOpsTaskPullRequests("workspace-retry", "task-1"), {
      wrapper,
    });
    await waitFor(() => expect(apiMocks.list).toHaveBeenCalledTimes(1));
    first.unmount();

    const linked = taskPullRequest({ id: "link-retry" });
    apiMocks.list.mockResolvedValueOnce({ taskPrs: { "task-1": [linked] } });
    const second = renderHook(() => useAzureDevOpsTaskPullRequests("workspace-retry", "task-1"), {
      wrapper,
    });
    await waitFor(() => expect(second.result.current).toEqual([linked]));
    expect(apiMocks.list).toHaveBeenCalledTimes(2);
  });

  it("replaces task associations when the workspace changes", async () => {
    const first = taskPullRequest({ id: "link-workspace-a", title: "Workspace A" });
    const second = taskPullRequest({ id: "link-workspace-b", title: "Workspace B" });
    apiMocks.list.mockImplementation((workspaceId: string) =>
      Promise.resolve({
        taskPrs: { "task-1": [workspaceId === "workspace-switch-a" ? first : second] },
      }),
    );
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useAzureDevOpsTaskPullRequests(workspaceId, "task-1"),
      { initialProps: { workspaceId: "workspace-switch-a" }, wrapper },
    );
    await waitFor(() => expect(result.current).toEqual([first]));
    rerender({ workspaceId: "workspace-switch-b" });
    await waitFor(() => expect(result.current).toEqual([second]));
  });
});
