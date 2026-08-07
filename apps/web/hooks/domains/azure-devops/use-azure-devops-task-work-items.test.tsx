import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AzureDevOpsTaskWorkItem } from "@/lib/types/azure-devops";

const taskWorkItems: Record<string, AzureDevOpsTaskWorkItem[]> = {};
const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  setAll: vi.fn(),
}));

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listWorkspaceAzureDevOpsTaskWorkItems: mocks.list,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      azureDevOpsTaskWorkItems: { byTaskId: taskWorkItems },
      setAzureDevOpsTaskWorkItems: mocks.setAll,
    }),
}));

import {
  cacheAzureDevOpsTaskWorkItem,
  useAzureDevOpsTaskWorkItems,
} from "./use-azure-devops-task-work-items";

const linked: AzureDevOpsTaskWorkItem = {
  id: "link-1",
  taskId: "task-1",
  workspaceId: "workspace-1",
  projectId: "project-1",
  workItemId: 42,
  workItemUrl: "https://dev.azure.com/acme/project/_workitems/edit/42",
  title: "Add Azure quick actions",
  state: "To Do",
  type: "Issue",
  createdAt: "2026-07-30T00:00:00Z",
  updatedAt: "2026-07-30T00:00:00Z",
};

beforeEach(() => {
  mocks.list.mockReset();
  mocks.setAll.mockReset();
  for (const key of Object.keys(taskWorkItems)) delete taskWorkItems[key];
});

afterEach(() => vi.clearAllMocks());

describe("useAzureDevOpsTaskWorkItems", () => {
  it("merges a newly cached association into the first workspace response", async () => {
    mocks.list.mockResolvedValue({ taskWorkItems: {} });
    cacheAzureDevOpsTaskWorkItem("workspace-1", "task-1", linked);

    renderHook(() => useAzureDevOpsTaskWorkItems("workspace-1", "task-1"));

    await waitFor(() => expect(mocks.setAll).toHaveBeenCalledWith({ "task-1": [linked] }));
  });
});
