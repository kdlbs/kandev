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
});
