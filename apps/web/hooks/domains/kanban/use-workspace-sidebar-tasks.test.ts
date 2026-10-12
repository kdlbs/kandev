import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { toKanbanTask } from "@/lib/kanban/map-task";

const mocks = vi.hoisted(() => ({
  state: {
    tasks: { activeTaskId: null as string | null },
    taskOverview: { byId: {} as Record<string, TaskOverview> },
    taskRemoval: {
      pendingTokenByTaskId: {} as Record<string, string>,
      operationsByToken: {} as Record<string, unknown>,
    },
    kanbanMulti: { snapshots: {} as Record<string, unknown> },
    sidebarStatusSummaryByWorkspaceId: {} as Record<string, Record<string, unknown>>,
    workflows: {
      items: [] as Array<{ id: string; workspaceId: string; name: string; hidden?: boolean }>,
    },
    kanban: {
      workflowId: null as string | null,
      steps: [] as Array<{ id: string; title: string; color: string; position: number }>,
    },
    workspaceContextGeneration: 0,
    workspaceContextRead: undefined as unknown,
    requestWorkspaceContextRefresh: vi.fn(),
  },
  page: {
    filters: [] as SidebarTaskQuery["filters"],
    response: null as SidebarTaskPageResponse | null,
    isLoading: false,
    error: null as string | null,
    refresh: vi.fn(),
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/kanban/use-sidebar-store-tasks", () => ({
  useSidebarStoreTasks: vi.fn(() => null),
}));
vi.mock("@/hooks/domains/kanban/use-sidebar-task-page", () => ({
  useSidebarTaskPage: vi.fn(() => ({
    ...mocks.page,
    view: { id: "view-1", group: "none", filters: mocks.page.filters },
  })),
}));

import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";
import { useSidebarTaskPage } from "./use-sidebar-task-page";
import { useSidebarStoreTasks } from "./use-sidebar-store-tasks";

function task(id: string, overrides: Partial<Task> = {}): Task {
  return {
    id,
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    title: id,
    description: "",
    state: "TODO",
    priority: "medium",
    position: 0,
    created_at: "2026-09-26T10:00:00Z",
    updated_at: "2026-09-26T10:00:00Z",
    ...overrides,
  } as Task;
}

function setPageTasks(tasks: Task[]) {
  mocks.state.taskOverview.byId = Object.fromEntries(
    tasks.map((item) => [item.id, toKanbanTask(item)]),
  );
  mocks.page.response = {
    query_key: "query-1",
    page: 1,
    page_size: 100,
    total_entries: tasks.length + 1,
    total_tasks: tasks.length,
    total_visible_tasks: tasks.length,
    has_previous: false,
    has_next: false,
    entries: [
      { kind: "group", group_key: "__all__", group_label: "__all__" },
      ...tasks.map((item) => ({ kind: "task" as const, task_id: item.id })),
    ],
  };
}

beforeEach(() => {
  mocks.state.tasks.activeTaskId = null;
  mocks.state.taskOverview.byId = {};
  mocks.state.taskRemoval = { pendingTokenByTaskId: {}, operationsByToken: {} };
  mocks.state.kanbanMulti = { snapshots: {} };
  mocks.state.sidebarStatusSummaryByWorkspaceId = {};
  mocks.state.workflows = { items: [{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }] };
  mocks.state.kanban = {
    workflowId: null,
    steps: [{ id: "step-1", title: "Start", color: "blue", position: 0 }],
  };
  mocks.state.workspaceContextGeneration = 0;
  mocks.state.workspaceContextRead = undefined;
  mocks.page.filters = [];
  mocks.page.response = null;
  mocks.page.isLoading = false;
  mocks.page.error = null;
  vi.clearAllMocks();
});

describe("useWorkspaceSidebarTasks", () => {
  it("reads active command data without owning a sidebar page", () => {
    setPageTasks([task("page-a"), task("page-b")]);
    mocks.state.tasks.activeTaskId = "active";
    mocks.state.taskOverview.byId.active = toKanbanTask(
      task("active", { archived_at: "2026-09-30T10:00:00Z" }),
    );

    const { result, rerender } = renderHook(() => useWorkspaceSidebarTasks("ws-1", true));

    expect(useSidebarStoreTasks).toHaveBeenLastCalledWith(null);
    expect(useSidebarTaskPage).toHaveBeenLastCalledWith(null, false, null);
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["active"]);
    expect(result.current.allTasks[0]?.isArchived).toBe(true);

    mocks.state.tasks.activeTaskId = "page-b";
    rerender();
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["page-b"]);
  });

  it("uses only the bounded page and does not leak workspace snapshot tasks", () => {
    setPageTasks([task("page-a"), task("page-b")]);
    mocks.state.kanbanMulti.snapshots = {
      "wf-1": {
        workflowId: "wf-1",
        workflowName: "Workflow",
        steps: [{ id: "step-1", title: "Start", color: "blue", position: 0 }],
        tasks: [task("off-page-task")],
      },
    };

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.allTasks.map((item) => item.id)).toEqual(["page-a", "page-b"]);
    expect(result.current.allTasks.map((item) => item._workflowId)).toEqual(["wf-1", "wf-1"]);
    expect(result.current.allTasks).toHaveLength(2);
  });

  it("overlays newer live status summaries on visible page rows", () => {
    setPageTasks([task("page-a", { status_summary: { revision: 1, updated_at: "old" } })]);
    mocks.state.sidebarStatusSummaryByWorkspaceId = {
      "ws-1": {
        "page-a": { revision: 2, updated_at: "new", pending_action: "clarification" },
      },
    };

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.allTasks[0]?.statusSummary).toMatchObject({
      revision: 2,
      pending_action: "clarification",
    });
  });

  it("keeps pending archive state for current rows and adopts the next accepted page", () => {
    setPageTasks([task("archive-target"), task("sibling")]);
    const { result, rerender } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    mocks.state.taskRemoval = {
      pendingTokenByTaskId: { "archive-target": "token-1" },
      operationsByToken: {
        "token-1": { action: "archive", workspaceId: "ws-1" },
      },
    };
    rerender();
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["archive-target", "sibling"]);
    expect(result.current.pendingRemovalTaskIds).toEqual(new Set(["archive-target"]));

    setPageTasks([task("sibling")]);
    mocks.state.taskRemoval = { pendingTokenByTaskId: {}, operationsByToken: {} };
    rerender();
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["sibling"]);
    expect(result.current.pendingRemovalTaskIds).toEqual(new Set());
  });

  it("uses queue position computed across tasks outside the current view page", () => {
    const queuedTask = task("queued-filtered", { queued_for_step_id: "step-1" });
    mocks.state.taskOverview.byId[queuedTask.id] = toKanbanTask(queuedTask);
    mocks.page.response = {
      query_key: "query-wip",
      page: 1,
      page_size: 100,
      total_entries: 2,
      total_tasks: 1,
      total_visible_tasks: 1,
      has_previous: false,
      has_next: false,
      entries: [
        { kind: "group", group_key: "__all__", group_label: "__all__" },
        {
          kind: "task",
          task_id: queuedTask.id,
          workflow_step_name: "Start",
          wip_queue_position: 3,
          wip_queue_total: 3,
        } as unknown as SidebarTaskPageResponse["entries"][number],
      ],
    };

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.wipQueueByTaskId.get(queuedTask.id)).toEqual({
      position: 3,
      total: 3,
      destinationTitle: "Start",
    });
  });
});

