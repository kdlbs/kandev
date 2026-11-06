import { useLayoutEffect } from "react";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import { TaskWorkspaceProvider } from "@/components/task/task-workspace-context";
import { AzureDevOpsTaskPullRequestChip } from "./azure-devops-task-pull-request-chip";

const api = vi.hoisted(() => ({ list: vi.fn(), sync: vi.fn() }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listWorkspaceAzureDevOpsTaskPullRequests: api.list,
  syncAzureDevOpsTaskPullRequest: api.sync,
}));
const pr: AzureDevOpsTaskPullRequest = {
  id: "beta-association",
  taskId: "beta-task",
  repositoryId: "beta-repo",
  organizationUrl: "https://dev.azure.com/beta",
  projectId: "project",
  azureRepositoryId: "repo",
  pullRequestId: 42,
  pullRequestUrl: "https://dev.azure.com/beta/project/_git/repo/pullrequest/42",
  title: "Beta pull request",
  sourceBranch: "feature",
  targetBranch: "main",
  authorId: "author",
  authorName: "Ada",
  status: "active",
  isDraft: false,
  lastSyncedAt: "2099-01-01T00:00:00Z",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
const alpha = { ...pr, id: "alpha-association", taskId: "alpha-task" };
let store: StoreApi<AppState>;
function Capture() {
  const current = useAppStoreApi();
  useLayoutEffect(() => {
    store = current;
  });
  return null;
}
function mount() {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: "alpha", items: [] },
        azureDevOpsTaskPullRequests: { byTaskId: { "alpha-task": [alpha] } },
      }}
    >
      <Capture />
      <TooltipProvider>
        <TaskWorkspaceProvider taskId="beta-task" workspaceId="beta" sessionId="beta-session">
          <AzureDevOpsTaskPullRequestChip taskId="beta-task" />
        </TaskWorkspaceProvider>
      </TooltipProvider>
    </StateProvider>,
  );
}
beforeEach(() => {
  api.list.mockReset().mockResolvedValue({ taskPrs: { "beta-task": [pr] } });
  api.sync.mockReset().mockResolvedValue(pr);
});
afterEach(cleanup);

it("renders the original Azure link without replacing another workspace's cache", async () => {
  const view = mount();
  const ordinary = store.getState().azureDevOpsTaskPullRequests;
  await waitFor(() => expect(view.getByRole("link").getAttribute("href")).toBe(pr.pullRequestUrl));
  expect(api.list.mock.calls[0][0]).toBe("beta");
  expect(store.getState().azureDevOpsTaskPullRequests).toBe(ordinary);
  expect(store.getState().workspaces.activeId).toBe("alpha");
});

it("does not reuse an embedded Azure response across authorization generations", async () => {
  const view = mount();
  await waitFor(() => expect(view.queryByRole("link")).not.toBeNull());
  const pending = Promise.withResolvers<{
    taskPrs: Record<string, AzureDevOpsTaskPullRequest[]>;
  }>();
  api.list.mockReturnValueOnce(pending.promise);
  act(() =>
    store.setState((state) => ({
      workspaceContextGeneration: state.workspaceContextGeneration + 1,
    })),
  );
  expect(view.queryByRole("link")).toBeNull();
  await act(async () => pending.resolve({ taskPrs: {} }));
  expect(view.queryByRole("link")).toBeNull();
  expect(api.list).toHaveBeenCalledTimes(2);
});

it("aborts embedded reads on unmount without publishing into the ordinary store", async () => {
  const pending = Promise.withResolvers<{
    taskPrs: Record<string, AzureDevOpsTaskPullRequest[]>;
  }>();
  api.list.mockReturnValueOnce(pending.promise);
  const view = mount();
  const ordinary = store.getState().azureDevOpsTaskPullRequests;
  const signal = api.list.mock.calls[0][1].init.signal as AbortSignal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => pending.resolve({ taskPrs: { "beta-task": [pr] } }));
  expect(store.getState().azureDevOpsTaskPullRequests).toBe(ordinary);
});

it("refreshes a stale embedded association in its original workspace only", async () => {
  api.list.mockResolvedValue({
    taskPrs: { "beta-task": [{ ...pr, lastSyncedAt: "2020-01-01T00:00:00Z" }] },
  });
  api.sync.mockResolvedValue({
    ...pr,
    pullRequestId: 43,
    pullRequestUrl: pr.pullRequestUrl.replace("42", "43"),
  });
  const view = mount();
  const ordinary = store.getState().azureDevOpsTaskPullRequests;
  await waitFor(() =>
    expect(view.getByRole("link").getAttribute("href")).toBe(pr.pullRequestUrl.replace("42", "43")),
  );
  expect(api.sync).toHaveBeenCalledWith(
    "beta",
    "beta-task",
    { repositoryId: "beta-repo", pullRequestId: 42 },
    expect.objectContaining({ init: expect.objectContaining({ signal: expect.any(AbortSignal) }) }),
  );
  expect(store.getState().azureDevOpsTaskPullRequests).toBe(ordinary);
});

it("never derives a VCS sync from a retired authorization response", async () => {
  const old = Promise.withResolvers<{ taskPrs: Record<string, AzureDevOpsTaskPullRequest[]> }>();
  api.list.mockReturnValueOnce(old.promise).mockResolvedValue({ taskPrs: {} });
  const view = mount();
  act(() =>
    store.setState((state) => ({
      workspaceContextGeneration: state.workspaceContextGeneration + 1,
    })),
  );
  await act(async () =>
    old.resolve({ taskPrs: { "beta-task": [{ ...pr, lastSyncedAt: "2020-01-01T00:00:00Z" }] } }),
  );
  expect(api.sync).not.toHaveBeenCalled();
  expect(view.queryByRole("link")).toBeNull();
});
