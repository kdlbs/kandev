import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
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

describe("detail mutation ownership", () => {
  it.each(["close", "replace", "reopen", "workspace"])(
    "rejects an older opening's completion after %s",
    async (change) => {
      apiMocks.detail.mockImplementation(async (_workspace, _project, id) => ({
        ...workItem,
        id,
        planningFields: [],
      }));
      const { result, rerender } = renderHook(
        ({ enabled, item, workspace }) =>
          useAzureDevOpsWorkItemDetail(workspace, project.id, item, enabled),
        { initialProps: { enabled: true, item: workItem, workspace: workspaceId } },
      );
      await waitFor(() => expect(result.current.loading).toBe(false));
      const mutation = result.current.beginMutation();
      if (change === "close" || change === "reopen") {
        rerender({ enabled: false, item: workItem, workspace: workspaceId });
        if (change === "reopen")
          rerender({ enabled: true, item: workItem, workspace: workspaceId });
      } else if (change === "replace") {
        rerender({ enabled: true, item: { ...workItem, id: 508 }, workspace: workspaceId });
      } else {
        rerender({ enabled: true, item: workItem, workspace: "workspace-other" });
      }
      await waitFor(() => expect(result.current.loading).toBe(false));
      const current = result.current.item;
      expect(mutation.isCurrent()).toBe(false);
      act(() => mutation.mergeItem({ ...workItem, revision: 99, title: "Late update" }));
      expect(result.current.item).toEqual(current);
    },
  );

  it("keeps an opening across a refresh and accepts its current mutation", async () => {
    const { result } = renderHook(() =>
      useAzureDevOpsWorkItemDetail(workspaceId, project.id, workItem, true),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    const mutation = result.current.beginMutation();
    await act(async () => {
      await result.current.refresh();
    });
    expect(mutation.isCurrent()).toBe(true);
    act(() => mutation.mergeItem({ ...workItem, revision: 5, title: "Committed update" }));
    expect(result.current.item?.title).toBe("Committed update");
  });
});

it("keeps an assignment receipt without merging it into a closed opening", async () => {
  let finish!: (value: typeof workItem) => void;
  apiMocks.assignment.mockReturnValue(
    new Promise<typeof workItem>((resolve) => {
      finish = resolve;
    }),
  );
  const { result, rerender } = renderHook(
    ({ enabled }) => useAzureDevOpsWorkItemDetail(workspaceId, project.id, workItem, enabled),
    { initialProps: { enabled: true } },
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  const pending = result.current.updateAssignee("assign_current_user");
  rerender({ enabled: false });
  await act(async () => {
    finish({ ...workItem, revision: 5 });
    await expect(pending).resolves.toMatchObject({ revision: 5 });
  });
  expect(result.current.item).toBeNull();
});

it("retires mutation ownership on unmount", async () => {
  const { result, unmount } = renderHook(() =>
    useAzureDevOpsWorkItemDetail(workspaceId, project.id, workItem, true),
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  const mutation = result.current.beginMutation();
  unmount();
  expect(mutation.isCurrent()).toBe(false);
});

it("reestablishes opening ownership after StrictMode's lifecycle check", async () => {
  const { result } = renderHook(
    () => useAzureDevOpsWorkItemDetail(workspaceId, project.id, workItem, true),
    { reactStrictMode: true },
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.beginMutation().isCurrent()).toBe(true);
});
