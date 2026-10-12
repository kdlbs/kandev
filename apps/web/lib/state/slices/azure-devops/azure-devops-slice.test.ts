import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { createAzureDevOpsSlice } from "./azure-devops-slice";
import type { AzureDevOpsSlice } from "./types";
import type { AzureDevOpsTaskPullRequest, AzureDevOpsTaskWorkItem } from "@/lib/types/azure-devops";

const taskAssociationTimestamp = "2026-01-01T00:00:00Z";

function taskPullRequest(
  overrides: Partial<AzureDevOpsTaskPullRequest> = {},
): AzureDevOpsTaskPullRequest {
  return {
    id: "link-1",
    taskId: "task-1",
    repositoryId: "repo-1",
    organizationUrl: "https://dev.azure.com/acme",
    projectId: "project-1",
    azureRepositoryId: "azure-repo-1",
    pullRequestId: 42,
    pullRequestUrl: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
    title: "Ship integration",
    sourceBranch: "refs/heads/feature",
    targetBranch: "refs/heads/main",
    authorId: "user-1",
    authorName: "Alice",
    status: "active",
    isDraft: false,
    createdAt: taskAssociationTimestamp,
    updatedAt: taskAssociationTimestamp,
    ...overrides,
  };
}

function taskWorkItem(overrides: Partial<AzureDevOpsTaskWorkItem> = {}): AzureDevOpsTaskWorkItem {
  return {
    id: "link-1",
    taskId: "task-1",
    workspaceId: "workspace-1",
    projectId: "project-1",
    workItemId: 73,
    workItemUrl: "https://dev.azure.com/acme/_workitems/edit/73",
    title: "Add feature",
    state: "Active",
    type: "User Story",
    createdAt: taskAssociationTimestamp,
    updatedAt: taskAssociationTimestamp,
    ...overrides,
  };
}

function makeStore() {
  return create<AzureDevOpsSlice>()(immer((set) => createAzureDevOpsSlice(set)));
}

describe("Azure DevOps task PR slice", () => {
  it("starts with empty associations in an isolated store", () => {
    const store = makeStore();

    expect(store.getState().azureDevOpsTaskPullRequests).toEqual({ byTaskId: {} });
    expect(store.getState().azureDevOpsTaskWorkItems).toEqual({ byTaskId: {} });
  });

  it("replaces workspace task associations as one snapshot", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({
      "task-1": [taskPullRequest()],
      "task-2": [taskPullRequest({ id: "link-2", taskId: "task-2", pullRequestId: 7 })],
    });

    expect(Object.keys(store.getState().azureDevOpsTaskPullRequests.byTaskId)).toEqual([
      "task-1",
      "task-2",
    ]);
  });

  it("upserts by persisted association id without changing other tasks", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({
      "task-1": [taskPullRequest()],
      "task-2": [taskPullRequest({ id: "link-2", taskId: "task-2" })],
    });
    store
      .getState()
      .setAzureDevOpsTaskPullRequest(
        "task-1",
        taskPullRequest({ title: "Updated", reviewState: "approved" }),
      );

    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]).toHaveLength(1);
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]?.[0]?.title).toBe(
      "Updated",
    );
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-2"]).toHaveLength(1);

    store
      .getState()
      .setAzureDevOpsTaskPullRequest("task-1", taskPullRequest({ id: "link-3", pullRequestId: 9 }));
    expect(
      store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]?.map((item) => item.id),
    ).toEqual(["link-1", "link-3"]);
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-2"]).toHaveLength(1);
  });

  it("resets all Azure task associations", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({ "task-1": [taskPullRequest()] });
    store.getState().setAzureDevOpsTaskWorkItems({ "task-1": [taskWorkItem()] });
    store.getState().resetAzureDevOpsTaskPullRequests();
    store.getState().resetAzureDevOpsTaskWorkItems();

    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId).toEqual({});
    expect(store.getState().azureDevOpsTaskWorkItems.byTaskId).toEqual({});
  });

  it("upserts work items by persisted association id without changing other tasks", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskWorkItems({
      "task-1": [taskWorkItem()],
      "task-2": [taskWorkItem({ id: "link-2", taskId: "task-2" })],
    });
    store.getState().setAzureDevOpsTaskWorkItem("task-1", taskWorkItem({ title: "Updated" }));

    expect(store.getState().azureDevOpsTaskWorkItems.byTaskId["task-1"]?.[0]?.title).toBe(
      "Updated",
    );
    store
      .getState()
      .setAzureDevOpsTaskWorkItem("task-1", taskWorkItem({ id: "link-3", workItemId: 91 }));
    expect(
      store.getState().azureDevOpsTaskWorkItems.byTaskId["task-1"]?.map((item) => item.id),
    ).toEqual(["link-1", "link-3"]);
    expect(store.getState().azureDevOpsTaskWorkItems.byTaskId["task-2"]).toHaveLength(1);
  });

  it("composes in the root store and preserves unrelated state references", () => {
    const store = createAppStore();
    const unrelatedTasks = store.getState().tasks;
    const unrelatedWorkspaces = store.getState().workspaces;
    const pullRequest = taskPullRequest();
    const workItem = taskWorkItem();

    store.getState().setAzureDevOpsTaskPullRequest("task-1", pullRequest);
    store.getState().setAzureDevOpsTaskWorkItem("task-1", workItem);

    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]).toEqual([pullRequest]);
    expect(store.getState().azureDevOpsTaskWorkItems.byTaskId["task-1"]).toEqual([workItem]);
    expect(store.getState().tasks).toBe(unrelatedTasks);
    expect(store.getState().workspaces).toBe(unrelatedWorkspaces);
  });
});