it("hides a known archive immediately while the server replacement remains pending", () => {
  setPageTasks([task("changed"), task("retained")]);
  const { result, rerender } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
  expect(result.current.allTasks.map((item) => item.id)).toEqual(["changed", "retained"]);
  mocks.state.taskOverview.byId = {
    ...mocks.state.taskOverview.byId,
    changed: { ...mocks.state.taskOverview.byId.changed, isArchived: true },
  };
  rerender();
  expect(result.current.allTasks.map((item) => item.id)).toEqual(["retained"]);
});

it("allows workspace-list recovery without retrying a denied task page", () => {
  mocks.state.workspaceContextRead = {
    workspaceId: "ws-1",
    generation: 0,
    errors: { workflows: "access_denied" },
    pending: {},
    snapshotError: null,
  };
  const hook = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
  expect(hook.result.current.workspaceContextAccessDenied).toBe(true);
  expect(hook.result.current.retryWorkspaceContext).toBe(
    mocks.state.requestWorkspaceContextRefresh,
  );
  mocks.state.workspaceContextRead = {
    workspaceId: "ws-1",
    generation: 0,
    errors: {},
    pending: {},
    snapshotError: "access_denied",
  };
  hook.rerender();
  expect(hook.result.current.workspaceContextAccessDenied).toBe(true);
  expect(hook.result.current.retryWorkspaceContext).toBeUndefined();
});

