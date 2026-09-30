import { expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { SidebarTaskPageResponse, SidebarTaskQuery, Task } from "@/lib/types/http";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { recordTaskOverviewChange } from "@/lib/state/slices/task-overview-merge";
import { SidebarTaskPageCache } from "./sidebar-task-page-cache";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
const query: SidebarTaskQuery = {
  filters: [],
  sort: { key: "title", direction: "asc" },
  group: "none",
  collapsed_group_keys: [],
  collapsed_task_ids: [],
  page: 1,
  page_size: 100,
  locale: "en",
};
const task = (id: string): Task => ({
  id: taskId(id),
  workspace_id: workspaceId("workspace"),
  workflow_id: workflowId("workflow"),
  workflow_step_id: "step",
  title: id,
  description: "",
  position: 0,
  state: "TODO",
  priority: "medium",
  created_at: "2026-09-29T00:00:00Z",
  updated_at: "2026-09-29T00:00:00Z",
});
const page = (ids: string[]): SidebarTaskPageResponse => ({
  query_key: "page",
  page: 1,
  page_size: 100,
  total_tasks: ids.length,
  total_visible_tasks: ids.length,
  total_entries: ids.length + 1,
  has_next: false,
  has_previous: false,
  entries: ids.map((id) => ({ kind: "task", task_id: id, task: task(id) })),
});

it("publishes the first safe response after three soft invalidations and never caches it as authoritative", async () => {
  const store = createAppStore({ workspaces: { activeId: "workspace", items: [] } });
  const cache = new SidebarTaskPageCache(store);
  let resolve!: (response: SidebarTaskPageResponse) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const read = cache.request("workspace", query, "view");
  for (const [id, patch] of [
    ["live", { id: "live", title: "Latest live title", updatedAt: "2026-09-29T01:00:00Z" }],
    ["deleted", null],
    ["archived", { id: "archived", isArchived: true }],
  ] as const) {
    store.setState((state) => ({
      taskOverview: recordTaskOverviewChange(state.taskOverview, id, patch),
      sidebarArchivedTasks: {
        ...state.sidebarArchivedTasks,
        revisionByWorkspaceId: {
          workspace: (state.sidebarArchivedTasks.revisionByWorkspaceId.workspace ?? 0) + 1,
        },
      },
    }));
  }
  resolve(page(["live", "deleted", "archived"]));
  const result = await read.promise;
  expect(result.entries.map((entry) => entry.task_id)).toEqual(["live"]);
  expect(result.provisional).toBe(true);
  expect(store.getState().taskOverview.byId.live.title).toBe("Latest live title");
  expect(cache.get("view")).toBeNull();
  expect(result.entries.every((entry) => entry.task === undefined)).toBe(true);
  store
    .getState()
    .retainTaskOverviews("sidebar:display", [store.getState().taskOverview.byId.live]);
  read.release();
  expect(store.getState().taskOverview.byId.live).toBeDefined();
  store.getState().releaseTaskOverviews("sidebar:display");
  expect(store.getState().taskOverview.byId).toEqual({});
});

it("releases cached entity ownership on expiry while a board owner survives", async () => {
  vi.useFakeTimers();
  try {
    const store = createAppStore({ workspaces: { activeId: "workspace", items: [] } });
    const cache = new SidebarTaskPageCache(store);
    vi.mocked(querySidebarTasks).mockResolvedValueOnce(page(["one"]));
    const request = cache.request("workspace", query, "view");
    await request.promise;
    request.release();
    expect(Object.keys(store.getState().taskOverview.byId)).toEqual(["one"]);
    store.getState().setWorkflowSnapshot("workflow", {
      workflowId: "workflow",
      workflowName: "Workflow",
      steps: [],
      tasks: [store.getState().taskOverview.byId.one],
    });
    vi.advanceTimersByTime(5 * 60 * 1000);
    expect(cache.get("view")).toBeNull();
    expect(store.getState().taskOverview.byId.one).toBeDefined();
    store.getState().resetKanbanWorkspaceContext();
    expect(store.getState().taskOverview.byId).toEqual({});
  } finally {
    vi.useRealTimers();
  }
});
