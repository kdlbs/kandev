import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AzureDevOpsTaskWorkItem, AzureDevOpsWorkItem } from "@/lib/types/azure-devops";

const apiMocks = vi.hoisted(() => ({
  detail: vi.fn(),
  comments: vi.fn(),
  links: vi.fn(),
  assignment: vi.fn(),
}));

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsWorkItemDetail: apiMocks.detail,
  listAzureDevOpsWorkItemComments: apiMocks.comments,
  listWorkspaceAzureDevOpsTaskWorkItems: apiMocks.links,
  updateAzureDevOpsWorkItemAssignment: apiMocks.assignment,
}));

import {
  mergeAzureDevOpsComments,
  useAzureDevOpsWorkItemDetail,
} from "./use-azure-devops-work-item-detail";

const workspaceId = "workspace-release";
const project = {
  id: "9b9336aa-2b0b-493b-8707-6adbaaf31284",
  name: "Release Planning",
};
const workItem = {
  id: 507,
  revision: 4,
  title: "Prepare release checklist",
  project: project.name,
  type: "Task",
  state: "Active",
  webUrl: "https://dev.azure.com/example/Release%20Planning/_workitems/edit/507",
} satisfies AzureDevOpsWorkItem;

function association(overrides: Partial<AzureDevOpsTaskWorkItem> = {}): AzureDevOpsTaskWorkItem {
  return {
    id: "association-release",
    taskId: "task-release",
    workspaceId,
    projectId: workItem.project,
    workItemId: workItem.id,
    workItemUrl: workItem.webUrl,
    title: workItem.title,
    state: workItem.state,
    type: workItem.type,
    createdAt: "2026-10-18T00:00:00Z",
    updatedAt: "2026-10-18T00:00:00Z",
    ...overrides,
  };
}

function respondWithLinks(rows: AzureDevOpsTaskWorkItem[]) {
  apiMocks.links.mockResolvedValue({
    taskWorkItems: Object.fromEntries(rows.map((row) => [row.taskId, [row]])),
  });
}

async function loadLinkedTasks(item: AzureDevOpsWorkItem = workItem, selectedProject = project.id) {
  const { result } = renderHook(() =>
    useAzureDevOpsWorkItemDetail(workspaceId, selectedProject, item, true),
  );
  await waitFor(() => expect(result.current.linkedTasksLoading).toBe(false));
  expect(apiMocks.links).toHaveBeenCalledWith(workspaceId, { cache: "no-store" });
  return result.current.linkedTasks;
}

beforeEach(() => {
  vi.clearAllMocks();
  apiMocks.detail.mockResolvedValue({ ...workItem, planningFields: [] });
  apiMocks.comments.mockResolvedValue({ comments: [] });
  respondWithLinks([]);
});

afterEach(cleanup);

describe("useAzureDevOpsWorkItemDetail linked tasks", () => {
  // @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.12
  it("shows name-valued first-party links when the selected project is a GUID", async () => {
    const linked = association();
    respondWithLinks([linked]);

    expect(await loadLinkedTasks()).toEqual([linked]);
  });

  it("keeps existing GUID links", async () => {
    const linked = association({ projectId: project.id });
    respondWithLinks([linked]);

    expect(await loadLinkedTasks()).toEqual([linked]);
  });

  // @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.13
  it.each([undefined, "", "   "])(
    "falls back to the selected ID without a usable item project name (%s)",
    async (name) => {
      const linked = association({ projectId: project.id });
      respondWithLinks([
        linked,
        association({ id: "missing", taskId: "task-missing", projectId: "" }),
        association({ id: "blank", taskId: "task-blank", projectId: "   " }),
        association({ id: "name", taskId: "task-name" }),
      ]);

      expect(await loadLinkedTasks({ ...workItem, project: name })).toEqual([linked]);
    },
  );

  it("returns each association once when project aliases coincide", async () => {
    const linked = association();
    respondWithLinks([linked]);

    expect(await loadLinkedTasks(workItem, project.name)).toEqual([linked]);
  });

  // @covers AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.12, AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.14
  it("retains all matching links and excludes foreign workspace project and item rows", async () => {
    const nameLink = association();
    const idLink = association({ id: "id-link", taskId: "task-id", projectId: project.id });
    respondWithLinks([
      nameLink,
      association({ id: "other-project", taskId: "task-project", projectId: "Other Project" }),
      idLink,
      association({ id: "other-item", taskId: "task-item", workItemId: workItem.id + 1 }),
      association({
        id: "other-workspace",
        taskId: "task-workspace",
        workspaceId: "workspace-other",
        projectId: project.id,
      }),
    ]);

    expect(await loadLinkedTasks()).toEqual([nameLink, idLink]);
  });

  it("does not normalize unrelated project references", async () => {
    const linked = association({ projectId: project.id });
    respondWithLinks([
      linked,
      association({ id: "case", taskId: "task-case", projectId: project.name.toLowerCase() }),
      association({ id: "padded", taskId: "task-padded", projectId: ` ${project.name} ` }),
      association({ id: "id-case", taskId: "task-id-case", projectId: project.id.toUpperCase() }),
      association({ id: "empty", taskId: "task-empty", projectId: "" }),
    ]);

    expect(await loadLinkedTasks()).toEqual([linked]);
  });
});

describe("mergeAzureDevOpsComments", () => {
  it("appends older pages without duplicating comments", () => {
    const current = [
      { id: 3, content: "new", author: { id: "u", displayName: "Ada" } },
      { id: 2, content: "middle", author: { id: "u", displayName: "Ada" } },
    ];
    const older = [
      { id: 2, content: "middle", author: { id: "u", displayName: "Ada" } },
      { id: 1, content: "old", author: { id: "u", displayName: "Ada" } },
    ];
    expect(mergeAzureDevOpsComments(current, older).map((comment) => comment.id)).toEqual([
      3, 2, 1,
    ]);
  });
});