describe("sidebar pending removal projection", () => {
  beforeEach(() => {
    mocks.page.filters = [{ dimension: "archived", op: "in", value: ["true", "false"] }];
  });
  // @covers AC-TASKS-REMOVAL-NAVIGATION-005.1, AC-TASKS-REMOVAL-NAVIGATION-005.2
  it.each([false, true])(
    "keeps delete loading through refresh and restores current data (archived=%s)",
    (archived) => {
      const target = task("delete-target", {
        archived_at: archived ? "2026-09-29T10:00:00Z" : null,
      });
      setPageTasks([target, task("sibling")]);
      const { result, rerender } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
      mocks.state.taskRemoval = {
        pendingTokenByTaskId: { "delete-target": "delete-token" },
        operationsByToken: { "delete-token": { action: "delete", workspaceId: "ws-1" } },
      };
      rerender();
      expect(result.current.pendingRemovalTaskIds).toEqual(new Set(["delete-target"]));
      setPageTasks([{ ...target, title: "Updated during deletion" }, task("sibling")]);
      rerender();
      expect(result.current.pendingRemovalTaskIds).toEqual(new Set(["delete-target"]));
      mocks.state.taskRemoval = { pendingTokenByTaskId: {}, operationsByToken: {} };
      rerender();
      expect(result.current.pendingRemovalTaskIds.size).toBe(0);
      expect(result.current.allTasks[0].title).toBe("Updated during deletion");
    },
  );

  // @covers AC-TASKS-REMOVAL-NAVIGATION-005.3
  it("projects bulk and cascade membership and retains only failed targets after reconciliation", () => {
    setPageTasks([task("parent"), task("child"), task("failed"), task("survivor")]);
    mocks.state.taskRemoval = {
      pendingTokenByTaskId: { parent: "batch", child: "batch", failed: "batch" },
      operationsByToken: { batch: { action: "delete", workspaceId: "ws-1" } },
    };
    const { result, rerender } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(result.current.pendingRemovalTaskIds).toEqual(new Set(["parent", "child", "failed"]));
    setPageTasks([task("failed"), task("survivor")]);
    rerender();
    expect(result.current.pendingRemovalTaskIds).toEqual(new Set(["failed"]));
    mocks.state.taskRemoval = { pendingTokenByTaskId: {}, operationsByToken: {} };
    rerender();
    expect(result.current.pendingRemovalTaskIds.size).toBe(0);
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["failed", "survivor"]);
  });

  it("ignores missing tokens, other workspaces, and archived archive targets", () => {
    setPageTasks([
      task("stale"),
      task("foreign"),
      task("archived", { archived_at: "2026-09-29T10:00:00Z" }),
      task("active"),
      task("unselected-child"),
    ]);
    mocks.state.taskRemoval = {
      pendingTokenByTaskId: {
        stale: "missing",
        foreign: "other",
        archived: "archive",
        active: "archive",
      },
      operationsByToken: {
        other: { action: "delete", workspaceId: "ws-2" },
        archive: { action: "archive", workspaceId: "ws-1" },
      },
    };
    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(result.current.pendingRemovalTaskIds).toEqual(new Set(["active"]));
    expect(result.current.allTasks).toHaveLength(5);
  });
});
